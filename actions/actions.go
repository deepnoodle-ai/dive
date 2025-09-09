package actions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var (
	actionsRegistry = make(map[string]Action)
)

// RegisterAction adds a new action for use in workflows
func RegisterAction(action Action) {
	actionsRegistry[action.Name()] = action
}

func init() {
	RegisterAction(NewGetTimeAction())
	RegisterAction(NewPrintAction())
}

// Action represents a named action that can be executed as part of a workflow
type Action interface {
	Name() string
	Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

// FileWriteAction implements writing to files directly
type FileWriteAction struct {
	baseDir string
}

func NewFileWriteAction(baseDir string) *FileWriteAction {
	if baseDir == "" {
		baseDir = "."
	}
	return &FileWriteAction{baseDir: baseDir}
}

func (a *FileWriteAction) Name() string {
	return "File.Write"
}

func (a *FileWriteAction) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	path, ok := params["Path"].(string)
	if !ok {
		return nil, errors.New("path parameter must be a string")
	}
	content, ok := params["Content"].(string)
	if !ok {
		return nil, errors.New("content parameter must be a string")
	}

	// Resolve path relative to base directory
	fullPath := path
	if !filepath.IsAbs(path) {
		fullPath = filepath.Join(a.baseDir, path)
	}

	// Ensure directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}
	return nil, nil
}

// FileReadAction implements reading from files directly
type FileReadAction struct {
	baseDir string
}

func NewFileReadAction(baseDir string) *FileReadAction {
	if baseDir == "" {
		baseDir = "."
	}
	return &FileReadAction{baseDir: baseDir}
}

func (a *FileReadAction) Name() string {
	return "File.Read"
}

func (a *FileReadAction) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	path, ok := params["Path"].(string)
	if !ok {
		return nil, fmt.Errorf("path parameter must be a string")
	}

	// Resolve path relative to base directory
	fullPath := path
	if !filepath.IsAbs(path) {
		fullPath = filepath.Join(a.baseDir, path)
	}

	// Read file
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return string(content), nil
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

type PrintAction struct {
}

func NewPrintAction() *PrintAction {
	return &PrintAction{}
}

func (a *PrintAction) Name() string {
	return "Print"
}

func (a *PrintAction) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	message, ok := params["Message"].(string)
	if !ok {
		return nil, errors.New("message parameter must be a string")
	}
	fmt.Println(message)
	return nil, nil
}