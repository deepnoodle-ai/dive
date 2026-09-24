package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/experimental/mcp"
	"github.com/deepnoodle-ai/dive/experimental/settings"
	"github.com/deepnoodle-ai/wonton/tui"
)

// MCP servers come from three places, in Claude Code's "mcpServers" format.
// A server name defined in more than one wins by this precedence, lowest
// first:
//
//  1. ~/.dive/settings.json "mcpServers" (user)
//  2. <workspace>/.mcp.json (project; each server must be approved, below)
//  3. --mcp-config <file> (flag; repeatable, later files win)
//
// A project .mcp.json arrives with the repository, and starting one of its
// stdio servers runs an arbitrary command. So, as in Claude Code, project
// servers start only once approved, here in ~/.dive/settings.json with
// "enableAllProjectMcpServers": true or "enabledMcpjsonServers": ["name"].
// "disabledMcpjsonServers" wins over both. Passing the file explicitly with
// --mcp-config is also an approval.

const (
	mcpSourceUser    = "~/.dive/settings.json"
	mcpSourceProject = ".mcp.json"

	// mcpToolNameLimit is the longest tool name the providers accept.
	mcpToolNameLimit = 64

	defaultMCPConnectTimeout = 30 * time.Second
)

// mcpServerEntry is one server to start, with the file that defined it.
type mcpServerEntry struct {
	Config *mcp.ServerConfig
	Source string
}

// mcpConfigResult is the merged server list plus what was left out and why.
type mcpConfigResult struct {
	Servers []mcpServerEntry
	// Unapproved lists project .mcp.json servers that were not started.
	Unapproved []string
	// Warnings describes config files or entries that could not be used.
	Warnings []string
}

// mcpApproval holds the project-server approval keys from the user settings.
type mcpApproval struct {
	EnableAll bool     `json:"enableAllProjectMcpServers"`
	Enabled   []string `json:"enabledMcpjsonServers"`
	Disabled  []string `json:"disabledMcpjsonServers"`
}

func (a mcpApproval) approves(name string) bool {
	if slices.Contains(a.Disabled, name) {
		return false
	}
	return a.EnableAll || slices.Contains(a.Enabled, name)
}

// loadMCPServerConfigs merges the MCP server definitions for a workspace.
// Nothing here is fatal: an unreadable or invalid source becomes a warning and
// the remaining servers still load.
func loadMCPServerConfigs(workspaceDir string, flagFiles []string) mcpConfigResult {
	var result mcpConfigResult
	merged := make(map[string]mcpServerEntry)
	add := func(servers map[string]*mcp.ServerConfig, source string, keep func(string) bool) {
		for name, cfg := range servers {
			if keep != nil && !keep(name) {
				continue
			}
			merged[name] = mcpServerEntry{Config: cfg, Source: source}
		}
	}
	load := func(path string) map[string]*mcp.ServerConfig {
		servers, err := mcp.LoadServersFile(path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			result.Warnings = append(result.Warnings, err.Error())
		}
		return servers
	}

	// User tier. The same file carries the project approval keys.
	var approval mcpApproval
	if userPath, err := settings.UserSettingsPath(); err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	} else {
		add(load(userPath), mcpSourceUser, nil)
		if data, err := os.ReadFile(userPath); err == nil {
			// A malformed file was already reported by load.
			_ = json.Unmarshal(data, &approval)
		}
	}

	// Project tier, approved servers only.
	if workspaceDir != "" {
		project := load(filepath.Join(workspaceDir, ".mcp.json"))
		add(project, mcpSourceProject, func(name string) bool {
			if approval.approves(name) {
				return true
			}
			if _, userDefined := merged[name]; !userDefined {
				result.Unapproved = append(result.Unapproved, name)
			}
			return false
		})
		sort.Strings(result.Unapproved)
	}

	// Explicit files, in order.
	for _, path := range flagFiles {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		servers, err := mcp.LoadServersFile(path)
		if err != nil {
			result.Warnings = append(result.Warnings, err.Error())
		}
		add(servers, path, nil)
	}

	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result.Servers = append(result.Servers, merged[name])
	}
	return result
}

// mcpConnectTimeout is how long one server gets to start and list its tools.
// MCP_TIMEOUT (milliseconds) overrides it, as in Claude Code.
func mcpConnectTimeout() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("MCP_TIMEOUT")); raw != "" {
		if ms, err := strconv.Atoi(raw); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return defaultMCPConnectTimeout
}

