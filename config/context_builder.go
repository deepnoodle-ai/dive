package config

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/diveagents/dive"
	"github.com/diveagents/dive/environment"
	"github.com/diveagents/dive/llm"
)

// buildContextContent converts a list of context entries (string paths/URLs or
// objects with an "Inline" key) into []llm.Content that can be added to a chat.
func buildContextContent(ctx context.Context, repo dive.DocumentRepository, basePath string, entries []Content) ([]llm.Content, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	var contents []llm.Content
	for _, entry := range entries {
		// Determine which field is set and process accordingly
		var content llm.Content
		var err error

		// Count how many fields are set to ensure only one is specified
		fieldsSet := 0
		if entry.Text != "" {
			fieldsSet++
		}
		if entry.Path != "" {
			fieldsSet++
		}
		if entry.URL != "" {
			fieldsSet++
		}
		if entry.Document != "" {
			fieldsSet++
		}
		if entry.Script != "" {
			fieldsSet++
		}
		if entry.ScriptPath != "" {
			fieldsSet++
		}

		if fieldsSet == 0 {
			return nil, fmt.Errorf("context entry must specify exactly one of Text, Path, URL, Document, Script, or ScriptPath")
		}
		if fieldsSet > 1 {
			return nil, fmt.Errorf("context entry must specify exactly one of Text, Path, URL, Document, Script, or ScriptPath, but multiple were set")
		}

		switch {
		case entry.Text != "":
			contents = append(contents, &llm.TextContent{Text: strings.TrimSpace(entry.Text)})
		case entry.Path != "":
			resolvedPath := entry.Path
			if !filepath.IsAbs(entry.Path) && basePath != "" {
				resolvedPath = filepath.Join(basePath, entry.Path)
			}
			if containsWildcards(entry.Path) {
				// Handle wildcard pattern
				matches, err := doublestar.FilepathGlob(resolvedPath)
				if err != nil {
					return nil, fmt.Errorf("failed to expand wildcard pattern %s: %w", entry.Path, err)
				}
				// Create content for each matching file
				for _, match := range matches {
					fileContent, err := buildMessageFromLocalFile(match)
					if err != nil {
						return nil, fmt.Errorf("failed to process file %s from pattern %s: %w", match, entry.Path, err)
					}
					contents = append(contents, fileContent)
				}
			} else {
				// Handle single file path
				if _, statErr := os.Stat(resolvedPath); statErr != nil {
					return nil, fmt.Errorf("unable to read file %s: %w", resolvedPath, statErr)
				}
				content, err = buildMessageFromLocalFile(resolvedPath)
				if err != nil {
					return nil, err
				}
				contents = append(contents, content)
			}
		case entry.URL != "":
			content, err = buildMessageFromRemote(entry.URL)
			if err != nil {
				return nil, err
			}
			contents = append(contents, content)
		case entry.Document != "":
			content, err = buildMessageFromDocument(ctx, repo, entry.Document)
			if err != nil {
				return nil, err
			}
			contents = append(contents, content)
		case entry.Script != "":
			// Create RisorContent for dynamic script evaluation
			contents = append(contents, &environment.RisorContent{
				Script:   entry.Script,
				BasePath: basePath,
			})
		case entry.ScriptPath != "":
			// Create ScriptPathContent for dynamic script evaluation
			contents = append(contents, &environment.ScriptPathContent{
				ScriptPath: entry.ScriptPath,
				BasePath:   basePath,
			})
		}
	}
	return contents, nil
}

func buildMessageFromRemote(remote string) (llm.Content, error) {
	if !strings.HasPrefix(remote, "http") {
		return nil, fmt.Errorf("remote content url must start with http:// or https://")
	}
	ext := strings.ToLower(filepath.Ext(remote))

	// Handle known binary formats first
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return &llm.ImageContent{
			Source: &llm.ContentSource{
				Type:      llm.ContentSourceTypeURL,
				MediaType: "image/" + ext[1:], // Remove the dot
				URL:       remote,
			},
		}, nil
	case ".pdf":
		return &llm.DocumentContent{
			Source: &llm.ContentSource{
				Type:      llm.ContentSourceTypeURL,
				MediaType: "application/pdf",
				URL:       remote,
			},
		}, nil
	}

	// For all other URLs, attempt to download and check if textual
	resp, err := http.Get(remote)
	if err != nil {
		return nil, fmt.Errorf("failed to download content from %s: %w", remote, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download content from %s: HTTP %d", remote, resp.StatusCode)
	}

	// Check Content-Type to see if it's textual
	contentType := resp.Header.Get("Content-Type")
	isTextual := strings.HasPrefix(contentType, "text/") ||
		strings.Contains(contentType, "json") ||
		strings.Contains(contentType, "xml") ||
		strings.Contains(contentType, "javascript") ||
		strings.Contains(contentType, "css") ||
		contentType == "" // Some servers don't set Content-Type

	if !isTextual {
		return nil, fmt.Errorf("remote content at %s is not textual (Content-Type: %s)", remote, contentType)
	}

	// Limit download to 500 KB
	const maxSize = 500 * 1024                            // 500 KB
	limitedReader := io.LimitReader(resp.Body, maxSize+1) // +1 to detect if we exceed the limit

	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read content from %s: %w", remote, err)
	}

	if len(data) > maxSize {
		return nil, fmt.Errorf("remote content at %s exceeds 500 KB limit", remote)
	}

	// Extract filename from URL for wrapping
	parsedURL, err := url.Parse(remote)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL %s: %w", remote, err)
	}
	filename := filepath.Base(parsedURL.Path)
	if filename == "" || filename == "." {
		filename = "remote_content"
		if ext != "" {
			filename += ext
		}
	}

	return &llm.TextContent{
		Text: wrapRemoteContextText(remote, string(data)),
	}, nil
}

func buildMessageFromDocument(ctx context.Context, repo dive.DocumentRepository, document string) (llm.Content, error) {
	doc, err := repo.GetDocument(ctx, document)
	if err != nil {
		return nil, err
	}
	return &llm.TextContent{Text: doc.Content()}, nil
}

func buildMessageFromLocalFile(path string) (llm.Content, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case isImageExt(ext):
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		encodedData := base64.StdEncoding.EncodeToString(data)
		return &llm.ImageContent{
			Source: &llm.ContentSource{
				Type:      llm.ContentSourceTypeBase64,
				MediaType: "image/" + ext[1:], // Remove the dot
				Data:      encodedData,
			},
		}, nil
	case ext == ".pdf":
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		encodedData := base64.StdEncoding.EncodeToString(data)
		return &llm.DocumentContent{
			Source: &llm.ContentSource{
				Type:      llm.ContentSourceTypeBase64,
				MediaType: "application/pdf",
				Data:      encodedData,
			},
		}, nil
	case ext == ".txt" || ext == ".md" || ext == ".markdown" || ext == ".csv" || ext == ".json":
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return &llm.TextContent{
			Text: wrapContextText(filepath.Base(path), string(data)),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported local file extension: %s", ext)
	}
}

func fileURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path // fallback to given
	}
	return (&url.URL{Scheme: "file", Path: abs}).String()
}

func isImageExt(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return true
	default:
		return false
	}
}

func wrapContextText(name, text string) string {
	return fmt.Sprintf("<file name=%q>\n%s\n</file>", name, text)
}

func wrapRemoteContextText(url, text string) string {
	return fmt.Sprintf("<file url=%q>\n%s\n</file>", url, text)
}

// containsWildcards checks if a path contains wildcard characters
func containsWildcards(path string) bool {
	return strings.ContainsAny(path, "*?[{")
}
