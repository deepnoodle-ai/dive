package mcp

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/diveagents/dive"
	"github.com/mark3labs/mcp-go/mcp"
)

// ResourceRepository implements Dive's DocumentRepository interface for MCP resources
type ResourceRepository struct {
	client     *Client
	serverName string
}

// NewResourceRepository creates a new MCP resource repository
func NewResourceRepository(client *Client, serverName string) *ResourceRepository {
	return &ResourceRepository{
		client:     client,
		serverName: serverName,
	}
}

// GetDocument returns a document by name (URI for MCP resources)
func (r *ResourceRepository) GetDocument(ctx context.Context, name string) (dive.Document, error) {
	if !r.client.IsConnected() {
		return nil, NewMCPError("get_document", r.serverName, ErrNotConnected)
	}
	resource, err := r.client.ReadResource(ctx, name)
	if err != nil {
		return nil, NewMCPError("get_document", r.serverName, err)
	}
	return r.convertMCPResourceToDocument(resource)
}

// ListDocuments lists documents matching the input criteria
func (r *ResourceRepository) ListDocuments(ctx context.Context, input *dive.ListDocumentInput) (*dive.ListDocumentOutput, error) {
	if !r.client.IsConnected() {
		return nil, NewMCPError("list_documents", r.serverName, ErrNotConnected)
	}
	resources, err := r.client.ListResources(ctx)
	if err != nil {
		return nil, NewMCPError("list_documents", r.serverName, err)
	}

	var documents []dive.Document
	for _, resource := range resources {
		// Apply path prefix filter if specified
		if input != nil && input.PathPrefix != "" {
			if !strings.HasPrefix(resource.URI, input.PathPrefix) {
				continue
			}
		}
		doc := r.convertMCPResourceMetadataToDocument(resource)
		documents = append(documents, doc)
	}
	return &dive.ListDocumentOutput{Items: documents}, nil
}

// PutDocument is not supported for MCP resources (read-only)
func (r *ResourceRepository) PutDocument(ctx context.Context, doc dive.Document) error {
	return NewMCPError("put_document", r.serverName, ErrUnsupportedOperation)
}

// DeleteDocument is not supported for MCP resources (read-only)
func (r *ResourceRepository) DeleteDocument(ctx context.Context, doc dive.Document) error {
	return NewMCPError("delete_document", r.serverName, ErrUnsupportedOperation)
}

// Exists checks if a resource exists by URI
func (r *ResourceRepository) Exists(ctx context.Context, name string) (bool, error) {
	if !r.client.IsConnected() {
		return false, NewMCPError("exists", r.serverName, ErrNotConnected)
	}
	resources, err := r.client.ListResources(ctx)
	if err != nil {
		return false, NewMCPError("exists", r.serverName, err)
	}
	for _, resource := range resources {
		if resource.URI == name {
			return true, nil
		}
	}
	return false, nil
}

// RegisterDocument is not supported for MCP resources
func (r *ResourceRepository) RegisterDocument(ctx context.Context, name, path string) error {
	return NewMCPError("register_document", r.serverName, ErrUnsupportedOperation)
}

// convertMCPResourceToDocument converts an MCP resource with content to a Dive document
func (r *ResourceRepository) convertMCPResourceToDocument(resource *mcp.ReadResourceResult) (dive.Document, error) {
	var content string
	var contentType string
	var uri string

	for _, resourceContent := range resource.Contents {
		switch rc := resourceContent.(type) {
		case mcp.TextResourceContents:
			content = rc.Text
			uri = rc.URI
			if rc.MIMEType != "" {
				contentType = rc.MIMEType
			} else {
				contentType = "text/plain"
			}
		case mcp.BlobResourceContents:
			// For binary resources, we can't provide the content directly
			// Instead, provide a description
			uri = rc.URI
			content = fmt.Sprintf("Binary resource: %s", rc.URI)
			if rc.MIMEType != "" {
				contentType = rc.MIMEType
			} else {
				contentType = "application/octet-stream"
			}
		default:
			return nil, fmt.Errorf("unsupported resource type: %T", rc)
		}
		break // Use the first content item
	}

	return dive.NewTextDocument(dive.TextDocumentOptions{
		ID:          fmt.Sprintf("mcp://%s/%s", r.serverName, uri),
		Name:        filepath.Base(uri),
		Description: "MCP Resource", // We don't have description from ReadResourceResult
		Path:        uri,
		Content:     content,
		ContentType: contentType,
	}), nil
}

// convertMCPResourceMetadataToDocument converts MCP resource metadata to a lightweight Dive document
func (r *ResourceRepository) convertMCPResourceMetadataToDocument(resource mcp.Resource) dive.Document {
	return dive.NewTextDocument(dive.TextDocumentOptions{
		ID:          fmt.Sprintf("mcp://%s/%s", r.serverName, resource.URI),
		Name:        filepath.Base(resource.URI),
		Description: resource.Description,
		Path:        resource.URI,
		Content:     "", // Content not loaded for listing
		ContentType: resource.MIMEType,
	})
}
