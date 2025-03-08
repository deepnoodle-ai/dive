package config

import (
	"github.com/getstingrai/dive/document"
	"github.com/getstingrai/dive/llm"
	"github.com/getstingrai/dive/slogger"
)

type BuildOptions struct {
	Variables     map[string]interface{}
	Tools         map[string]llm.Tool
	Logger        slogger.Logger
	LogLevel      string
	DocumentsDir  string
	DocumentsRepo document.Repository
}

type BuildOption func(*BuildOptions)

func WithVariables(vars map[string]interface{}) BuildOption {
	return func(opts *BuildOptions) {
		opts.Variables = vars
	}
}

func WithTools(tools map[string]llm.Tool) BuildOption {
	return func(opts *BuildOptions) {
		opts.Tools = tools
	}
}

func WithLogger(logger slogger.Logger) BuildOption {
	return func(opts *BuildOptions) {
		opts.Logger = logger
	}
}

func WithDocumentsDir(dir string) BuildOption {
	return func(opts *BuildOptions) {
		opts.DocumentsDir = dir
	}
}

func WithDocumentsRepo(repo document.Repository) BuildOption {
	return func(opts *BuildOptions) {
		opts.DocumentsRepo = repo
	}
}
