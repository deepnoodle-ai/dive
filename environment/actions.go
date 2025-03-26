package environment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/diveagents/dive"
)

// Action represents a named action that can be executed as part of a workflow
type Action interface {
	Name() string
	Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

// DocumentWriteAction implements writing to the document repository
type DocumentWriteAction struct {
	repo dive.DocumentRepository
}

func NewDocumentWriteAction(repo dive.DocumentRepository) *DocumentWriteAction {
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
	doc := dive.NewTextDocument(dive.TextDocumentOptions{
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
	repo dive.DocumentRepository
}

func NewDocumentReadAction(repo dive.DocumentRepository) *DocumentReadAction {
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

// GetTimeAction implements getting the current time
type GetTimeAction struct {
}

func NewGetTimeAction() *GetTimeAction {
	return &GetTimeAction{}
}

func (a *GetTimeAction) Name() string {
	return "Time.Now"
}

func (a *GetTimeAction) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	return time.Now().Format(time.RFC3339), nil
}
