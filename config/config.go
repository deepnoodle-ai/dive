package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

// Save writes a DiveConfig to a file. The file extension is used to
// determine the configuration format:
// - .json -> JSON
// - .yml or .yaml -> YAML
func (config *Config) Save(path string) error {
	// Determine format from extension
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return config.SaveJSON(path)
	case ".yml", ".yaml":
		return config.SaveYAML(path)
	default:
		return fmt.Errorf("unsupported file extension: %s", ext)
	}
}

// SaveYAML writes a DiveConfig to a YAML file
func (config *Config) SaveYAML(path string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// SaveJSON writes a DiveConfig to a JSON file
func (config *Config) SaveJSON(path string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Write a DiveConfig to a writer in YAML format
func (config *Config) Write(w io.Writer) error {
	return yaml.NewEncoder(w).Encode(config)
}

// GetMCPServers returns the MCP server configurations
func (config *Config) GetMCPServers() []MCPServer {
	return config.MCPServers
}
