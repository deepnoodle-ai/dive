package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/deepnoodle-ai/dive"
)

// LoadDirectory loads all YAML and JSON files from a directory and combines
// them into a single DiveConfig. Files are loaded in lexicographical order.
// Later files can override values from earlier files.
func LoadDirectory(dirPath string, opts ...BuildOption) ([]dive.Agent, error) {
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
	var merged *DiveConfig
	for _, file := range configFiles {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", file, err)
		}

		var config *DiveConfig
		ext := strings.ToLower(filepath.Ext(file))
		if ext == ".json" {
			config, err = ParseJSON(data)
		} else {
			config, err = ParseYAML(data)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to parse file %s: %w", file, err)
		}

		for i := range config.Workflows {
			config.Workflows[i].Path = file
		}

		if merged == nil {
			merged = config
		} else {
			merged = Merge(merged, config)
		}
	}

	return merged.BuildAgents(opts...)
}
