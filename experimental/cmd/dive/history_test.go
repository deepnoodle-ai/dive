package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/tui"
)

func TestInputHistoryStore_SaveAndLoad(t *testing.T) {
	store := &inputHistoryStore{path: filepath.Join(t.TempDir(), ".dive", "history.json")}
	entries := []string{"first prompt", "a prompt\nwith multiple lines"}

	assert.NoError(t, store.Save(entries))

	raw, err := os.ReadFile(store.path)
	assert.NoError(t, err)
	var file inputHistoryFile
	assert.NoError(t, json.Unmarshal(raw, &file))
	assert.Equal(t, entries, file.Entries)

	info, err := os.Stat(store.path)
	assert.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	loaded, err := store.Load()
	assert.NoError(t, err)
	assert.Equal(t, entries, loaded)
}

func TestLimitInputHistoryKeepsNewestEntries(t *testing.T) {
	entries := make([]string, maxInputHistoryEntries+1)
	entries[0] = "oldest"
	entries[len(entries)-1] = "newest"

	limited := limitInputHistory(entries)

	assert.Equal(t, maxInputHistoryEntries, len(limited))
	assert.Equal(t, "", limited[0])
	assert.Equal(t, "newest", limited[len(limited)-1])
}

func TestInputHistoryStore_LoadMissingFile(t *testing.T) {
	store := &inputHistoryStore{path: filepath.Join(t.TempDir(), ".dive", "history.json")}

	entries, err := store.Load()

	assert.NoError(t, err)
	assert.Equal(t, 0, len(entries))
}

func TestInputHistoryStore_LoadRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	assert.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))

	_, err := (&inputHistoryStore{path: path}).Load()

	assert.Error(t, err)
}

func TestAppRecordInputHistoryReplacesInvalidHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".dive", "history.json")
	assert.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	assert.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))

	app := newTestApp()
	app.historyStore = &inputHistoryStore{path: path}
	_, err := app.historyStore.Load()
	assert.Error(t, err)

	app.recordInputHistory("replacement prompt")

	entries, err := app.historyStore.Load()
	assert.NoError(t, err)
	assert.Equal(t, []string{"replacement prompt"}, entries)
}

func TestAppRecordInputHistoryPersistsAcrossApps(t *testing.T) {
	store := &inputHistoryStore{path: filepath.Join(t.TempDir(), ".dive", "history.json")}
	first := newTestApp()
	first.historyStore = store
	first.recordInputHistory("first prompt")
	first.recordInputHistory("second prompt")

	entries, err := store.Load()
	assert.NoError(t, err)

	second := newTestApp()
	second.history = entries
	assert.True(t, second.handleInputNavKey(tui.KeyEvent{Key: tui.KeyArrowUp}))
	assert.Equal(t, "second prompt", second.inputText)
	assert.True(t, second.handleInputNavKey(tui.KeyEvent{Key: tui.KeyArrowUp}))
	assert.Equal(t, "first prompt", second.inputText)
}
