package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseFile(t *testing.T) {
	// Create temporary test files
	tmpDir := t.TempDir()

	// Test YAML file
	yamlContent := `
Name: test-env
Description: Test Environment
`
	yamlFile := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(yamlFile, []byte(yamlContent), 0644)
	assert.NoError(t, err)

	// Test JSON file
	jsonContent := `{
		"Name": "test-env",
		"Description": "Test Environment"
	}`
	jsonFile := filepath.Join(tmpDir, "test.json")
	err = os.WriteFile(jsonFile, []byte(jsonContent), 0644)
	assert.NoError(t, err)

	// Test invalid extension
	invalidFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(invalidFile, []byte("test"), 0644)
	assert.NoError(t, err)

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "parse yaml file",
			path:    yamlFile,
			wantErr: false,
		},
		{
			name:    "parse json file",
			path:    jsonFile,
			wantErr: false,
		},
		{
			name:    "invalid file extension",
			path:    invalidFile,
			wantErr: true,
		},
		{
			name:    "non-existent file",
			path:    "nonexistent.yaml",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := ParseFile(tt.path)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, env)
			assert.Equal(t, "test-env", env.Name)
			assert.Equal(t, "Test Environment", env.Description)
		})
	}
}

func TestParseYAML(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name: "valid yaml",
			data: []byte(`
Name: test-env
Description: Test Environment
`),
			wantErr: false,
		},
		{
			name:    "invalid yaml",
			data:    []byte(`invalid: yaml: content:`),
			wantErr: true,
		},
		{
			name:    "empty yaml",
			data:    []byte(``),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := ParseYAML(tt.data)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, env)
			if len(tt.data) > 0 {
				assert.Equal(t, "test-env", env.Name)
				assert.Equal(t, "Test Environment", env.Description)
			}
		})
	}
}

func TestParseJSON(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name: "valid json",
			data: []byte(`{
				"name": "test-env",
				"description": "Test Environment"
			}`),
			wantErr: false,
		},
		{
			name:    "invalid json",
			data:    []byte(`{invalid json}`),
			wantErr: true,
		},
		{
			name:    "empty json",
			data:    []byte(`{}`),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := ParseJSON(tt.data)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, env)
			if len(tt.data) > 0 && string(tt.data) != "{}" {
				assert.Equal(t, "test-env", env.Name)
				assert.Equal(t, "Test Environment", env.Description)
			}
		})
	}
}

func TestLoadDirectory(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()

	// Create multiple config files
	files := []struct {
		name    string
		content string
	}{
		{
			name: "01-base.yaml",
			content: `
Name: test-env
Description: Base Environment
`,
		},
		{
			name: "02-override.yaml",
			content: `
Name: test-env2
Description: Override 1
`,
		},
		{
			name: "03-additional.json",
			content: `{
"Name": "test-env3",
"Description": "Override 2"
			}`,
		},
	}

	for _, f := range files {
		err := os.WriteFile(filepath.Join(tmpDir, f.name), []byte(f.content), 0644)
		assert.NoError(t, err)
	}

	tests := []struct {
		name    string
		dir     string
		wantErr bool
	}{
		{
			name:    "load valid directory",
			dir:     tmpDir,
			wantErr: false,
		},
		{
			name:    "empty directory",
			dir:     t.TempDir(),
			wantErr: true,
		},
		{
			name:    "non-existent directory",
			dir:     "nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := LoadDirectory(tt.dir)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, env)
			// The last file (JSON) should override previous values
			assert.Equal(t, "test-env3", env.Name())
			assert.Equal(t, "Override 2", env.Description())
		})
	}
}
