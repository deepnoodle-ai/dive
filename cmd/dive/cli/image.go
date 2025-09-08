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
	openaiapi "github.com/openai/openai-go"
	"github.com/spf13/cobra"
	"google.golang.org/genai"
)

var imageCmd = &cobra.Command{
	Use:   "image",
	Short: "Image generation and editing commands",
	Long:  "Commands for generating and editing images using AI providers like OpenAI DALL-E, gpt-image-1, and Google Imagen.",
}

var imageGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate images from text prompts",
	Long:  "Generate images from text prompts using AI providers. Outputs to file or stdout in base64 for piping into other tools.",
	RunE:  runImageGenerate,
}

var imageEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit existing images based on prompts",
	Long:  "Edit existing images based on text instructions. Supports input from stdin for composable workflows.",
	RunE:  runImageEdit,
}

var (
	// Generate flags
	generatePrompt            string
	generateSize              string
	generateProvider          string
	generateOutput            string
	generateStdout            bool
	generateModel             string
	generateQuality           string
	generateN                 int
	generateModeration        string
	generateOutputFormat      string
	generateOutputCompression int
	generateIncludeRAI        bool
	generateIncludeSafety     bool
	generateOutputMIMEType    string

	// Edit flags
	editInput      string
	editPrompt     string
	editMask       string
	editProvider   string
	editOutput     string
	editStdout     bool
	editModel      string
	editSize       string
	editModeration string
)

func runImageGenerate(cmd *cobra.Command, args []string) error {
	// Validate required parameters
	if err := validateGenerateImageParams(); err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	// Set defaults
	if err := setGenerateImageDefaults(); err != nil {
		return fmt.Errorf("error setting defaults: %w", err)
	}

	return generateImageWithMediaPackage()
}

