package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive/experimental/mcp"
	"github.com/google/uuid"
)

//go:embed mcp.json
var configJSON []byte

func main() {
	query := flag.String("query", "Dive Go MCP integration", "search objective and query")
	pageURL := flag.String("url", "", "fetch this URL instead of searching")
	sessionID := flag.String("session-id", "", "reuse this session ID across related calls (default: a new UUID)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := run(ctx, configJSON, *query, *pageURL, *sessionID, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

// run discovers the remote tools and calls one through Dive's Tool interface.
// It makes no model request and does not read saved credentials. An empty
// sessionID generates a new one.
func run(ctx context.Context, config []byte, query, pageURL, sessionID string, out io.Writer) error {
	if sessionID == "" {
		sessionID = uuid.NewString()
	}
	servers, err := mcp.ParseServersJSON(config)
	if err != nil {
		return err
	}
	cfg, ok := servers["parallel"]
	if !ok {
		return fmt.Errorf("configuration must contain the parallel server")
	}
	client, err := mcp.NewClient(cfg)
	if err != nil {
		return err
	}
	if err := client.Connect(ctx); err != nil {
		return err
	}
	defer client.Close()
	tools, err := client.ListTools(ctx)
	if err != nil {
		return err
	}

	name := "web_search"
	args := map[string]any{
		"objective":      query,
		"search_queries": []string{query},
		"session_id":     sessionID,
	}
	if pageURL != "" {
		name = "web_fetch"
		args = map[string]any{"urls": []string{pageURL}, "session_id": sessionID}
	}
	for _, tool := range tools {
		if tool.Name != name {
			continue
		}
		adapter := mcp.NewQualifiedToolAdapter(client, tool, "parallel")
		result, err := adapter.Call(ctx, args)
		if err != nil {
			return err
		}
		if result.IsError {
			var messages []string
			for _, content := range result.Content {
				messages = append(messages, content.Text)
			}
			return fmt.Errorf("%s failed: %s", adapter.Name(), strings.Join(messages, "\n"))
		}
		for _, content := range result.Content {
			if content.Text != "" {
				if _, err := fmt.Fprintln(out, content.Text); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return fmt.Errorf("server did not advertise %s", name)
}
