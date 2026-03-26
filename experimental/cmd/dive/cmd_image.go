package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
		model = getDefaultImageModel()
		if model == "" {
			return fmt.Errorf("no image generation API key found (set OPENAI_API_KEY, GOOGLE_API_KEY, or XAI_API_KEY)")
		}
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
	count := ctx.Int("count")
	if count < 1 {
		count = 1
	}
	opts = append(opts, media.WithCount(count))

	timeout := 5 * time.Minute
	opts = append(opts, media.WithTimeout(timeout))

	fmt.Printf("Generating image with %s...\n", model)
	start := time.Now()

	results, err := media.GenerateImageBatch(context.Background(), prompt, opts...)
	if err != nil {
		return fmt.Errorf("generation failed: %w", err)
	}

	elapsed := time.Since(start)
	outBase := ctx.String("out")

	for i, result := range results {
		outPath := outBase
		if outPath == "" {
			slug := media.SlugifyPrompt(prompt, 40)
			if count > 1 {
				outPath = fmt.Sprintf("%s-%d%s", slug, i+1, result.Format.FileExtension())
			} else {
				outPath = slug + result.Format.FileExtension()
			}
		} else if count > 1 {
			ext := filepath.Ext(outPath)
			base := strings.TrimSuffix(outPath, ext)
			outPath = fmt.Sprintf("%s-%d%s", base, i+1, ext)
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
		cmd = exec.Command("cmd", "/c", "start", "", path)
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