func generateImageWithMediaPackage() error {
	// Normalize provider name
	provider := strings.ToLower(generateProvider)
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
		Prompt:  generatePrompt,
		Model:   generateModel,
		Size:    generateSize,
		Quality: generateQuality,
		Count:   generateN,
	}

	if req.Count == 0 {
		req.Count = 1
	}

	// Set response format for base64 output
	req.ResponseFormat = "b64_json"

	// Add provider-specific parameters
	providerSpecific := make(map[string]interface{})

	if generateModeration != "" {
		providerSpecific["moderation"] = generateModeration
	}
	if generateOutputFormat != "" {
		providerSpecific["output_format"] = generateOutputFormat
	}
	if generateOutputCompression > 0 {
		providerSpecific["output_compression"] = generateOutputCompression
	}
	if generateIncludeRAI {
		providerSpecific["include_rai_reason"] = generateIncludeRAI
	}
	if generateIncludeSafety {
		providerSpecific["include_safety_attributes"] = generateIncludeSafety
	}
	if generateOutputMIMEType != "" {
		providerSpecific["output_mime_type"] = generateOutputMIMEType
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

		if generateStdout {
			// Output base64 to stdout
			fmt.Print(imageData.B64JSON)
		} else {
			// Decode and save to file
			imageBytes, err := base64.StdEncoding.DecodeString(imageData.B64JSON)
			if err != nil {
				return fmt.Errorf("error decoding base64 image: %v", err)
			}

			outputPath := generateOutput
			if generateN > 1 {
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

func runImageEdit(cmd *cobra.Command, args []string) error {
	// Validate required parameters
	if err := validateEditImageParams(); err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	// Handle input from stdin or file
	inputImagePath, cleanupFunc, err := prepareEditInputImage()
	if err != nil {
		return fmt.Errorf("error preparing input image: %w", err)
	}
	if cleanupFunc != nil {
		defer cleanupFunc()
	}

	// Set defaults
	if err := setEditImageDefaults(); err != nil {
		return fmt.Errorf("error setting defaults: %w", err)
	}

	return editImageWithMediaPackage(inputImagePath)
}

func editImageWithMediaPackage(inputImagePath string) error {
	// Normalize provider name
	provider := strings.ToLower(editProvider)
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
		Prompt: editPrompt,
		Model:  editModel,
		Size:   editSize,
		Count:  1,
	}

	// Set response format for base64 output
	req.ResponseFormat = "b64_json"

	// Add mask if provided
	if editMask != "" {
		maskFile, err := os.Open(editMask)
		if err != nil {
			return fmt.Errorf("error opening mask image: %v", err)
		}
		defer maskFile.Close()
		req.Mask = maskFile
	}

	// Add provider-specific parameters
	if editModeration != "" {
		if req.ProviderSpecific == nil {
			req.ProviderSpecific = make(map[string]interface{})
		}
		req.ProviderSpecific["moderation"] = editModeration
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

	if editStdout {
		// Output base64 to stdout
		fmt.Print(imageData.B64JSON)
	} else {
		// Decode and save to file
		imageBytes, err := base64.StdEncoding.DecodeString(imageData.B64JSON)
		if err != nil {
			return fmt.Errorf("error decoding base64 image: %v", err)
		}

		// Create directory if needed
		dir := filepath.Dir(editOutput)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("error creating directory: %v", err)
		}

		// Write image to file
		if err := os.WriteFile(editOutput, imageBytes, 0644); err != nil {
			return fmt.Errorf("error saving edited image: %v", err)
		}

		fmt.Printf("Edited image saved to: %s\n", editOutput)
	}

	return nil
}

// validateGenerateImageParams validates the parameters for image generation
func validateGenerateImageParams() error {
	if strings.TrimSpace(generatePrompt) == "" {
		return fmt.Errorf("prompt is required and cannot be empty")
	}

	if generateProvider != "" {
		validProviders := []string{"openai", "dalle", "google"}
		isValid := false
		for _, provider := range validProviders {
			if generateProvider == provider {
				isValid = true
				break
			}
		}
		if !isValid {
			return fmt.Errorf("invalid provider '%s', must be one of: %s", generateProvider, strings.Join(validProviders, ", "))
		}
	}

	if generateSize != "" {
		validSizes := []string{"256x256", "512x512", "1024x1024", "1536x1024", "1024x1536", "1792x1024", "1024x1792", "auto"}
		isValid := false
		for _, size := range validSizes {
			if generateSize == size {
				isValid = true
				break
			}
		}
		if !isValid {
			return fmt.Errorf("invalid size '%s', must be one of: %s", generateSize, strings.Join(validSizes, ", "))
		}
	}

	if generateN < 1 || generateN > 10 {
		return fmt.Errorf("count must be between 1 and 10, got %d", generateN)
	}

	if generateQuality != "" {
		validQualities := []string{"high", "medium", "low", "auto", "standard", "hd"}
		isValid := false
		for _, quality := range validQualities {
			if generateQuality == quality {
				isValid = true
				break
			}
		}
		if !isValid {
			return fmt.Errorf("invalid quality '%s', must be one of: %s", generateQuality, strings.Join(validQualities, ", "))
		}
	}

	return nil
}

// setGenerateImageDefaults sets default values for image generation parameters
func setGenerateImageDefaults() error {
	if generateProvider == "" {
		generateProvider = "openai"
	}

	if generateSize == "" {
		generateSize = "1024x1024"
	}

	if generateModel == "" {
		switch strings.ToLower(generateProvider) {
		case "openai", "dalle":
			generateModel = "gpt-image-1"
		case "google":
			generateModel = "imagen-3.0-generate-002"
		default:
			return fmt.Errorf("unsupported provider: %s", generateProvider)
		}
	}

	// Create output path if not using stdout
	if !generateStdout && generateOutput == "" {
		generateOutput = "generated_image.png"
	}

	return nil
}

// validateEditImageParams validates the parameters for image editing
func validateEditImageParams() error {
	if strings.TrimSpace(editPrompt) == "" {
		return fmt.Errorf("prompt is required and cannot be empty")
	}

	if editProvider != "" {
		validProviders := []string{"openai", "dalle"}
		isValid := false
		for _, provider := range validProviders {
			if editProvider == provider {
				isValid = true
				break
			}
		}
		if !isValid {
			return fmt.Errorf("invalid provider '%s', must be one of: %s (Google does not support image editing)", editProvider, strings.Join(validProviders, ", "))
		}
	}

	if editSize != "" {
		validSizes := []string{"256x256", "512x512", "1024x1024"}
		isValid := false
		for _, size := range validSizes {
			if editSize == size {
				isValid = true
				break
			}
		}
		if !isValid {
			return fmt.Errorf("invalid size '%s', must be one of: %s", editSize, strings.Join(validSizes, ", "))
		}
	}

	if editModel != "" && editModel != "dall-e-2" {
		return fmt.Errorf("invalid model '%s', only dall-e-2 supports image editing", editModel)
	}

	return nil
}

// setEditImageDefaults sets default values for image editing parameters
func setEditImageDefaults() error {
	if editProvider == "" {
		editProvider = "openai"
	}

	if editModel == "" {
		editModel = "dall-e-2"
	}

	if editSize == "" {
		editSize = "1024x1024"
	}

	if editOutput == "" && !editStdout {
		editOutput = "edited_image.png"
	}

	return nil
}

// prepareEditInputImage handles input image preparation from file or stdin
func prepareEditInputImage() (string, func(), error) {
	if editInput != "" {
		// Check if input file exists
		if _, err := os.Stat(editInput); os.IsNotExist(err) {
			return "", nil, fmt.Errorf("input file does not exist: %s", editInput)
		}
		return editInput, nil, nil
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

func init() {
	rootCmd.AddCommand(imageCmd)

	// Add subcommands
	imageCmd.AddCommand(imageGenerateCmd)
	imageCmd.AddCommand(imageEditCmd)

	// Generate command flags
	imageGenerateCmd.Flags().StringVarP(&generatePrompt, "prompt", "p", "", "Text description of the desired image (required)")
	imageGenerateCmd.Flags().StringVarP(&generateSize, "size", "s", "1024x1024", "Image resolution (e.g., 1024x1024, 1536x1024, 1024x1536)")
	imageGenerateCmd.Flags().StringVarP(&generateProvider, "provider", "", "openai", "AI provider to use (openai, google)")
	imageGenerateCmd.Flags().StringVarP(&generateOutput, "output", "o", "", "Output file path (defaults to generated_image.png)")
	imageGenerateCmd.Flags().BoolVarP(&generateStdout, "stdout", "", false, "Output base64 image to stdout instead of saving to file")
	imageGenerateCmd.Flags().StringVarP(&generateModel, "model", "m", "", "Model to use (OpenAI: dall-e-2, dall-e-3, gpt-image-1; Google: imagen-3.0-generate-001, imagen-3.0-generate-002)")
	imageGenerateCmd.Flags().StringVarP(&generateQuality, "quality", "q", "", "Image quality (high, medium, low, auto for gpt-image-1; standard, hd for dall-e-3)")
	imageGenerateCmd.Flags().IntVarP(&generateN, "count", "n", 1, "Number of images to generate (1-10, only 1 supported for dall-e-3)")

	// Provider-specific flags for generation
	imageGenerateCmd.Flags().StringVar(&generateModeration, "moderation", "", "Moderation level (low, auto) - OpenAI only")
	imageGenerateCmd.Flags().StringVar(&generateOutputFormat, "output-format", "", "Output format (png, jpeg, webp) - gpt-image-1 only")
	imageGenerateCmd.Flags().IntVar(&generateOutputCompression, "output-compression", 0, "Output compression level (0-100) - gpt-image-1 with jpeg/webp only")
	imageGenerateCmd.Flags().BoolVar(&generateIncludeRAI, "include-rai", false, "Include Responsible AI filter reasons - Google only")
	imageGenerateCmd.Flags().BoolVar(&generateIncludeSafety, "include-safety", false, "Include safety attributes - Google only")
	imageGenerateCmd.Flags().StringVar(&generateOutputMIMEType, "output-mime-type", "", "Output MIME type (e.g., image/jpeg) - Google only")

	// Mark required flags
	imageGenerateCmd.MarkFlagRequired("prompt")

	// Edit command flags
	imageEditCmd.Flags().StringVarP(&editInput, "input", "i", "", "Input image file path (or read from stdin)")
	imageEditCmd.Flags().StringVarP(&editPrompt, "prompt", "p", "", "Text instructions for editing the image (required)")
	imageEditCmd.Flags().StringVarP(&editMask, "mask", "", "", "Mask image file path (optional)")
	imageEditCmd.Flags().StringVarP(&editProvider, "provider", "", "openai", "AI provider to use (openai - only OpenAI supports image editing)")
	imageEditCmd.Flags().StringVarP(&editOutput, "output", "o", "", "Output file path (defaults to edited_image.png)")
	imageEditCmd.Flags().BoolVarP(&editStdout, "stdout", "", false, "Output base64 image to stdout instead of saving to file")
	imageEditCmd.Flags().StringVarP(&editModel, "model", "m", "dall-e-2", "Model to use (only dall-e-2 supports image editing)")
	imageEditCmd.Flags().StringVarP(&editSize, "size", "s", "1024x1024", "Output image size (256x256, 512x512, 1024x1024)")

	// Provider-specific flags for editing
	imageEditCmd.Flags().StringVar(&editModeration, "moderation", "", "Moderation level (low, auto) - OpenAI only")

	// Mark required flags
	imageEditCmd.MarkFlagRequired("prompt")
}
