package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Token represents an OAuth 2.0 token with all relevant fields
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	Scope        string    `json:"scope,omitempty"`
}

// IsExpired checks if the token has expired
func (t *Token) IsExpired() bool {
	if t.ExpiresAt.IsZero() {
		return false // No expiration time set
	}
	return time.Now().After(t.ExpiresAt)
}

// OAuthTokenStore is an interface for storing and retrieving OAuth tokens
type OAuthTokenStore interface {
	// GetToken returns the current token
	GetToken() (*Token, error)
	// SaveToken saves a token
	SaveToken(token *Token) error
}

// FileOAuthTokenStore implements OAuthTokenStore interface using file-based persistence
type FileOAuthTokenStore struct {
	filePath string
	mutex    sync.RWMutex
}

// NewFileOAuthTokenStore creates a new file-based OAuth token store
func NewFileOAuthTokenStore(filePath string) (*FileOAuthTokenStore, error) {
	if filePath == "" {
		return nil, fmt.Errorf("file path cannot be empty")
	}

	// Ensure the directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	return &FileOAuthTokenStore{
		filePath: filePath,
	}, nil
}

// GetToken retrieves the token from the file
func (fs *FileOAuthTokenStore) GetToken() (*Token, error) {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	// Check if file exists
	if _, err := os.Stat(fs.filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("no token found")
	}

	// Read file content
	data, err := os.ReadFile(fs.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read token file: %w", err)
	}

	// Parse JSON
	var token Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("failed to parse token file: %w", err)
	}

	return &token, nil
}

// SaveToken saves the token to the file
func (fs *FileOAuthTokenStore) SaveToken(token *Token) error {
	if token == nil {
		return fmt.Errorf("token cannot be nil")
	}

	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	// Marshal token to JSON
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	// Write to temporary file first for atomic operation
	tempFile := fs.filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write token file: %w", err)
	}

	// Atomically rename temporary file to target file
	if err := os.Rename(tempFile, fs.filePath); err != nil {
		os.Remove(tempFile) // Clean up on failure
		return fmt.Errorf("failed to save token file: %w", err)
	}

	return nil
}

// DeleteToken removes the token file
func (fs *FileOAuthTokenStore) DeleteToken() error {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()

	if err := os.Remove(fs.filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete token file: %w", err)
	}

	return nil
}

// HasToken checks if a token exists in the store
func (fs *FileOAuthTokenStore) HasToken() bool {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()

	_, err := os.Stat(fs.filePath)
	return err == nil
}
