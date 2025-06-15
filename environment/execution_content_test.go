package environment

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/diveagents/dive/llm"
	"github.com/stretchr/testify/require"
)

func TestContentFingerprinting(t *testing.T) {
	// Create a temporary file for testing
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")
	testContent := "# Test Markdown\n\nThis is test content."

	err := os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	// Create an execution instance for testing
	execution := &Execution{}
	ctx := context.Background()

	t.Run("TextContent fingerprinting", func(t *testing.T) {
		content := &llm.TextContent{Text: "Hello, world!"}

		fingerprint, err := execution.calculateContentFingerprint(ctx, content)
		require.NoError(t, err)
		require.NotEmpty(t, fingerprint.Hash)
		require.Equal(t, "text", fingerprint.Source)
		require.Equal(t, int64(13), fingerprint.Size)

		// Same content should produce same fingerprint
		fingerprint2, err := execution.calculateContentFingerprint(ctx, content)
		require.NoError(t, err)
		require.Equal(t, fingerprint.Hash, fingerprint2.Hash)
	})

	t.Run("Multiple content fingerprinting", func(t *testing.T) {
		content := []llm.Content{
			&llm.TextContent{Text: "First content"},
			&llm.TextContent{Text: "Second content"},
		}

		fingerprints, err := execution.calculateContentFingerprints(ctx, content)
		require.NoError(t, err)
		require.Len(t, fingerprints, 2)

		// Each fingerprint should be different
		require.NotEqual(t, fingerprints[0].Hash, fingerprints[1].Hash)
		require.Equal(t, "text", fingerprints[0].Source)
		require.Equal(t, "text", fingerprints[1].Source)
	})

	t.Run("Content snapshot creation", func(t *testing.T) {
		content := []llm.Content{
			&llm.TextContent{Text: "Test content for snapshot"},
		}

		snapshot, err := execution.createContentSnapshot(ctx, content)
		require.NoError(t, err)
		require.NotNil(t, snapshot)
		require.NotEmpty(t, snapshot.Fingerprint.Hash)
		require.Equal(t, "combined", snapshot.Fingerprint.Source)
		require.Len(t, snapshot.Content, 1)

		// Same content should produce same snapshot hash
		snapshot2, err := execution.createContentSnapshot(ctx, content)
		require.NoError(t, err)
		require.Equal(t, snapshot.Fingerprint.Hash, snapshot2.Fingerprint.Hash)
	})

	t.Run("Different content produces different hashes", func(t *testing.T) {
		content1 := []llm.Content{&llm.TextContent{Text: "Content A"}}
		content2 := []llm.Content{&llm.TextContent{Text: "Content B"}}

		snapshot1, err := execution.createContentSnapshot(ctx, content1)
		require.NoError(t, err)

		snapshot2, err := execution.createContentSnapshot(ctx, content2)
		require.NoError(t, err)

		require.NotEqual(t, snapshot1.Fingerprint.Hash, snapshot2.Fingerprint.Hash)
	})

	t.Run("ImageContent fingerprinting", func(t *testing.T) {
		content := &llm.ImageContent{
			Source: &llm.ContentSource{
				Type: llm.ContentSourceTypeBase64,
				Data: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==",
			},
		}

		fingerprint, err := execution.calculateContentFingerprint(ctx, content)
		require.NoError(t, err)
		require.NotEmpty(t, fingerprint.Hash)
		require.Equal(t, "image-base64", fingerprint.Source)
		require.Greater(t, fingerprint.Size, int64(0))
	})

	t.Run("DocumentContent fingerprinting", func(t *testing.T) {
		content := &llm.DocumentContent{
			Source: &llm.ContentSource{
				Type: llm.ContentSourceTypeURL,
				URL:  "https://example.com/document.pdf",
			},
		}

		fingerprint, err := execution.calculateContentFingerprint(ctx, content)
		require.NoError(t, err)
		require.NotEmpty(t, fingerprint.Hash)
		require.Equal(t, "document-url:https://example.com/document.pdf", fingerprint.Source)
	})
}

func TestContentSnapshotDeterminism(t *testing.T) {
	execution := &Execution{}
	ctx := context.Background()

	// Test that the same content always produces the same hash
	content := []llm.Content{
		&llm.TextContent{Text: "Deterministic content"},
		&llm.TextContent{Text: "More content"},
	}

	// Create multiple snapshots
	snapshots := make([]*ContentSnapshot, 5)
	for i := 0; i < 5; i++ {
		snapshot, err := execution.createContentSnapshot(ctx, content)
		require.NoError(t, err)
		snapshots[i] = snapshot
	}

	// All snapshots should have the same hash
	expectedHash := snapshots[0].Fingerprint.Hash
	for i := 1; i < len(snapshots); i++ {
		require.Equal(t, expectedHash, snapshots[i].Fingerprint.Hash,
			"Snapshot %d should have the same hash as snapshot 0", i)
	}
}
