package cli

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	openaiapi "github.com/openai/openai-go"
	"github.com/spf13/cobra"
)

var imageCmd = &cobra.Command{
	Use:   "image",
	Short: "Image generation and editing commands",
	Long:  "Commands for generating and editing images using AI providers like DALL-E and Grok.",
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
	generatePrompt   string
	generateSize     string
	generateProvider string
	generateOutput   string
	generateStdout   bool
	generateModel    string
	generateQuality  string
	generateN        int

	// Edit flags
	editInput    string
	editPrompt   string
	editMask     string
	editProvider string
	editOutput   string
	editStdout   bool
	editModel    string
	editSize     string
)

func runImageGenerate(cmd *cobra.Command, args []string) error {
	// Validate required parameters
	if generatePrompt == "" {
		return fmt.Errorf("prompt is required")
	}

	// Set defaults
	if generateProvider == "" {
		generateProvider = "dalle"
	}
	if generateSize == "" {
		generateSize = "1024x1024"
	}
	if generateModel == "" {
		if generateProvider == "dalle" {
			generateModel = "gpt-image-1"
		}
	}

	// Create output path if not using stdout
	if !generateStdout && generateOutput == "" {
		generateOutput = "generated_image.png"
	}

	switch strings.ToLower(generateProvider) {
	case "dalle":
		return generateImageWithDALLE()
	case "grok":
		return fmt.Errorf("grok does not currently support image generation. Please use 'dalle' provider")
	default:
		return fmt.Errorf("unsupported provider: %s. Supported providers: dalle", generateProvider)
	}
}

