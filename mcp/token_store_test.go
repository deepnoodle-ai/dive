package mcp

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewFileOAuthTokenStore(t *testing.T) {
	t.Run("creates store with valid path", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "tokens", "oauth.json")

		store, err := NewFileOAuthTokenStore(filePath)
		require.NoError(t, err)
		require.NotNil(t, store)
		require.Equal(t, filePath, store.filePath)

		// Check that directory was created
		require.DirExists(t, filepath.Dir(filePath))
	})

	t.Run("fails with empty path", func(t *testing.T) {
		store, err := NewFileOAuthTokenStore("")
		require.Error(t, err)
		require.Nil(t, store)
		require.Contains(t, err.Error(), "file path cannot be empty")
	})
}

func TestFileOAuthTokenStore_SaveAndGetToken(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "oauth.json")

	store, err := NewFileOAuthTokenStore(filePath)
	require.NoError(t, err)

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
		require.NoError(t, err)

		// Retrieve token
		retrieved, err := store.GetToken()
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		require.Equal(t, token.AccessToken, retrieved.AccessToken)
		require.Equal(t, token.RefreshToken, retrieved.RefreshToken)
		require.Equal(t, token.TokenType, retrieved.TokenType)
		require.Equal(t, token.Scope, retrieved.Scope)
		// Time comparison with some tolerance due to JSON marshaling precision
		require.WithinDuration(t, token.ExpiresAt, retrieved.ExpiresAt, time.Second)
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
		require.NoError(t, err)

		// Retrieve token
		retrieved, err := store.GetToken()
		require.NoError(t, err)
		require.Equal(t, newToken.AccessToken, retrieved.AccessToken)
		require.Equal(t, newToken.RefreshToken, retrieved.RefreshToken)
		require.Equal(t, newToken.Scope, retrieved.Scope)
	})

	t.Run("fails to save nil token", func(t *testing.T) {
		err := store.SaveToken(nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "token cannot be nil")
	})
}

func TestFileOAuthTokenStore_GetToken_NoFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "nonexistent.json")

	store, err := NewFileOAuthTokenStore(filePath)
	require.NoError(t, err)

	// Try to get token when file doesn't exist
	token, err := store.GetToken()
	require.Error(t, err)
	require.Nil(t, token)
	require.Contains(t, err.Error(), "no token found")
}

func TestFileOAuthTokenStore_GetToken_CorruptedFile(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "corrupted.json")

	store, err := NewFileOAuthTokenStore(filePath)
	require.NoError(t, err)

	// Write invalid JSON to file
	err = os.WriteFile(filePath, []byte("invalid json content"), 0600)
	require.NoError(t, err)

	// Try to get token from corrupted file
	token, err := store.GetToken()
	require.Error(t, err)
	require.Nil(t, token)
	require.Contains(t, err.Error(), "failed to parse token file")
}

func TestFileOAuthTokenStore_HasToken(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "oauth.json")

	store, err := NewFileOAuthTokenStore(filePath)
	require.NoError(t, err)

	t.Run("returns false when no token exists", func(t *testing.T) {
		has := store.HasToken()
		require.False(t, has)
	})

	t.Run("returns true when token exists", func(t *testing.T) {
		token := &Token{
			AccessToken: "test123",
			TokenType:   "Bearer",
		}

		err := store.SaveToken(token)
		require.NoError(t, err)

		has := store.HasToken()
		require.True(t, has)
	})
}

func TestFileOAuthTokenStore_DeleteToken(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "oauth.json")

	store, err := NewFileOAuthTokenStore(filePath)
	require.NoError(t, err)

	t.Run("deletes existing token", func(t *testing.T) {
		// First save a token
		token := &Token{
			AccessToken: "test123",
			TokenType:   "Bearer",
		}
		err := store.SaveToken(token)
		require.NoError(t, err)
		require.True(t, store.HasToken())

		// Delete the token
		err = store.DeleteToken()
		require.NoError(t, err)
		require.False(t, store.HasToken())

		// Verify we can't get the token anymore
		_, err = store.GetToken()
		require.Error(t, err)
	})

	t.Run("succeeds when no token exists", func(t *testing.T) {
		// Delete non-existent token should not error
		err := store.DeleteToken()
		require.NoError(t, err)
	})
}

func TestToken_IsExpired(t *testing.T) {
	t.Run("returns false for token with no expiration", func(t *testing.T) {
		token := &Token{
			AccessToken: "test123",
			// ExpiresAt is zero value
		}
		require.False(t, token.IsExpired())
	})

	t.Run("returns false for token not yet expired", func(t *testing.T) {
		token := &Token{
			AccessToken: "test123",
			ExpiresAt:   time.Now().Add(time.Hour),
		}
		require.False(t, token.IsExpired())
	})

	t.Run("returns true for expired token", func(t *testing.T) {
		token := &Token{
			AccessToken: "test123",
			ExpiresAt:   time.Now().Add(-time.Hour),
		}
		require.True(t, token.IsExpired())
	})
}

func TestFileOAuthTokenStore_Concurrency(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "concurrent.json")

	store, err := NewFileOAuthTokenStore(filePath)
	require.NoError(t, err)

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
	require.NoError(t, err)

	retrieved, err := store.GetToken()
	require.NoError(t, err)
	require.Equal(t, "final_token", retrieved.AccessToken)
}

func TestFileOAuthTokenStore_AtomicWrites(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "atomic.json")

	store, err := NewFileOAuthTokenStore(filePath)
	require.NoError(t, err)

	// Save a token
	token := &Token{
		AccessToken: "test123",
		TokenType:   "Bearer",
	}
	err = store.SaveToken(token)
	require.NoError(t, err)

	// Verify no temporary files are left behind
	files, err := os.ReadDir(tempDir)
	require.NoError(t, err)

	var tmpFiles []string
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".tmp" {
			tmpFiles = append(tmpFiles, file.Name())
		}
	}
	require.Empty(t, tmpFiles, "Should not leave temporary files behind")
}
