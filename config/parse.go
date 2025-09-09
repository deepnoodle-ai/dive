package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

// ParseFile loads a DiveConfig from a file. The file extension
// is used to determine the configuration format (JSON or YAML).
func ParseFile(path string) (*Config, error) {
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

// ParseYAML loads a DiveConfig from YAML
func ParseYAML(data []byte) (*Config, error) {
	var config Config
	if err := yaml.UnmarshalWithOptions(data, &config, yaml.Strict()); err != nil {
		return nil, err
	}
	return &config, nil
}

// ParseJSON loads a DiveConfig from JSON
func ParseJSON(data []byte) (*Config, error) {
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}