func generateImageWithDALLE() error {
	// Get API key
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY environment variable is required")
	}

	// Create OpenAI client
	client := openaiapi.NewClient()

	// Set up parameters
	params := openaiapi.ImageGenerateParams{
		Prompt: generatePrompt,
		Model:  openaiapi.ImageModelGPTImage1,
		Size:   openaiapi.ImageGenerateParamsSize(generateSize),
		N:      openaiapi.Int(int64(generateN)),
	}

	// Set model
	if generateModel != "" {
		switch generateModel {
		case "dall-e-2":
			params.Model = openaiapi.ImageModelDallE2
		case "dall-e-3":
			params.Model = openaiapi.ImageModelDallE3
		case "gpt-image-1":
			params.Model = openaiapi.ImageModelGPTImage1
		default:
			return fmt.Errorf("invalid model: %s. Supported models: dall-e-2, dall-e-3, gpt-image-1", generateModel)
		}
	}

	// Set quality
	if generateQuality != "" {
		if params.Model == openaiapi.ImageModelGPTImage1 {
			switch generateQuality {
			case "high":
				params.Quality = openaiapi.ImageGenerateParamsQualityHigh
			case "medium":
				params.Quality = openaiapi.ImageGenerateParamsQualityMedium
			case "low":
				params.Quality = openaiapi.ImageGenerateParamsQualityLow
			case "auto":
				params.Quality = openaiapi.ImageGenerateParamsQualityAuto
			default:
				params.Quality = openaiapi.ImageGenerateParamsQualityAuto
			}
		} else if params.Model == openaiapi.ImageModelDallE3 {
			switch generateQuality {
			case "standard":
				params.Quality = openaiapi.ImageGenerateParamsQualityStandard
			case "hd":
				params.Quality = openaiapi.ImageGenerateParamsQualityHD
			default:
				params.Quality = openaiapi.ImageGenerateParamsQualityHD
			}
		}
	}

	// Set response format for non-gpt-image-1 models
	if params.Model != openaiapi.ImageModelGPTImage1 {
		params.ResponseFormat = openaiapi.ImageGenerateParamsResponseFormatB64JSON
	}

	// Generate image
	ctx := context.Background()
	response, err := client.Images.Generate(ctx, params)
	if err != nil {
		return fmt.Errorf("error generating image: %v", err)
	}

	if len(response.Data) == 0 {
		return fmt.Errorf("no images were generated")
	}

	// Handle output
	for i, imageData := range response.Data {
		var imageBytes []byte
		var err error

		if imageData.B64JSON != "" {
			imageBytes, err = base64.StdEncoding.DecodeString(imageData.B64JSON)
			if err != nil {
				return fmt.Errorf("error decoding base64 image: %v", err)
			}
		} else if imageData.URL != "" {
			return fmt.Errorf("URL response format not supported yet")
		} else {
			return fmt.Errorf("no image data in response")
		}

		if generateStdout {
			// Output base64 to stdout
			fmt.Print(imageData.B64JSON)
		} else {
			// Save to file
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
	if editPrompt == "" {
		return fmt.Errorf("prompt is required")
	}

	// Handle input from stdin or file
	var inputImagePath string
	if editInput == "" {
		// Check if input is coming from stdin
		stat, err := os.Stdin.Stat()
		if err != nil {
			return fmt.Errorf("error checking stdin: %v", err)
		}

		if (stat.Mode() & os.ModeCharDevice) == 0 {
			// Data is being piped in, read from stdin
			inputData, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("error reading from stdin: %v", err)
			}

			// Create temporary file
			tmpFile, err := os.CreateTemp("", "dive_image_edit_*.png")
			if err != nil {
				return fmt.Errorf("error creating temporary file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			// Check if input is base64
			if isBase64(string(inputData)) {
				// Decode base64
				imageBytes, err := base64.StdEncoding.DecodeString(string(inputData))
				if err != nil {
					return fmt.Errorf("error decoding base64 input: %v", err)
				}
				if _, err := tmpFile.Write(imageBytes); err != nil {
					return fmt.Errorf("error writing to temporary file: %v", err)
				}
			} else {
				// Assume it's raw image data
				if _, err := tmpFile.Write(inputData); err != nil {
					return fmt.Errorf("error writing to temporary file: %v", err)
				}
			}

			tmpFile.Close()
			inputImagePath = tmpFile.Name()
		} else {
			return fmt.Errorf("input image path is required (use --input flag or pipe data to stdin)")
		}
	} else {
		inputImagePath = editInput
	}

	// Set defaults
	if editProvider == "" {
		editProvider = "dalle"
	}
	if editOutput == "" && !editStdout {
		editOutput = "edited_image.png"
	}

	switch strings.ToLower(editProvider) {
	case "dalle":
		return editImageWithDALLE(inputImagePath)
	default:
		return fmt.Errorf("unsupported provider: %s. Supported providers: dalle", editProvider)
	}
}

func editImageWithDALLE(inputImagePath string) error {
	// Get API key
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY environment variable is required")
	}

	// Create OpenAI client
	client := openaiapi.NewClient()

	// Read input image
	imageFile, err := os.Open(inputImagePath)
	if err != nil {
		return fmt.Errorf("error opening input image: %v", err)
	}
	defer imageFile.Close()

	// Set up parameters
	params := openaiapi.ImageEditParams{
		Image:  openaiapi.ImageEditParamsImageUnion{OfFile: imageFile},
		Prompt: editPrompt,
		Model:  openaiapi.ImageModelDallE2, // DALL-E 2 is the only model that supports editing
		Size:   openaiapi.ImageEditParamsSize1024x1024,
	}

	// Set model if specified
	if editModel != "" {
		switch editModel {
		case "dall-e-2":
			params.Model = openaiapi.ImageModelDallE2
		default:
			return fmt.Errorf("invalid model for editing: %s. Only dall-e-2 supports image editing", editModel)
		}
	}

	// Set size if specified
	if editSize != "" {
		switch editSize {
		case "256x256":
			params.Size = openaiapi.ImageEditParamsSize256x256
		case "512x512":
			params.Size = openaiapi.ImageEditParamsSize512x512
		case "1024x1024":
			params.Size = openaiapi.ImageEditParamsSize1024x1024
		default:
			return fmt.Errorf("invalid size: %s. Supported sizes: 256x256, 512x512, 1024x1024", editSize)
		}
	}

	// Add mask if provided
	if editMask != "" {
		maskFile, err := os.Open(editMask)
		if err != nil {
			return fmt.Errorf("error opening mask image: %v", err)
		}
		defer maskFile.Close()
		params.Mask = maskFile
	}

	// Set response format
	params.ResponseFormat = openaiapi.ImageEditParamsResponseFormatB64JSON

	// Edit image
	ctx := context.Background()
	response, err := client.Images.Edit(ctx, params)
	if err != nil {
		return fmt.Errorf("error editing image: %v", err)
	}

	if len(response.Data) == 0 {
		return fmt.Errorf("no edited images were returned")
	}

	// Handle output
	imageData := response.Data[0]
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

// isBase64 checks if a string is valid base64
func isBase64(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return false
	}
	_, err := base64.StdEncoding.DecodeString(s)
	return err == nil
}

func init() {
	rootCmd.AddCommand(imageCmd)

	// Add subcommands
	imageCmd.AddCommand(imageGenerateCmd)
	imageCmd.AddCommand(imageEditCmd)

	// Generate command flags
	imageGenerateCmd.Flags().StringVarP(&generatePrompt, "prompt", "p", "", "Text description of the desired image (required)")
	imageGenerateCmd.Flags().StringVarP(&generateSize, "size", "s", "1024x1024", "Image resolution (e.g., 1024x1024, 1536x1024, 1024x1536)")
	imageGenerateCmd.Flags().StringVarP(&generateProvider, "provider", "", "dalle", "AI provider to use (dalle)")
	imageGenerateCmd.Flags().StringVarP(&generateOutput, "output", "o", "", "Output file path (defaults to generated_image.png)")
	imageGenerateCmd.Flags().BoolVarP(&generateStdout, "stdout", "", false, "Output base64 image to stdout instead of saving to file")
	imageGenerateCmd.Flags().StringVarP(&generateModel, "model", "m", "", "Model to use (dall-e-2, dall-e-3, gpt-image-1)")
	imageGenerateCmd.Flags().StringVarP(&generateQuality, "quality", "q", "", "Image quality (high, medium, low, auto for gpt-image-1; standard, hd for dall-e-3)")
	imageGenerateCmd.Flags().IntVarP(&generateN, "count", "n", 1, "Number of images to generate (1-10, only 1 supported for dall-e-3)")

	// Mark required flags
	imageGenerateCmd.MarkFlagRequired("prompt")

	// Edit command flags
	imageEditCmd.Flags().StringVarP(&editInput, "input", "i", "", "Input image file path (or read from stdin)")
	imageEditCmd.Flags().StringVarP(&editPrompt, "prompt", "p", "", "Text instructions for editing the image (required)")
	imageEditCmd.Flags().StringVarP(&editMask, "mask", "", "", "Mask image file path (optional)")
	imageEditCmd.Flags().StringVarP(&editProvider, "provider", "", "dalle", "AI provider to use (dalle)")
	imageEditCmd.Flags().StringVarP(&editOutput, "output", "o", "", "Output file path (defaults to edited_image.png)")
	imageEditCmd.Flags().BoolVarP(&editStdout, "stdout", "", false, "Output base64 image to stdout instead of saving to file")
	imageEditCmd.Flags().StringVarP(&editModel, "model", "m", "dall-e-2", "Model to use (only dall-e-2 supports image editing)")
	imageEditCmd.Flags().StringVarP(&editSize, "size", "s", "1024x1024", "Output image size (256x256, 512x512, 1024x1024)")

	// Mark required flags
	imageEditCmd.MarkFlagRequired("prompt")
}