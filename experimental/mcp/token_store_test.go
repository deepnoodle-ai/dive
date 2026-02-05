package mcp

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestNewFileOAuthTokenStore(t *testing.T) {
	t.Run("creates store with valid path", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "tokens", "oauth.json")

		store, err := NewFileOAuthTokenStore(filePath)
		assert.NoError(t, err)
		assert.NotNil(t, store)
		assert.Equal(t, filePath, store.filePath)

		// Check that directory was created
		dirPath := filepath.Dir(filePath)
		info, err := os.Stat(dirPath)
		assert.NoError(t, err, "directory should exist")
		assert.True(t, info.IsDir(), "path should be a directory")
	})

	t.Run("fails with empty path", func(t *testing.T) {
		store, err := NewFileOAuthTokenStore("")
		assert.Error(t, err)
		assert.Nil(t, store)
		assert.Contains(t, err.Error(), "file path cannot be empty")
	})
}

func TestFileOAuthTokenStore_SaveAndGetToken(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "oauth.json")

	store, err := NewFileOAuthTokenStore(filePath)
	assert.NoError(t, err)

	t.Run("saves and retrieves token successfully", func(t *testing.T) {
		expiresAt := time.Now().Add(time.Hour)
		token := &Token{
			AccessToken:  "access123",
			RefreshToken: "refresh456",
			TokenType:    "Bearer",
			ExpiresAt:    expiresAt,
			Scope:        "read write",
		}

		// Save token
		err := store.SaveToken(token)
		assert.NoError(t, err)

		// Retrieve token
		retrieved, err := store.GetToken()
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, token.AccessToken, retrieved.AccessToken)
		assert.Equal(t, token.RefreshToken, retrieved.RefreshToken)
		assert.Equal(t, token.TokenType, retrieved.TokenType)
		assert.Equal(t, token.Scope, retrieved.Scope)
		// Time comparison with some tolerance due to JSON marshaling precision
		timeDiff := token.ExpiresAt.Sub(retrieved.ExpiresAt)
		if timeDiff < 0 {
			timeDiff = -timeDiff
		}
		assert.True(t, timeDiff <= time.Second, "ExpiresAt times should be within 1 second")
	})

	t.Run("overwrites existing token", func(t *testing.T) {
		newToken := &Token{
			AccessToken:  "newaccess789",
			RefreshToken: "newrefresh012",
			TokenType:    "Bearer",
			ExpiresAt:    time.Now().Add(2 * time.Hour),
			Scope:        "admin",
		}

		// Save new token (should overwrite existing)
		err := store.SaveToken(newToken)
		assert.NoError(t, err)

		// Retrieve token
		retrieved, err := store.GetToken()
		assert.NoError(t, err)
		assert.Equal(t, newToken.AccessToken, retrieved.AccessToken)
		assert.Equal(t, newToken.RefreshToken, retrieved.RefreshToken)
		assert.Equal(t, newToken.Scope, retrieved.Scope)
	})

	t.Run("fails to save nil token", func(t *testing.T) {
		err := store.SaveToken(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token cannot be nil")
	})
}

func TestFileOAuthTokenStore_GetToken_NoFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "nonexistent.json")

	store, err := NewFileOAuthTokenStore(filePath)
	assert.NoError(t, err)

	// Try to get token when file doesn't exist
	token, err := store.GetToken()
	assert.Error(t, err)
	assert.Nil(t, token)
	assert.Contains(t, err.Error(), "no token found")
}

func TestFileOAuthTokenStore_GetToken_CorruptedFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "corrupted.json")

	store, err := NewFileOAuthTokenStore(filePath)
	assert.NoError(t, err)

	// Write invalid JSON to file
	err = os.WriteFile(filePath, []byte("invalid json content"), 0600)
	assert.NoError(t, err)

	// Try to get token from corrupted file
	token, err := store.GetToken()
	assert.Error(t, err)
	assert.Nil(t, token)
	assert.Contains(t, err.Error(), "failed to parse token file")
}

