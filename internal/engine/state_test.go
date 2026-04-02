package engine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mayur19/docs-crawler/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCrawlState(t *testing.T) {
	state := engine.NewCrawlState("https://docs.example.com")
	assert.Equal(t, "https://docs.example.com", state.SeedURL)
	assert.Empty(t, state.CompletedURLs)
	assert.Empty(t, state.PendingURLs)
	assert.False(t, state.SavedAt.IsZero())
}

func TestSaveAndLoadState(t *testing.T) {
	dir := t.TempDir()

	state := engine.NewCrawlState("https://docs.example.com")
	state.CompletedURLs = []string{
		"https://docs.example.com/page1",
		"https://docs.example.com/page2",
	}
	state.PendingURLs = []string{
		"https://docs.example.com/page3",
	}

	err := engine.SaveState(dir, state)
	require.NoError(t, err)

	// Verify file exists.
	_, err = os.Stat(filepath.Join(dir, "state.json"))
	require.NoError(t, err)

	loaded, err := engine.LoadState(dir)
	require.NoError(t, err)

	assert.Equal(t, state.SeedURL, loaded.SeedURL)
	assert.Equal(t, state.CompletedURLs, loaded.CompletedURLs)
	assert.Equal(t, state.PendingURLs, loaded.PendingURLs)
}

func TestLoadStateNotExists(t *testing.T) {
	dir := t.TempDir()

	state, err := engine.LoadState(dir)
	require.NoError(t, err)
	assert.Empty(t, state.SeedURL)
}

func TestLoadStateInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("not json"), 0o644)
	require.NoError(t, err)

	_, err = engine.LoadState(dir)
	assert.Error(t, err)
}
