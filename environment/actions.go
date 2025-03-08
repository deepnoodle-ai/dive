package environment

import (
	"context"
	"errors"
	"fmt"

	"github.com/getstingrai/dive/document"
)

// Action represents a named action that can be executed as part of a workflow
type Action interface {
	Name() string
	Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

// DocumentWriteAction implements writing to the document repository
type DocumentWriteAction struct {
	repo document.Repository
}

func NewDocumentWriteAction(repo document.Repository) *DocumentWriteAction {
	return &DocumentWriteAction{repo: repo}
}

func (a *DocumentWriteAction) Name() string {
	return "Document.Write"
}

func (a *DocumentWriteAction) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	path, ok := params["Path"].(string)
	if !ok {
		return nil, errors.New("path parameter must be a string")
	}
	content, ok := params["Content"].(string)
	if !ok {
		return nil, errors.New("content parameter must be a string")
	}
	doc := document.New(document.Options{
		Path:    path,
		Content: content,
	})
	if err := a.repo.PutDocument(ctx, doc); err != nil {
		return nil, fmt.Errorf("failed to write document: %w", err)
	}
	return nil, nil
}

// DocumentReadAction implements reading from the document repository
type DocumentReadAction struct {
	repo document.Repository
}

func NewDocumentReadAction(repo document.Repository) *DocumentReadAction {
	return &DocumentReadAction{repo: repo}
}

func (a *DocumentReadAction) Name() string {
	return "Document.Read"
}

func (a *DocumentReadAction) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	path, ok := params["Path"].(string)
	if !ok {
		return nil, fmt.Errorf("path parameter must be a string")
	}

	doc, err := a.repo.GetDocument(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to read document: %w", err)
	}
	return doc.Content(), nil
}
