// Command mcpserver is a tiny stdio MCP server for the CLI's MCP tests and
// for trying MCP support by hand. It has two tools: "echo" returns its
// message as text, and "test_image" returns a 64x64 PNG whose left half is
// red and right half is blue.
//
//	go build -o /tmp/mcpserver ./testdata/mcpserver
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	s := server.NewMCPServer("dive-test", "1.0.0")
	s.AddTool(
		mcp.NewTool("echo",
			mcp.WithDescription("Return the message unchanged"),
			mcp.WithString("message", mcp.Required(), mcp.Description("Text to echo")),
		),
		func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("echo: " + req.GetString("message", "")), nil
		},
	)
	s.AddTool(
		mcp.NewTool("test_image",
			mcp.WithDescription("Return a small test image"),
			mcp.WithReadOnlyHintAnnotation(true),
		),
		func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			data, err := testPNG()
			if err != nil {
				return nil, err
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.NewImageContent(data, "image/png")},
			}, nil
		},
	)
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// testPNG draws the image and returns it base64-encoded.
func testPNG() (string, error) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			c := color.RGBA{R: 255, A: 255}
			if x >= 32 {
				c = color.RGBA{B: 255, A: 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
