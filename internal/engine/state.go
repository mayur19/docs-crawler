package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CrawlState represents the persisted state of an in-progress crawl.
type CrawlState struct {
	SeedURL      string    `json:"seed_url"`
	CompletedURLs []string `json:"completed_urls"`
	PendingURLs  []string  `json:"pending_urls"`
	SavedAt      time.Time `json:"saved_at"`
}

// NewCrawlState creates a new empty CrawlState for the given seed URL.
func NewCrawlState(seedURL string) CrawlState {
	return CrawlState{
		SeedURL:       seedURL,
		CompletedURLs: []string{},
		PendingURLs:   []string{},
		SavedAt:       time.Now(),
	}
}

// SaveState writes the crawl state to a JSON file in the output directory.
func SaveState(outputDir string, state CrawlState) error {
	state.SavedAt = time.Now()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	statePath := filepath.Join(outputDir, "state.json")
	if err := os.WriteFile(statePath, data, 0o644); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}

	return nil
}

// LoadState reads the crawl state from a JSON file in the output directory.
// Returns an empty state if the file does not exist.
func LoadState(outputDir string) (CrawlState, error) {
	statePath := filepath.Join(outputDir, "state.json")

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return CrawlState{}, nil
		}
		return CrawlState{}, fmt.Errorf("read state file: %w", err)
	}

	var state CrawlState
	if err := json.Unmarshal(data, &state); err != nil {
		return CrawlState{}, fmt.Errorf("unmarshal state: %w", err)
	}

	return state, nil
}
