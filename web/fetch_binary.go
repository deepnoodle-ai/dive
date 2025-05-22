package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// BinaryFetchInput contains parameters for fetching binary files
type BinaryFetchInput struct {
	URL            string            `json:"url"`
	Headers        map[string]string `json:"headers,omitempty"`
	OutputPath     string            `json:"output_path,omitempty"`
	CreateDirs     bool              `json:"create_dirs,omitempty"`
	MaxSizeBytes   int64             `json:"max_size_bytes,omitempty"`
	ExpectedType   string            `json:"expected_type,omitempty"` // Expected MIME type, e.g. "application/pdf", "image/jpeg"
	VerifyMimeType bool              `json:"verify_mime_type,omitempty"`
}

// BinaryFetchResult contains the result of a binary file fetch operation
type BinaryFetchResult struct {
	Filename     string
	Size         int64
	ContentType  string
	DownloadPath string
	Data         []byte // Only populated if no OutputPath is specified
}

// BinaryFetcher defines the interface for fetching binary files
type BinaryFetcher interface {
	FetchBinary(ctx context.Context, input *BinaryFetchInput) (*BinaryFetchResult, error)
}

// DefaultBinaryFetcher provides a standard implementation of BinaryFetcher
type DefaultBinaryFetcher struct {
	Client *http.Client
}

// NewDefaultBinaryFetcher creates a new binary fetcher with default HTTP client
func NewDefaultBinaryFetcher() *DefaultBinaryFetcher {
	return &DefaultBinaryFetcher{
		Client: &http.Client{},
	}
}

// FetchBinary downloads a binary file from the specified URL
func (f *DefaultBinaryFetcher) FetchBinary(ctx context.Context, input *BinaryFetchInput) (*BinaryFetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", input.URL, nil)
	if err != nil {
		return nil, err
	}

	// Add headers if specified
	for key, value := range input.Headers {
		req.Header.Add(key, value)
	}

	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, NewFetchError(resp.StatusCode, fmt.Errorf("failed to fetch binary from %s", input.URL))
	}

	contentType := resp.Header.Get("Content-Type")

	// Verify content type if requested
	if input.VerifyMimeType && input.ExpectedType != "" && contentType != input.ExpectedType {
		return nil, fmt.Errorf("content type mismatch: expected %s, got %s", input.ExpectedType, contentType)
	}

	// Get content length if available
	contentLength := resp.ContentLength

	// Check size limits if specified
	if input.MaxSizeBytes > 0 && contentLength > input.MaxSizeBytes {
		return nil, fmt.Errorf("file size exceeds maximum allowed size: %d > %d", contentLength, input.MaxSizeBytes)
	}

	// Determine filename from URL or Content-Disposition header
	filename := filenameFromResponse(resp)

	result := &BinaryFetchResult{
		Filename:    filename,
		ContentType: contentType,
		Size:        contentLength,
	}

	// If output path is specified, save to file
	if input.OutputPath != "" {
		outputPath := input.OutputPath

		// If output path is a directory, append the filename
		fileInfo, err := os.Stat(outputPath)
		if err == nil && fileInfo.IsDir() {
			outputPath = filepath.Join(outputPath, filename)
		}

		// Create directories if requested and needed
		if input.CreateDirs {
			dir := filepath.Dir(outputPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create directory structure: %w", err)
			}
		}

		// Create the file
		outputFile, err := os.Create(outputPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create output file: %w", err)
		}
		defer outputFile.Close()

		// Copy the response body to the file
		written, err := io.Copy(outputFile, resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to write file contents: %w", err)
		}

		result.Size = written
		result.DownloadPath = outputPath
	} else {
		// If no output path, read into memory
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		result.Data = data
		result.Size = int64(len(data))
	}

	return result, nil
}

// filenameFromResponse attempts to extract a filename from the response
func filenameFromResponse(resp *http.Response) string {
	// Try Content-Disposition header first
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if filename := extractFilenameFromContentDisposition(cd); filename != "" {
			return filename
		}
	}

	// Fall back to the URL path
	path := resp.Request.URL.Path
	return filepath.Base(path)
}

// extractFilenameFromContentDisposition extracts filename from Content-Disposition header
func extractFilenameFromContentDisposition(cd string) string {
	// Simple implementation - could be enhanced with full RFC parsing
	const filenamePrefix = "filename="
	if idx := strings.Index(cd, filenamePrefix); idx >= 0 {
		filename := cd[idx+len(filenamePrefix):]

		// Handle quoted filenames
		if len(filename) > 0 && (filename[0] == '"' || filename[0] == '\'') {
			quote := filename[0]
			if endIdx := strings.IndexByte(filename[1:], quote); endIdx >= 0 {
				return filename[1 : endIdx+1]
			}
		}

		// Handle unquoted filenames (ending at first semicolon or end of string)
		if endIdx := strings.IndexByte(filename, ';'); endIdx >= 0 {
			return filename[:endIdx]
		}

		return filename
	}

	return ""
}