func TestFileOAuthTokenStore_HasToken(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "oauth.json")

	store, err := NewFileOAuthTokenStore(filePath)
	assert.NoError(t, err)

	t.Run("returns false when no token exists", func(t *testing.T) {
		has := store.HasToken()
		assert.False(t, has)
	})

	t.Run("returns true when token exists", func(t *testing.T) {
		token := &Token{
			AccessToken: "test123",
			TokenType:   "Bearer",
		}

		err := store.SaveToken(token)
		assert.NoError(t, err)

		has := store.HasToken()
		assert.True(t, has)
	})
}

func TestFileOAuthTokenStore_DeleteToken(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "oauth.json")

	store, err := NewFileOAuthTokenStore(filePath)
	assert.NoError(t, err)

	t.Run("deletes existing token", func(t *testing.T) {
		// First save a token
		token := &Token{
			AccessToken: "test123",
			TokenType:   "Bearer",
		}
		err := store.SaveToken(token)
		assert.NoError(t, err)
		assert.True(t, store.HasToken())

		// Delete the token
		err = store.DeleteToken()
		assert.NoError(t, err)
		assert.False(t, store.HasToken())

		// Verify we can't get the token anymore
		_, err = store.GetToken()
		assert.Error(t, err)
	})

	t.Run("succeeds when no token exists", func(t *testing.T) {
		// Delete non-existent token should not error
		err := store.DeleteToken()
		assert.NoError(t, err)
	})
}

func TestToken_IsExpired(t *testing.T) {
	t.Run("returns false for token with no expiration", func(t *testing.T) {
		token := &Token{
			AccessToken: "test123",
			// ExpiresAt is zero value
		}
		assert.False(t, token.IsExpired())
	})

	t.Run("returns false for token not yet expired", func(t *testing.T) {
		token := &Token{
			AccessToken: "test123",
			ExpiresAt:   time.Now().Add(time.Hour),
		}
		assert.False(t, token.IsExpired())
	})

	t.Run("returns true for expired token", func(t *testing.T) {
		token := &Token{
			AccessToken: "test123",
			ExpiresAt:   time.Now().Add(-time.Hour),
		}
		assert.True(t, token.IsExpired())
	})
}

func TestFileOAuthTokenStore_Concurrency(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "concurrent.json")

	store, err := NewFileOAuthTokenStore(filePath)
	assert.NoError(t, err)

	// Test concurrent reads and writes
	done := make(chan bool, 2)

	// Goroutine 1: Write tokens
	go func() {
		defer func() { done <- true }()
		for i := 0; i < 100; i++ {
			token := &Token{
				AccessToken: "concurrent_token",
				TokenType:   "Bearer",
			}
			store.SaveToken(token)
		}
	}()

	// Goroutine 2: Read tokens
	go func() {
		defer func() { done <- true }()
		for i := 0; i < 100; i++ {
			store.GetToken() // May error if file doesn't exist yet, that's ok
			store.HasToken()
		}
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// Final verification that we can still read a token
	token := &Token{
		AccessToken: "final_token",
		TokenType:   "Bearer",
	}
	err = store.SaveToken(token)
	assert.NoError(t, err)

	retrieved, err := store.GetToken()
	assert.NoError(t, err)
	assert.Equal(t, "final_token", retrieved.AccessToken)
}

func TestFileOAuthTokenStore_AtomicWrites(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "atomic.json")

	store, err := NewFileOAuthTokenStore(filePath)
	assert.NoError(t, err)

	// Save a token
	token := &Token{
		AccessToken: "test123",
		TokenType:   "Bearer",
	}
	err = store.SaveToken(token)
	assert.NoError(t, err)

	// Verify no temporary files are left behind
	files, err := os.ReadDir(tempDir)
	assert.NoError(t, err)

	var tmpFiles []string
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".tmp" {
			tmpFiles = append(tmpFiles, file.Name())
		}
	}
	assert.Empty(t, tmpFiles, "Should not leave temporary files behind")
}
