package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/wonton/cli"

	// Register media providers
	_ "github.com/deepnoodle-ai/dive/providers/google"
	_ "github.com/deepnoodle-ai/dive/providers/openai"
)

func runVideo(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) == 0 {
		return fmt.Errorf("usage: dive video \"prompt\" [flags]")
	}
	prompt := args[0]

	model := ctx.String("model")
	if model == "" {
		model = "veo-3.1-generate-preview"
	}

	var opts []media.Option
	opts = append(opts, media.WithModel(model))

	if ar := ctx.String("aspect"); ar != "" {
		opts = append(opts, media.WithAspectRatio(media.AspectRatio(ar)))
	}

	duration := 8 * time.Second
	if d := ctx.String("duration"); d != "" {
		parsed, err := time.ParseDuration(d)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", d, err)
		}
		duration = parsed
	}
	opts = append(opts, media.WithDuration(duration))
	opts = append(opts, media.WithTimeout(15*time.Minute))

	fmt.Printf("Generating video with %s (duration: %s)...\n", model, duration)
	start := time.Now()

	// Simple progress ticker
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				fmt.Printf("  Still generating... (%s elapsed)\n", time.Since(start).Round(time.Second))
			}
		}
	}()

	result, err := media.GenerateVideo(context.Background(), prompt, opts...)
	close(done)
	if err != nil {
		return fmt.Errorf("generation failed: %w", err)
	}

	elapsed := time.Since(start)

	// Determine output path
	outPath := ctx.String("out")
	if outPath == "" {
		slug := media.SlugifyPrompt(prompt, 40)
		ext := ".mp4"
		if result.Format == "webm" {
			ext = ".webm"
		}
		outPath = slug + ext
	}

	outPath, err = result.WriteTo(outPath)
	if err != nil {
		return fmt.Errorf("writing video: %w", err)
	}

	absPath, err := filepath.Abs(outPath)
	if err != nil {
		absPath = outPath
	}

	fmt.Printf("Saved: %s (%dx%d %s, %s, %s, %.1fs)\n",
		absPath, result.Width, result.Height,
		result.Format, result.Duration.Round(time.Second),
		formatBytes(len(result.Data)), elapsed.Seconds())

	if ctx.Bool("open") {
		openFile(absPath)
	}
	return nil
}
