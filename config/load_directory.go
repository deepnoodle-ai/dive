package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

// LoadDirectory loads all YAML and JSON files from a directory and combines
// them into a single DiveConfig. Files are loaded in lexicographical order.
// Later files can override values from earlier files.
func LoadDirectory(dirPath string) (*Config, error) {
	// Read all files in the directory
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	// Collect all YAML and JSON files
	var configFiles []string
	for _, entry := range entries {
		if !entry.IsDir() {
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if ext == ".yml" || ext == ".yaml" || ext == ".json" {
				configFiles = append(configFiles, filepath.Join(dirPath, entry.Name()))
			}
		}
	}

	// Sort files for deterministic loading order
	sort.Strings(configFiles)

	// Consider an empty directory an error
	if len(configFiles) == 0 {
		return nil, fmt.Errorf("no yaml or json files found in directory: %s", dirPath)
	}

	// Merge all configuration files
	var merged *Config
	for _, file := range configFiles {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", file, err)
		}
		var config *Config
		ext := strings.ToLower(filepath.Ext(file))
		if ext == ".json" {
			config, err = ParseJSON(data)
		} else {
			config, err = ParseYAML(data)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to parse file %s: %w", file, err)
		}
		if merged == nil {
			merged = config
		} else {
			merged = Merge(merged, config)
		}
	}
	return merged, nil
}

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
