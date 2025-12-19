package cli

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/dive/media/providers/google"
	"github.com/deepnoodle-ai/dive/media/providers/openai"
	wontoncli "github.com/deepnoodle-ai/wonton/cli"
	openaiapi "github.com/openai/openai-go"
	"google.golang.org/genai"
)

// imageGenerateParams holds parameters for image generation
type imageGenerateParams struct {
	prompt            string
	size              string
	provider          string
	output            string
	stdout            bool
	model             string
	quality           string
	count             int
	moderation        string
	outputFormat      string
	outputCompression int
	includeRAI        bool
	includeSafety     bool
	outputMIMEType    string
}

// imageEditParams holds parameters for image editing
type imageEditParams struct {
	input      string
	prompt     string
	mask       string
	provider   string
	output     string
	stdout     bool
	model      string
	size       string
	moderation string
}

func runImageGenerate(params imageGenerateParams) error {
	// Validate required parameters
	if err := validateGenerateImageParams(params); err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	// Set defaults
	params = setGenerateImageDefaults(params)

	return generateImageWithMediaPackage(params)
}

func generateImageWithMediaPackage(params imageGenerateParams) error {
	// Normalize provider name
	provider := strings.ToLower(params.provider)
	if provider == "dalle" {
		provider = "openai" // Map dalle to openai for backward compatibility
	}

	// Create the image generator directly
	generator, cleanup, err := createImageGenerator(provider)
	if err != nil {
		return fmt.Errorf("error creating provider %s: %v", provider, err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	// Create the media request
	req := &media.ImageGenerationRequest{
		Prompt:  params.prompt,
		Model:   params.model,
		Size:    params.size,
		Quality: params.quality,
		Count:   params.count,
	}

	if req.Count == 0 {
		req.Count = 1
	}

	// Set response format for base64 output
	req.ResponseFormat = "b64_json"

	// Add provider-specific parameters
	providerSpecific := make(map[string]interface{})

	if params.moderation != "" {
		providerSpecific["moderation"] = params.moderation
	}
	if params.outputFormat != "" {
		providerSpecific["output_format"] = params.outputFormat
	}
	if params.outputCompression > 0 {
		providerSpecific["output_compression"] = params.outputCompression
	}
	if params.includeRAI {
		providerSpecific["include_rai_reason"] = params.includeRAI
	}
	if params.includeSafety {
		providerSpecific["include_safety_attributes"] = params.includeSafety
	}
	if params.outputMIMEType != "" {
		providerSpecific["output_mime_type"] = params.outputMIMEType
	}

	if len(providerSpecific) > 0 {
		req.ProviderSpecific = providerSpecific
	}

	// Generate image
	ctx := context.Background()
	response, err := generator.GenerateImage(ctx, req)
	if err != nil {
		return fmt.Errorf("error generating image: %v", err)
	}

	if len(response.Images) == 0 {
		return fmt.Errorf("no images were generated")
	}

	// Handle output
	for i, imageData := range response.Images {
		if imageData.B64JSON == "" {
			return fmt.Errorf("no image data in response")
		}

		if params.stdout {
			// Output base64 to stdout
			fmt.Print(imageData.B64JSON)
		} else {
			// Decode and save to file
			imageBytes, err := base64.StdEncoding.DecodeString(imageData.B64JSON)
			if err != nil {
				return fmt.Errorf("error decoding base64 image: %v", err)
			}

			outputPath := params.output
			if params.count > 1 {
				ext := filepath.Ext(outputPath)
				name := strings.TrimSuffix(outputPath, ext)
				outputPath = fmt.Sprintf("%s_%d%s", name, i+1, ext)
			}

			// Create directory if needed
			dir := filepath.Dir(outputPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("error creating directory: %v", err)
			}

			// Write image to file
			if err := os.WriteFile(outputPath, imageBytes, 0644); err != nil {
				return fmt.Errorf("error saving image: %v", err)
			}

			fmt.Printf("Image saved to: %s\n", outputPath)
		}
	}

	return nil
}

func runImageEdit(params imageEditParams) error {
	// Validate required parameters
	if err := validateEditImageParams(params); err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	// Handle input from stdin or file
	inputImagePath, cleanupFunc, err := prepareEditInputImage(params.input)
	if err != nil {
		return fmt.Errorf("error preparing input image: %w", err)
	}
	if cleanupFunc != nil {
		defer cleanupFunc()
	}

	// Set defaults
	params = setEditImageDefaults(params)

	return editImageWithMediaPackage(inputImagePath, params)
}

func editImageWithMediaPackage(inputImagePath string, params imageEditParams) error {
	// Normalize provider name
	provider := strings.ToLower(params.provider)
	if provider == "dalle" {
		provider = "openai" // Map dalle to openai for backward compatibility
	}

	// Create the image generator directly
	generator, cleanup, err := createImageGenerator(provider)
	if err != nil {
		return fmt.Errorf("error creating provider %s: %v", provider, err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	// Open input image
	imageFile, err := os.Open(inputImagePath)
	if err != nil {
		return fmt.Errorf("error opening input image: %v", err)
	}
	defer imageFile.Close()

	// Create the media request
	req := &media.ImageEditRequest{
		Image:  imageFile,
		Prompt: params.prompt,
		Model:  params.model,
		Size:   params.size,
		Count:  1,
	}

	// Set response format for base64 output
	req.ResponseFormat = "b64_json"

	// Add mask if provided
	if params.mask != "" {
		maskFile, err := os.Open(params.mask)
		if err != nil {
			return fmt.Errorf("error opening mask image: %v", err)
		}
		defer maskFile.Close()
		req.Mask = maskFile
	}

	// Add provider-specific parameters
	if params.moderation != "" {
		if req.ProviderSpecific == nil {
			req.ProviderSpecific = make(map[string]interface{})
		}
		req.ProviderSpecific["moderation"] = params.moderation
	}

	// Edit image
	ctx := context.Background()
	response, err := generator.EditImage(ctx, req)
	if err != nil {
		return fmt.Errorf("error editing image: %v", err)
	}

	if len(response.Images) == 0 {
		return fmt.Errorf("no edited images were returned")
	}

	// Handle output
	imageData := response.Images[0]
	if imageData.B64JSON == "" {
		return fmt.Errorf("no image data in response")
	}

	if params.stdout {
		// Output base64 to stdout
		fmt.Print(imageData.B64JSON)
	} else {
		// Decode and save to file
		imageBytes, err := base64.StdEncoding.DecodeString(imageData.B64JSON)
		if err != nil {
			return fmt.Errorf("error decoding base64 image: %v", err)
		}

		// Create directory if needed
		dir := filepath.Dir(params.output)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("error creating directory: %v", err)
		}

		// Write image to file
		if err := os.WriteFile(params.output, imageBytes, 0644); err != nil {
			return fmt.Errorf("error saving edited image: %v", err)
		}

		fmt.Printf("Edited image saved to: %s\n", params.output)
	}

	return nil
}

// validateGenerateImageParams validates the parameters for image generation
func validateGenerateImageParams(params imageGenerateParams) error {
	if strings.TrimSpace(params.prompt) == "" {
		return fmt.Errorf("prompt is required and cannot be empty")
	}

	if params.provider != "" {
		validProviders := []string{"openai", "dalle", "google"}
		isValid := false
		for _, provider := range validProviders {
			if params.provider == provider {
				isValid = true
				break
			}
		}
		if !isValid {
			return fmt.Errorf("invalid provider '%s', must be one of: %s", params.provider, strings.Join(validProviders, ", "))
		}
	}

	if params.size != "" {
		validSizes := []string{"256x256", "512x512", "1024x1024", "1536x1024", "1024x1536", "1792x1024", "1024x1792", "auto"}
		isValid := false
		for _, size := range validSizes {
			if params.size == size {
				isValid = true
				break
			}
		}
		if !isValid {
			return fmt.Errorf("invalid size '%s', must be one of: %s", params.size, strings.Join(validSizes, ", "))
		}
	}

	if params.count < 1 || params.count > 10 {
		return fmt.Errorf("count must be between 1 and 10, got %d", params.count)
	}

	if params.quality != "" {
		validQualities := []string{"high", "medium", "low", "auto", "standard", "hd"}
		isValid := false
		for _, quality := range validQualities {
			if params.quality == quality {
				isValid = true
				break
			}
		}
		if !isValid {
			return fmt.Errorf("invalid quality '%s', must be one of: %s", params.quality, strings.Join(validQualities, ", "))
		}
	}

	return nil
}

// setGenerateImageDefaults sets default values for image generation parameters
func setGenerateImageDefaults(params imageGenerateParams) imageGenerateParams {
	if params.provider == "" {
		params.provider = "openai"
	}

	if params.size == "" {
		params.size = "1024x1024"
	}

	if params.model == "" {
		switch strings.ToLower(params.provider) {
		case "openai", "dalle":
			params.model = "gpt-image-1"
		case "google":
			params.model = "imagen-3.0-generate-002"
		}
	}

	// Create output path if not using stdout
	if !params.stdout && params.output == "" {
		params.output = "generated_image.png"
	}

	return params
}

// validateEditImageParams validates the parameters for image editing
func validateEditImageParams(params imageEditParams) error {
	if strings.TrimSpace(params.prompt) == "" {
		return fmt.Errorf("prompt is required and cannot be empty")
	}

	if params.provider != "" {
		validProviders := []string{"openai", "dalle"}
		isValid := false
		for _, provider := range validProviders {
			if params.provider == provider {
				isValid = true
				break
			}
		}
		if !isValid {
			return fmt.Errorf("invalid provider '%s', must be one of: %s (Google does not support image editing)", params.provider, strings.Join(validProviders, ", "))
		}
	}

	if params.size != "" {
		validSizes := []string{"256x256", "512x512", "1024x1024"}
		isValid := false
		for _, size := range validSizes {
			if params.size == size {
				isValid = true
				break
			}
		}
		if !isValid {
			return fmt.Errorf("invalid size '%s', must be one of: %s", params.size, strings.Join(validSizes, ", "))
		}
	}

	if params.model != "" && params.model != "dall-e-2" {
		return fmt.Errorf("invalid model '%s', only dall-e-2 supports image editing", params.model)
	}

	return nil
}

// setEditImageDefaults sets default values for image editing parameters
func setEditImageDefaults(params imageEditParams) imageEditParams {
	if params.provider == "" {
		params.provider = "openai"
	}

	if params.model == "" {
		params.model = "dall-e-2"
	}

	if params.size == "" {
		params.size = "1024x1024"
	}

	if params.output == "" && !params.stdout {
		params.output = "edited_image.png"
	}

	return params
}

// prepareEditInputImage handles input image preparation from file or stdin
func prepareEditInputImage(inputPath string) (string, func(), error) {
	if inputPath != "" {
		// Check if input file exists
		if _, err := os.Stat(inputPath); os.IsNotExist(err) {
			return "", nil, fmt.Errorf("input file does not exist: %s", inputPath)
		}
		return inputPath, nil, nil
	}

	// Handle stdin input
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", nil, fmt.Errorf("error checking stdin: %w", err)
	}

	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return "", nil, fmt.Errorf("input image path is required (use --input flag or pipe image data to stdin)")
	}

	// Data is being piped in, read from stdin
	inputData, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", nil, fmt.Errorf("error reading from stdin: %w", err)
	}

	if len(inputData) == 0 {
		return "", nil, fmt.Errorf("no data received from stdin")
	}

	// Create temporary file
	tmpFile, err := os.CreateTemp("", "dive_image_edit_*.png")
	if err != nil {
		return "", nil, fmt.Errorf("error creating temporary file: %w", err)
	}

	cleanupFunc := func() {
		os.Remove(tmpFile.Name())
	}

	// Check if input is base64
	if isBase64(string(inputData)) {
		// Decode base64
		imageBytes, err := base64.StdEncoding.DecodeString(string(inputData))
		if err != nil {
			tmpFile.Close()
			return "", cleanupFunc, fmt.Errorf("error decoding base64 input: %w", err)
		}
		if _, err := tmpFile.Write(imageBytes); err != nil {
			tmpFile.Close()
			return "", cleanupFunc, fmt.Errorf("error writing to temporary file: %w", err)
		}
	} else {
		// Assume it's raw image data
		if _, err := tmpFile.Write(inputData); err != nil {
			tmpFile.Close()
			return "", cleanupFunc, fmt.Errorf("error writing to temporary file: %w", err)
		}
	}

	tmpFile.Close()
	return tmpFile.Name(), cleanupFunc, nil
}

