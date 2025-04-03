package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

// ParseFile loads an Environment configuration from a file. The file extension
// is used to determine the configuration format (JSON or YAML).
func ParseFile(path string) (*Environment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return ParseJSON(data)
	case ".yml", ".yaml":
		return ParseYAML(data)
	default:
		return nil, fmt.Errorf("unsupported file extension: %s", ext)
	}
}

// ParseYAML loads an Environment configuration from YAML
func ParseYAML(data []byte) (*Environment, error) {
	var env Environment
	if err := yaml.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	return &env, nil
}

// ParseJSON loads an Environment configuration from JSON
func ParseJSON(data []byte) (*Environment, error) {
	var env Environment
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	return &env, nil
}
