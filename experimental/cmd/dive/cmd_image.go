package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/wonton/cli"

	// Register media providers
	_ "github.com/deepnoodle-ai/dive/providers/google"
	_ "github.com/deepnoodle-ai/dive/providers/openai"
)

func runImage(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) == 0 {
		return fmt.Errorf("usage: dive image \"prompt\" [flags]")
	}
	prompt := args[0]

	model := ctx.String("model")
	if model == "" {
		model = "gpt-image-1"
	}

	var opts []media.Option
	opts = append(opts, media.WithModel(model))

	if ar := ctx.String("aspect"); ar != "" {
		opts = append(opts, media.WithAspectRatio(media.AspectRatio(ar)))
	}
	if f := ctx.String("format"); f != "" {
		format := media.Format(f)
		if err := media.ValidateFormat(format); err != nil {
			return err
		}
		opts = append(opts, media.WithOutputFormat(format))
	}
	if n := ctx.Int("count"); n > 1 {
		opts = append(opts, media.WithCount(n))
	}

	timeout := 5 * time.Minute
	opts = append(opts, media.WithTimeout(timeout))

	fmt.Printf("Generating image with %s...\n", model)
	start := time.Now()

	result, err := media.GenerateImage(context.Background(), prompt, opts...)
	if err != nil {
		return fmt.Errorf("generation failed: %w", err)
	}

	elapsed := time.Since(start)

	// Determine output path
	outPath := ctx.String("out")
	if outPath == "" {
		slug := media.SlugifyPrompt(prompt, 40)
		outPath = slug + result.Format.FileExtension()
	}

	outPath, err = result.WriteTo(outPath)
	if err != nil {
		return fmt.Errorf("writing image: %w", err)
	}

	absPath, err := filepath.Abs(outPath)
	if err != nil {
		absPath = outPath
	}

	fmt.Printf("Saved: %s (%dx%d %s, %s, %.1fs)\n",
		absPath, result.Width, result.Height,
		result.Format, formatBytes(len(result.Data)), elapsed.Seconds())

	if ctx.Bool("open") {
		openFile(absPath)
	}
	return nil
}

func openFile(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", path)
	default:
		return
	}
	_ = cmd.Start()
}

func formatBytes(n int) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/1024/1024)
	case n >= 1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