// mcpServerStatus is the outcome of starting one server.
type mcpServerStatus struct {
	Name   string
	Source string
	Type   string
	Tools  []string // names exposed to the model
	Err    error    // set when the server failed to start
	Notes  []string // tools left out, and why
}

// mcpRuntime owns the connected MCP servers for one CLI run.
type mcpRuntime struct {
	servers    []mcpServerStatus
	unapproved []string
	warnings   []string
	tools      []dive.Tool
	clients    []*mcp.Client
}

// connectMCPServers starts every configured server in parallel, each bounded
// by timeout. A server that fails is recorded and skipped; the rest still
// load. The returned runtime is never nil and must be closed.
func connectMCPServers(ctx context.Context, cfg mcpConfigResult, timeout time.Duration) *mcpRuntime {
	rt := &mcpRuntime{
		unapproved: cfg.Unapproved,
		warnings:   cfg.Warnings,
		servers:    make([]mcpServerStatus, len(cfg.Servers)),
	}
	type connected struct {
		client *mcp.Client
		tools  []*mcp.ToolAdapter
	}
	results := make([]connected, len(cfg.Servers))

	var wg sync.WaitGroup
	for i, entry := range cfg.Servers {
		rt.servers[i] = mcpServerStatus{Name: entry.Config.Name, Source: entry.Source, Type: entry.Config.Type}
		wg.Add(1)
		go func() {
			defer wg.Done()
			client, tools, err := connectMCPServer(ctx, entry.Config, timeout)
			if err != nil {
				rt.servers[i].Err = err
				return
			}
			results[i] = connected{client: client, tools: tools}
		}()
	}
	wg.Wait()

	// Register tools in server-name order so duplicates resolve the same way
	// on every run.
	seen := make(map[string]string)
	for i := range rt.servers {
		status := &rt.servers[i]
		if results[i].client == nil {
			continue
		}
		rt.clients = append(rt.clients, results[i].client)
		for _, tool := range results[i].tools {
			name := tool.Name()
			if len(name) > mcpToolNameLimit {
				status.Notes = append(status.Notes, fmt.Sprintf("skipped %s: name longer than %d characters", name, mcpToolNameLimit))
				continue
			}
			if owner, dup := seen[name]; dup {
				status.Notes = append(status.Notes, fmt.Sprintf("skipped %s: name already used by server %q", name, owner))
				continue
			}
			seen[name] = status.Name
			status.Tools = append(status.Tools, name)
			rt.tools = append(rt.tools, tool)
		}
	}
	return rt
}

func connectMCPServer(ctx context.Context, cfg *mcp.ServerConfig, timeout time.Duration) (*mcp.Client, []*mcp.ToolAdapter, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, err := mcp.NewClient(cfg)
	if err != nil {
		return nil, nil, err
	}
	if err := client.Connect(ctx); err != nil {
		return nil, nil, mcpContextError(ctx, timeout, err)
	}
	listed, err := client.ListTools(ctx)
	if err != nil {
		_ = client.Close()
		return nil, nil, mcpContextError(ctx, timeout, err)
	}
	tools := make([]*mcp.ToolAdapter, 0, len(listed))
	for _, tool := range listed {
		tools = append(tools, mcp.NewQualifiedToolAdapter(client, tool, cfg.Name))
	}
	return client, tools, nil
}

func mcpContextError(ctx context.Context, timeout time.Duration, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("timed out after %s: %w", timeout, err)
	}
	return err
}

// Tools returns the tools of every connected server.
func (rt *mcpRuntime) Tools() []dive.Tool {
	if rt == nil {
		return nil
	}
	return rt.tools
}

// Close shuts down every server, stopping stdio child processes.
func (rt *mcpRuntime) Close() {
	if rt == nil {
		return
	}
	var wg sync.WaitGroup
	for _, client := range rt.clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = client.Close()
		}()
	}
	wg.Wait()
	rt.clients = nil
}

func (rt *mcpRuntime) connectedCount() (servers, tools int) {
	for _, s := range rt.servers {
		if s.Err == nil {
			servers++
		}
	}
	return servers, len(rt.tools)
}

func (rt *mcpRuntime) failedCount() int {
	n := 0
	for _, s := range rt.servers {
		if s.Err != nil {
			n++
		}
	}
	return n
}

