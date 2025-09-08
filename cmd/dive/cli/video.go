package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/dive/media/providers/google"
	"github.com/spf13/cobra"
	"google.golang.org/genai"
)

var videoCmd = &cobra.Command{
	Use:   "video",
	Short: "Video generation commands",
	Long:  "Commands for generating videos using AI providers like Google Veo.",
}

var videoGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate videos from text prompts",
	Long:  "Generate videos from text prompts using AI providers. Currently supports Google Veo models.",
	RunE:  runVideoGenerate,
}

var videoStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of a video generation operation",
	Long:  "Check the status of a long-running video generation operation using its operation ID.",
	RunE:  runVideoStatus,
}

var (
	// Generate flags
	videoGeneratePrompt       string
	videoGenerateProvider     string
	videoGenerateModel        string
	videoGenerateOutput       string
	videoGenerateDuration     float64
	videoGenerateWait         bool
	videoGeneratePollInterval time.Duration

	// Status flags
	videoStatusOperationID string
	videoStatusProvider    string
)

func runVideoGenerate(cmd *cobra.Command, args []string) error {
	// Validate required parameters
	if videoGeneratePrompt == "" {
		return fmt.Errorf("prompt is required")
	}

	// Set defaults
	if videoGenerateProvider == "" {
		videoGenerateProvider = "google"
	}
	if videoGenerateModel == "" {
		videoGenerateModel = "veo-2.0-generate-001"
	}
	if videoGenerateOutput == "" {
		videoGenerateOutput = "generated_video.mp4"
	}

	return generateVideoWithMediaPackage()
}

func generateVideoWithMediaPackage() error {
	// Create the video generator directly
	generator, cleanup, err := createVideoGenerator(videoGenerateProvider)
	if err != nil {
		return fmt.Errorf("error creating provider %s: %v", videoGenerateProvider, err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	// Create the media request
	req := &media.VideoGenerationRequest{
		Prompt:   videoGeneratePrompt,
		Model:    videoGenerateModel,
		Duration: videoGenerateDuration,
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

	if videoGenerateWait {
		// Wait for completion
		fmt.Println("Waiting for video generation to complete...")
		return waitForVideoCompletion(generator, response.OperationID)
	}

	fmt.Printf("Use 'dive video status --operation-id %s --provider %s' to check the status\n",
		response.OperationID, videoGenerateProvider)
	return nil
}

func waitForVideoCompletion(generator media.VideoGenerator, operationID string) error {
	// Check if the generator supports operation checking
	checker, ok := generator.(media.VideoOperationChecker)
	if !ok {
		return fmt.Errorf("provider does not support operation status checking")
	}

	pollInterval := videoGeneratePollInterval
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

func runVideoStatus(cmd *cobra.Command, args []string) error {
	// Validate required parameters
	if videoStatusOperationID == "" {
		return fmt.Errorf("operation-id is required")
	}

	// Set defaults
	if videoStatusProvider == "" {
		videoStatusProvider = "google"
	}

	// Create the video generator directly
	generator, cleanup, err := createVideoGenerator(videoStatusProvider)
	if err != nil {
		return fmt.Errorf("error creating provider %s: %v", videoStatusProvider, err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	// Check if the generator supports operation checking
	checker, ok := generator.(media.VideoOperationChecker)
	if !ok {
		return fmt.Errorf("provider %s does not support operation status checking", videoStatusProvider)
	}

	// Check operation status
	ctx := context.Background()
	status, err := checker.CheckVideoOperation(ctx, videoStatusOperationID)
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
		fmt.Println("✅ Video generation completed!")
		if result, ok := status.Result.(*media.VideoGenerationResponse); ok {
			if len(result.Videos) > 0 {
				return saveVideo(result.Videos[0])
			}
		}
	case "failed":
		fmt.Printf("❌ Video generation failed: %s\n", status.Error)
	case "running":
		fmt.Println("🔄 Video generation is in progress...")
	case "pending":
		fmt.Println("⏳ Video generation is queued...")
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

func init() {
	rootCmd.AddCommand(videoCmd)

	// Add subcommands
	videoCmd.AddCommand(videoGenerateCmd)
	videoCmd.AddCommand(videoStatusCmd)

	// Generate command flags
	videoGenerateCmd.Flags().StringVarP(&videoGeneratePrompt, "prompt", "p", "", "Text description of the desired video (required)")
	videoGenerateCmd.Flags().StringVarP(&videoGenerateProvider, "provider", "", "google", "AI provider to use (currently only google)")
	videoGenerateCmd.Flags().StringVarP(&videoGenerateModel, "model", "m", "veo-2.0-generate-001", "Model to use for video generation")
	videoGenerateCmd.Flags().StringVarP(&videoGenerateOutput, "output", "o", "generated_video.mp4", "Output file path")
	videoGenerateCmd.Flags().Float64VarP(&videoGenerateDuration, "duration", "d", 0, "Duration of the video in seconds (optional)")
	videoGenerateCmd.Flags().BoolVarP(&videoGenerateWait, "wait", "w", false, "Wait for video generation to complete")
	videoGenerateCmd.Flags().DurationVar(&videoGeneratePollInterval, "poll-interval", 20*time.Second, "Polling interval when waiting")

	// Mark required flags
	videoGenerateCmd.MarkFlagRequired("prompt")

	// Status command flags
	videoStatusCmd.Flags().StringVarP(&videoStatusOperationID, "operation-id", "i", "", "Operation ID to check (required)")
	videoStatusCmd.Flags().StringVarP(&videoStatusProvider, "provider", "", "google", "AI provider used for the operation")

	// Mark required flags
	videoStatusCmd.MarkFlagRequired("operation-id")
}