// isBase64 checks if a string is valid base64
func isBase64(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return false
	}
	_, err := base64.StdEncoding.DecodeString(s)
	return err == nil
}

// createImageGenerator creates an image generator for the specified provider
func createImageGenerator(provider string) (media.ImageGenerator, func(), error) {
	switch provider {
	case "openai", "dalle":
		return createOpenAIProvider()
	case "google":
		return createGoogleProvider()
	default:
		return nil, nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

// createOpenAIProvider creates an OpenAI provider instance
func createOpenAIProvider() (media.ImageGenerator, func(), error) {
	// Check for API key
	if os.Getenv("OPENAI_API_KEY") == "" {
		return nil, nil, fmt.Errorf("OPENAI_API_KEY environment variable is required")
	}

	client := openaiapi.NewClient()
	provider := openai.NewProvider(&client)
	return provider, nil, nil
}

// createGoogleProvider creates a Google GenAI provider instance
func createGoogleProvider() (media.ImageGenerator, func(), error) {
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

// hasGoogleCredentials checks if Google GenAI credentials are available
func hasGoogleCredentials() bool {
	// Check for Gemini API key
	if os.Getenv("GEMINI_API_KEY") != "" || os.Getenv("GOOGLE_API_KEY") != "" {
		return true
	}

	// Check for Vertex AI credentials
	if os.Getenv("GOOGLE_GENAI_USE_VERTEXAI") != "" && os.Getenv("GOOGLE_CLOUD_PROJECT") != "" {
		return true
	}

	return false
}

func registerImageCommand(app *wontoncli.App) {
	imageGroup := app.Group("image").
		Description("Image generation and editing commands")

	// Generate command
	imageGroup.Command("generate").
		Description("Generate images from text prompts").
		Long("Generate images from text prompts using AI providers. Outputs to file or stdout in base64 for piping into other tools.").
		NoArgs().
		Flags(
			wontoncli.String("prompt", "p").Required().Help("Text description of the desired image"),
			wontoncli.String("size", "s").Default("1024x1024").Help("Image resolution (e.g., 1024x1024, 1536x1024, 1024x1536)"),
			wontoncli.String("provider", "").Default("openai").Help("AI provider to use (openai, google)"),
			wontoncli.String("output", "o").Help("Output file path (defaults to generated_image.png)"),
			wontoncli.Bool("stdout", "").Help("Output base64 image to stdout instead of saving to file"),
			wontoncli.String("model", "m").Help("Model to use (OpenAI: dall-e-2, dall-e-3, gpt-image-1; Google: imagen-3.0-generate-001, imagen-3.0-generate-002)"),
			wontoncli.String("quality", "q").Help("Image quality (high, medium, low, auto for gpt-image-1; standard, hd for dall-e-3)"),
			wontoncli.Int("count", "n").Default(1).Help("Number of images to generate (1-10, only 1 supported for dall-e-3)"),
			wontoncli.String("moderation", "").Help("Moderation level (low, auto) - OpenAI only"),
			wontoncli.String("output-format", "").Help("Output format (png, jpeg, webp) - gpt-image-1 only"),
			wontoncli.Int("output-compression", "").Help("Output compression level (0-100) - gpt-image-1 with jpeg/webp only"),
			wontoncli.Bool("include-rai", "").Help("Include Responsible AI filter reasons - Google only"),
			wontoncli.Bool("include-safety", "").Help("Include safety attributes - Google only"),
			wontoncli.String("output-mime-type", "").Help("Output MIME type (e.g., image/jpeg) - Google only"),
		).
		Run(func(ctx *wontoncli.Context) error {
			parseGlobalFlags(ctx)

			params := imageGenerateParams{
				prompt:            ctx.String("prompt"),
				size:              ctx.String("size"),
				provider:          ctx.String("provider"),
				output:            ctx.String("output"),
				stdout:            ctx.Bool("stdout"),
				model:             ctx.String("model"),
				quality:           ctx.String("quality"),
				count:             ctx.Int("count"),
				moderation:        ctx.String("moderation"),
				outputFormat:      ctx.String("output-format"),
				outputCompression: ctx.Int("output-compression"),
				includeRAI:        ctx.Bool("include-rai"),
				includeSafety:     ctx.Bool("include-safety"),
				outputMIMEType:    ctx.String("output-mime-type"),
			}

			if err := runImageGenerate(params); err != nil {
				return wontoncli.Errorf("%v", err)
			}
			return nil
		})

	// Edit command
	imageGroup.Command("edit").
		Description("Edit existing images based on prompts").
		Long("Edit existing images based on text instructions. Supports input from stdin for composable workflows.").
		NoArgs().
		Flags(
			wontoncli.String("input", "i").Help("Input image file path (or read from stdin)"),
			wontoncli.String("prompt", "p").Required().Help("Text instructions for editing the image"),
			wontoncli.String("mask", "").Help("Mask image file path (optional)"),
			wontoncli.String("provider", "").Default("openai").Help("AI provider to use (openai - only OpenAI supports image editing)"),
			wontoncli.String("output", "o").Help("Output file path (defaults to edited_image.png)"),
			wontoncli.Bool("stdout", "").Help("Output base64 image to stdout instead of saving to file"),
			wontoncli.String("model", "m").Default("dall-e-2").Help("Model to use (only dall-e-2 supports image editing)"),
			wontoncli.String("size", "s").Default("1024x1024").Help("Output image size (256x256, 512x512, 1024x1024)"),
			wontoncli.String("moderation", "").Help("Moderation level (low, auto) - OpenAI only"),
		).
		Run(func(ctx *wontoncli.Context) error {
			parseGlobalFlags(ctx)

			params := imageEditParams{
				input:      ctx.String("input"),
				prompt:     ctx.String("prompt"),
				mask:       ctx.String("mask"),
				provider:   ctx.String("provider"),
				output:     ctx.String("output"),
				stdout:     ctx.Bool("stdout"),
				model:      ctx.String("model"),
				size:       ctx.String("size"),
				moderation: ctx.String("moderation"),
			}

			if err := runImageEdit(params); err != nil {
				return wontoncli.Errorf("%v", err)
			}
			return nil
		})
}
