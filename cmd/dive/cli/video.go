package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/dive/media/providers/google"
	wontoncli "github.com/deepnoodle-ai/wonton/cli"
	"google.golang.org/genai"
)

// videoGenerateParams holds parameters for video generation
type videoGenerateParams struct {
	prompt       string
	provider     string
	model        string
	output       string
	duration     float64
	noWait       bool
	pollInterval time.Duration
}

// videoStatusParams holds parameters for video status check
type videoStatusParams struct {
	operationID string
	provider    string
}

func runVideoGenerate(params videoGenerateParams) error {
	// Validate required parameters
	if params.prompt == "" {
		return fmt.Errorf("prompt is required")
	}

	// Set defaults
	if params.provider == "" {
		params.provider = "google"
	}
	if params.model == "" {
		params.model = "veo-2.0-generate-001"
	}
	if params.output == "" {
		params.output = "generated_video.mp4"
	}

	// Determine wait behavior: wait by default unless --no-wait is specified
	shouldWait := !params.noWait

	return generateVideoWithMediaPackage(params, shouldWait)
}

func generateVideoWithMediaPackage(params videoGenerateParams, shouldWait bool) error {
	// Create the video generator directly
	generator, cleanup, err := createVideoGenerator(params.provider)
	if err != nil {
		return fmt.Errorf("error creating provider %s: %v", params.provider, err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	// Create the media request
	req := &media.VideoGenerationRequest{
		Prompt:   params.prompt,
		Model:    params.model,
		Duration: params.duration,
		Format:   "mp4", // Default format
	}

	// Generate video
	ctx := context.Background()
	response, err := generator.GenerateVideo(ctx, req)
	if err != nil {
		return fmt.Errorf("error generating video: %v", err)
	}

	fmt.Printf("Video generation started with operation ID: %s\n", response.OperationID)
	fmt.Printf("Status: %s\n", response.Status)

	if response.Status == "completed" && len(response.Videos) > 0 {
		// Video is already complete
		return saveVideo(response.Videos[0])
	}

	if shouldWait {
		// Wait for completion
		fmt.Println("Waiting for video generation to complete...")
		return waitForVideoCompletion(generator, response.OperationID, params.pollInterval)
	}

	fmt.Printf("Use 'dive video status --operation-id %s --provider %s' to check the status\n",
		response.OperationID, params.provider)
	return nil
}

func waitForVideoCompletion(generator media.VideoGenerator, operationID string, pollInterval time.Duration) error {
	// Check if the generator supports operation checking
	checker, ok := generator.(media.VideoOperationChecker)
	if !ok {
		return fmt.Errorf("provider does not support operation status checking")
	}

	if pollInterval == 0 {
		pollInterval = 20 * time.Second
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	ctx := context.Background()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			status, err := checker.CheckVideoOperation(ctx, operationID)
			if err != nil {
				return fmt.Errorf("error checking operation status: %v", err)
			}

			fmt.Printf("Status: %s", status.Status)
			if status.Progress > 0 {
				fmt.Printf(" (%d%%)", status.Progress)
			}
			fmt.Println()

			switch status.Status {
			case "completed":
				if result, ok := status.Result.(*media.VideoGenerationResponse); ok {
					if len(result.Videos) > 0 {
						return saveVideo(result.Videos[0])
					}
				}
				return fmt.Errorf("video generation completed but no video data found")
			case "failed":
				return fmt.Errorf("video generation failed: %s", status.Error)
			}
			// Continue polling for "pending" or "running" status
		}
	}
}

func saveVideo(video media.GeneratedVideo) error {
	if video.URL == "" && video.B64Data == "" {
		return fmt.Errorf("no video data available")
	}

	if video.B64Data != "" {
		// Save base64 data directly (for small videos)
		// Note: This is unlikely for videos but included for completeness
		return fmt.Errorf("base64 video data saving not implemented - videos are typically too large")
	}

	fmt.Printf("Video URL: %s\n", video.URL)
	fmt.Printf("Duration: %.2f seconds\n", video.Duration)
	fmt.Printf("Format: %s\n", video.Format)
	if video.Resolution != "" {
		fmt.Printf("Resolution: %s\n", video.Resolution)
	}

	// For now, just display the URL - actual download would require provider-specific logic
	fmt.Printf("Video is available at: %s\n", video.URL)
	fmt.Println("Note: Download the video manually from the URL above")

	return nil
}

func runVideoStatus(params videoStatusParams) error {
	// Validate required parameters
	if params.operationID == "" {
		return fmt.Errorf("operation-id is required")
	}

	// Set defaults
	provider := params.provider
	if provider == "" {
		provider = "google"
	}

	// Create the video generator directly
	generator, cleanup, err := createVideoGenerator(provider)
	if err != nil {
		return fmt.Errorf("error creating provider %s: %v", provider, err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	// Check if the generator supports operation checking
	checker, ok := generator.(media.VideoOperationChecker)
	if !ok {
		return fmt.Errorf("provider %s does not support operation status checking", provider)
	}

	// Check operation status
	ctx := context.Background()
	status, err := checker.CheckVideoOperation(ctx, params.operationID)
	if err != nil {
		return fmt.Errorf("error checking operation status: %v", err)
	}

	fmt.Printf("Operation ID: %s\n", status.ID)
	fmt.Printf("Status: %s\n", status.Status)
	if status.Progress > 0 {
		fmt.Printf("Progress: %d%%\n", status.Progress)
	}

	switch status.Status {
	case "completed":
		fmt.Println("Video generation completed!")
		if result, ok := status.Result.(*media.VideoGenerationResponse); ok {
			if len(result.Videos) > 0 {
				return saveVideo(result.Videos[0])
			}
		}
	case "failed":
		fmt.Printf("Video generation failed: %s\n", status.Error)
	case "running":
		fmt.Println("Video generation is in progress...")
	case "pending":
		fmt.Println("Video generation is queued...")
	}

	return nil
}

// createVideoGenerator creates a video generator for the specified provider
func createVideoGenerator(provider string) (media.VideoGenerator, func(), error) {
	switch provider {
	case "google":
		return createGoogleVideoProvider()
	default:
		return nil, nil, fmt.Errorf("unsupported video provider: %s (only google is supported)", provider)
	}
}

// createGoogleVideoProvider creates a Google GenAI provider instance for video generation
func createGoogleVideoProvider() (media.VideoGenerator, func(), error) {
	// Check for Google credentials
	if !hasGoogleCredentials() {
		return nil, nil, fmt.Errorf("google GenAI credentials are required (GEMINI_API_KEY, GOOGLE_API_KEY, or Vertex AI credentials)")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Google GenAI client: %w", err)
	}

	provider := google.NewProvider(client)
	cleanup := func() {
		// Google GenAI client doesn't require explicit cleanup in current version
	}
	return provider, cleanup, nil
}

func registerVideoCommand(app *wontoncli.App) {
	videoGroup := app.Group("video").
		Description("Video generation commands")

	// Generate command
	videoGroup.Command("generate").
		Description("Generate videos from text prompts").
		Long("Generate videos from text prompts using AI providers. Currently supports Google Veo models. Waits for completion by default - use --no-wait to return immediately.").
		NoArgs().
		Flags(
			wontoncli.String("prompt", "p").Required().Help("Text description of the desired video"),
			wontoncli.String("provider", "").Default("google").Help("AI provider to use (currently only google)"),
			wontoncli.String("model", "m").Default("veo-2.0-generate-001").Help("Model to use for video generation"),
			wontoncli.String("output", "o").Default("generated_video.mp4").Help("Output file path"),
			wontoncli.Float("duration", "d").Help("Duration of the video in seconds (optional)"),
			wontoncli.Bool("no-wait", "").Help("Don't wait for video generation to complete"),
			wontoncli.String("poll-interval", "").Default("20s").Help("Polling interval when waiting (e.g., 20s, 1m)"),
		).
		Run(func(ctx *wontoncli.Context) error {
			parseGlobalFlags(ctx)

			// Parse poll interval
			pollIntervalStr := ctx.String("poll-interval")
			pollInterval, err := time.ParseDuration(pollIntervalStr)
			if err != nil {
				pollInterval = 20 * time.Second
			}

			params := videoGenerateParams{
				prompt:       ctx.String("prompt"),
				provider:     ctx.String("provider"),
				model:        ctx.String("model"),
				output:       ctx.String("output"),
				duration:     ctx.Float64("duration"),
				noWait:       ctx.Bool("no-wait"),
				pollInterval: pollInterval,
			}

			if err := runVideoGenerate(params); err != nil {
				return wontoncli.Errorf("%v", err)
			}
			return nil
		})

	// Status command
	videoGroup.Command("status").
		Description("Check the status of a video generation operation").
		Long("Check the status of a long-running video generation operation using its operation ID.").
		NoArgs().
		Flags(
			wontoncli.String("operation-id", "i").Required().Help("Operation ID to check"),
			wontoncli.String("provider", "").Default("google").Help("AI provider used for the operation"),
		).
		Run(func(ctx *wontoncli.Context) error {
			parseGlobalFlags(ctx)

			params := videoStatusParams{
				operationID: ctx.String("operation-id"),
				provider:    ctx.String("provider"),
			}

			if err := runVideoStatus(params); err != nil {
				return wontoncli.Errorf("%v", err)
			}
			return nil
		})
}
