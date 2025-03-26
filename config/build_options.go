package config

import (
	"github.com/diveagents/dive"
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/slogger"
)

type BuildOptions struct {
	Variables     map[string]interface{}
	Tools         map[string]llm.Tool
	Logger        slogger.Logger
	LogLevel      string
	DocumentsDir  string
	DocumentsRepo dive.DocumentRepository
	ThreadRepo    dive.ThreadRepository
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

func WithDocumentsRepo(repo dive.DocumentRepository) BuildOption {
	return func(opts *BuildOptions) {
		opts.DocumentsRepo = repo
	}
}

func WithThreadRepo(repo dive.ThreadRepository) BuildOption {
	return func(opts *BuildOptions) {
		opts.ThreadRepo = repo
	}
}
