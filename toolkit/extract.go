package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/schema"
)

var _ dive.TypedTool[*ExtractInput] = &ExtractTool{}

// ExtractInput represents the input for the extraction tool
type ExtractInput struct {
	// Path to the input file (text, image, PDF, etc.)
	InputPath string `json:"input_path"`

	// JSON schema defining the structure to extract
	Schema map[string]interface{} `json:"schema"`

	// Optional: Path to save the extracted data
	OutputPath string `json:"output_path,omitempty"`

	// Optional: Bias filtering instructions
	BiasFilter string `json:"bias_filter,omitempty"`

	// Optional: Additional extraction instructions
	Instructions string `json:"instructions,omitempty"`
}

// ExtractToolOptions configures the extraction tool
type ExtractToolOptions struct {
	MaxFileSize int64 `json:"max_file_size,omitempty"`
}

// ExtractTool implements structured data extraction from various input types
type ExtractTool struct {
	maxFileSize int64
}

// NewExtractTool creates a new extraction tool
func NewExtractTool(options ExtractToolOptions) *dive.TypedToolAdapter[*ExtractInput] {
	if options.MaxFileSize == 0 {
		options.MaxFileSize = 10 * 1024 * 1024 // 10MB default
	}
	return dive.ToolAdapter(&ExtractTool{
		maxFileSize: options.MaxFileSize,
	})
}

func (t *ExtractTool) Name() string {
	return "extract"
}

func (t *ExtractTool) Description() string {
	return "Extract structured data from text, images, or PDF files using a JSON schema. Supports bias filtering and custom extraction instructions."
}

func (t *ExtractTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"input_path", "schema"},
		Properties: map[string]*schema.Property{
			"input_path": {
				Type:        "string",
				Description: "Path to the input file (text, image, PDF, etc.)",
			},
			"schema": {
				Type:        "object",
				Description: "JSON schema defining the structure to extract from the input",
			},
			"output_path": {
				Type:        "string",
				Description: "Optional path to save the extracted JSON data",
			},
			"bias_filter": {
				Type:        "string",
				Description: "Optional instructions for filtering or avoiding bias in extraction",
			},
			"instructions": {
				Type:        "string",
				Description: "Additional instructions for the extraction process",
			},
		},
	}
}

func (t *ExtractTool) Call(ctx context.Context, input *ExtractInput) (*dive.ToolResult, error) {
	if input.InputPath == "" {
		return NewToolResultError("Error: No input path provided"), nil
	}

	if input.Schema == nil {
		return NewToolResultError("Error: No schema provided"), nil
	}

	// Resolve absolute path
	absPath, err := filepath.Abs(input.InputPath)
	if err != nil {
		return NewToolResultError(fmt.Sprintf("Error: Failed to resolve absolute path: %s", err.Error())), nil
	}

	// Check if file exists and get info
	fileInfo, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewToolResultError(fmt.Sprintf("Error: File not found at path: %s", input.InputPath)), nil
		}
		return NewToolResultError(fmt.Sprintf("Error: Failed to access file: %s", err.Error())), nil
	}

	// Check file size
	if fileInfo.Size() > t.maxFileSize {
		return NewToolResultError(fmt.Sprintf("Error: File %s is too large (%d bytes). Maximum allowed size is %d bytes",
			input.InputPath, fileInfo.Size(), t.maxFileSize)), nil
	}

	// Detect file type and read content
	fileType, content, err := t.readAndDetectFileType(absPath)
	if err != nil {
		return NewToolResultError(fmt.Sprintf("Error: Failed to read file: %s", err.Error())), nil
	}

	// Build analysis result
	analysisResult := t.buildAnalysisResult(input, fileType, content)

	// Format as JSON for the agent to process
	jsonResult, err := json.MarshalIndent(analysisResult, "", "  ")
	if err != nil {
		return NewToolResultError(fmt.Sprintf("Error: Failed to format result: %s", err.Error())), nil
	}

	return NewToolResultText(string(jsonResult)), nil
}

func (t *ExtractTool) readAndDetectFileType(path string) (string, []byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()

	// Read file content
	content, err := io.ReadAll(file)
	if err != nil {
		return "", nil, err
	}

	// Detect file type
	fileType := t.detectFileType(path, content)
	return fileType, content, nil
}

func (t *ExtractTool) detectFileType(path string, content []byte) string {
	// Check file extension first
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".pdf":
		return "pdf"
	case ".txt", ".md", ".csv", ".json", ".xml", ".html":
		return "text"
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp":
		return "image"
	}

	// Use MIME type detection
	mimeType := http.DetectContentType(content)
	mediaType, _, _ := mime.ParseMediaType(mimeType)

	switch {
	case strings.HasPrefix(mediaType, "image/"):
		return "image"
	case strings.HasPrefix(mediaType, "text/"):
		return "text"
	case mediaType == "application/pdf":
		return "pdf"
	default:
		// Default to text if we can't determine
		return "text"
	}
}

func (t *ExtractTool) getContentPreview(content []byte, fileType string) string {
	switch fileType {
	case "text":
		preview := string(content)
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		return preview
	case "image":
		return fmt.Sprintf("Image file (%d bytes)", len(content))
	case "pdf":
		return fmt.Sprintf("PDF file (%d bytes)", len(content))
	default:
		return fmt.Sprintf("File content (%d bytes)", len(content))
	}
}

func (t *ExtractTool) buildAnalysisResult(input *ExtractInput, fileType string, content []byte) map[string]interface{} {
	result := map[string]interface{}{
		"file_type":       fileType,
		"file_path":       input.InputPath,
		"file_size":       len(content),
		"schema":          input.Schema,
		"extraction_task": "extract_structured_data",
	}

	// Add content based on file type
	switch fileType {
	case "text":
		result["content"] = string(content)
		result["content_type"] = "text/plain"
	case "image":
		// For images, we'll include metadata and let the LLM handle the image
		result["content_type"] = "image"
		result["content_info"] = fmt.Sprintf("Image file with %d bytes", len(content))
		result["note"] = "Image content will be analyzed by the AI model"
	case "pdf":
		result["content_type"] = "application/pdf"
		result["content_info"] = fmt.Sprintf("PDF file with %d bytes", len(content))
		result["note"] = "PDF content will be extracted and analyzed by the AI model"
	default:
		result["content"] = string(content)
		result["content_type"] = "unknown"
	}

	// Add extraction guidelines
	guidelines := []string{
		"Extract data strictly according to the provided JSON schema",
		"Return valid JSON that conforms to the schema structure",
		"Use null values for fields that cannot be determined from the content",
		"Maintain data accuracy and avoid hallucination",
		"Preserve original data types as specified in the schema",
	}

	if input.BiasFilter != "" {
		guidelines = append(guidelines, fmt.Sprintf("Apply bias filtering: %s", input.BiasFilter))
		result["bias_filter"] = input.BiasFilter
	}

	if input.Instructions != "" {
		guidelines = append(guidelines, fmt.Sprintf("Additional instructions: %s", input.Instructions))
		result["custom_instructions"] = input.Instructions
	}

	result["extraction_guidelines"] = guidelines

	if input.OutputPath != "" {
		result["output_path"] = input.OutputPath
	}

	return result
}

func (t *ExtractTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "extract",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}