// empty reports whether there is nothing MCP-related to show.
func (rt *mcpRuntime) empty() bool {
	return rt == nil || (len(rt.servers) == 0 && len(rt.unapproved) == 0 && len(rt.warnings) == 0)
}

// summary is the one-line startup description, or "" when there is nothing
// to say.
func (rt *mcpRuntime) summary() string {
	if rt.empty() {
		return ""
	}
	servers, tools := rt.connectedCount()
	parts := []string{fmt.Sprintf("%d %s connected (%d %s)",
		servers, plural(servers, "server", "servers"), tools, plural(tools, "tool", "tools"))}
	problems := false
	if failed := rt.failedCount(); failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
		problems = true
	}
	if n := len(rt.unapproved); n > 0 {
		parts = append(parts, fmt.Sprintf("%d %s approval", n, plural(n, "needs", "need")))
		problems = true
	}
	if len(rt.warnings) > 0 {
		parts = append(parts, "config warnings")
		problems = true
	}
	line := strings.Join(parts, ", ")
	if problems {
		line += " · /mcp for details"
	}
	return line
}

// problems returns one line per failure or warning, for print mode's stderr.
func (rt *mcpRuntime) problems() []string {
	if rt == nil {
		return nil
	}
	var lines []string
	lines = append(lines, rt.warnings...)
	for _, s := range rt.servers {
		if s.Err != nil {
			lines = append(lines, fmt.Sprintf("MCP server %q failed to start: %v", s.Name, s.Err))
		}
		lines = append(lines, s.Notes...)
	}
	if len(rt.unapproved) > 0 {
		lines = append(lines, unapprovedMCPMessage(rt.unapproved))
	}
	return lines
}

func unapprovedMCPMessage(names []string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = strconv.Quote(name)
	}
	return fmt.Sprintf("Project .mcp.json servers not started until approved: %s. "+
		"Approve them in ~/.dive/settings.json with \"enabledMcpjsonServers\": [%s] "+
		"(or \"enableAllProjectMcpServers\": true), or pass --mcp-config .mcp.json.",
		strings.Join(names, ", "), strings.Join(quoted, ", "))
}

// report renders the /mcp output as plain lines.
func (rt *mcpRuntime) report() []string {
	if rt.empty() {
		return []string{
			"No MCP servers configured.",
			"Add them under \"mcpServers\" in ~/.dive/settings.json or .mcp.json, or pass --mcp-config <file>.",
		}
	}
	var lines []string
	for _, s := range rt.servers {
		if s.Err != nil {
			lines = append(lines, fmt.Sprintf("✗ %s (%s, from %s): failed: %v", s.Name, s.Type, s.Source, s.Err))
			continue
		}
		lines = append(lines, fmt.Sprintf("✓ %s (%s, from %s): %d %s",
			s.Name, s.Type, s.Source, len(s.Tools), plural(len(s.Tools), "tool", "tools")))
		for _, tool := range s.Tools {
			lines = append(lines, "    "+tool)
		}
		for _, note := range s.Notes {
			lines = append(lines, "    "+note)
		}
	}
	if len(rt.unapproved) > 0 {
		lines = append(lines, "", unapprovedMCPMessage(rt.unapproved))
	}
	if len(rt.warnings) > 0 {
		lines = append(lines, "", "Config warnings:")
		for _, w := range rt.warnings {
			lines = append(lines, "  "+w)
		}
	}
	return lines
}

// printMCPReport shows the /mcp report in the transcript.
func (a *App) printMCPReport() {
	views := []tui.View{tui.Text("MCP servers").Bold()}
	for _, line := range a.mcp.report() {
		views = append(views, tui.Text("  %s", line))
	}
	a.appendReport(tui.Stack(views...))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// startMCPServers loads the config and connects, reporting progress on
// stderr when announce is set and there is anything to start. Cancelling ctx
// abandons the remaining connects.
func startMCPServers(ctx context.Context, workspaceDir string, flagFiles []string, announce bool) *mcpRuntime {
	cfg := loadMCPServerConfigs(workspaceDir, flagFiles)
	if announce && len(cfg.Servers) > 0 {
		fmt.Fprintf(os.Stderr, "Connecting to %d MCP %s...\n", len(cfg.Servers), plural(len(cfg.Servers), "server", "servers"))
	}
	return connectMCPServers(ctx, cfg, mcpConnectTimeout())
}
