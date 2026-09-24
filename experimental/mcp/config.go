package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

// ParseServersJSON reads MCP server definitions in Claude Code's format: a
// JSON object whose "mcpServers" key maps server names to definitions. Any
// other top-level keys are ignored, so the same function reads a project
// .mcp.json file and a settings file that embeds an "mcpServers" block.
//
//	{
//	  "mcpServers": {
//	    "files":  {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-filesystem", "."]},
//	    "remote": {"type": "http", "url": "https://example.com/mcp", "headers": {"Authorization": "Bearer ${TOKEN}"}}
//	  }
//	}
//
// "type" is "stdio", "http", or "sse". When it is omitted, a definition with
// a command is stdio and one with a url is http. Each returned config has its
// Name set to its key. ${VAR} and ${VAR:-default} references are left in
// place; Client.Connect expands them.
//
// A malformed document returns an error and no servers. An invalid entry is
// left out, and the returned error describes it alongside the valid servers,
// so callers can warn about one bad entry and still use the rest.
func ParseServersJSON(data []byte) (map[string]*ServerConfig, error) {
	var doc struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	servers := make(map[string]*ServerConfig, len(doc.MCPServers))
	names := make([]string, 0, len(doc.MCPServers))
	for name := range doc.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	var errs []error
	for _, name := range names {
		cfg, err := parseServerEntry(name, doc.MCPServers[name])
		if err != nil {
			errs = append(errs, fmt.Errorf("mcp server %q: %w", name, err))
			continue
		}
		servers[name] = cfg
	}
	return servers, errors.Join(errs...)
}

// LoadServersFile reads a file with ParseServersJSON. A missing file returns
// an error satisfying errors.Is(err, fs.ErrNotExist).
func LoadServersFile(path string) (map[string]*ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	servers, err := ParseServersJSON(data)
	if err != nil {
		return servers, fmt.Errorf("%s: %w", path, err)
	}
	return servers, nil
}

func parseServerEntry(name string, raw json.RawMessage) (*ServerConfig, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("server name is empty")
	}
	var cfg ServerConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	cfg.Name = name
	cfg.Type = strings.ToLower(strings.TrimSpace(cfg.Type))
	if cfg.Type == "" {
		switch {
		case cfg.Command != "":
			cfg.Type = "stdio"
		case cfg.URL != "":
			cfg.Type = "http"
		}
	}
	switch cfg.Type {
	case "stdio":
		if strings.TrimSpace(cfg.Command) == "" {
			return nil, errors.New(`stdio server needs a "command"`)
		}
	case "http", "sse":
		if strings.TrimSpace(cfg.URL) == "" {
			return nil, fmt.Errorf(`%s server needs a "url"`, cfg.Type)
		}
	case "":
		return nil, errors.New(`needs a "command" (stdio) or a "url" (http, sse)`)
	default:
		return nil, fmt.Errorf("unsupported type %q (supported: stdio, http, sse)", cfg.Type)
	}
	return &cfg, nil
}

// QualifiedToolName returns the name Claude Code gives an MCP tool:
// "mcp__<server>__<tool>". Characters outside [A-Za-z0-9_-] become "_", since
// model providers accept only those in tool names. Permission rules can then
// match a whole server with a pattern such as "mcp__github__*".
func QualifiedToolName(server, tool string) string {
	return "mcp__" + sanitizeToolNamePart(server) + "__" + sanitizeToolNamePart(tool)
}

func sanitizeToolNamePart(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// ExpandEnv replaces ${VAR}, ${VAR:-default}, and $VAR references with
// values from the environment. ${VAR:-default} uses default when VAR is unset
// or empty. Unset variables without a default expand to "".
func ExpandEnv(s string) string {
	if !strings.Contains(s, "$") {
		return s
	}
	return os.Expand(s, func(key string) string {
		if name, def, ok := strings.Cut(key, ":-"); ok {
			if value := os.Getenv(name); value != "" {
				return value
			}
			return def
		}
		return os.Getenv(key)
	})
}

func expandEnvMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return m
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = ExpandEnv(v)
	}
	return out
}
