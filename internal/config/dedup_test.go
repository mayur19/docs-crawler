package config

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDeduplicator(t *testing.T) {
	d := NewDeduplicator()
	require.NotNil(t, d)

	stats := d.Stats()
	assert.Equal(t, 0, stats.URLsSeen)
	assert.Equal(t, 0, stats.ContentDups)
}

func TestDeduplicator_SeenURL(t *testing.T) {
	tests := []struct {
		name     string
		urls     []string
		query    string
		wantSeen bool
	}{
		{
			name:     "first URL returns false",
			urls:     nil,
			query:    "https://example.com/page1",
			wantSeen: false,
		},
		{
			name:     "duplicate URL returns true",
			urls:     []string{"https://example.com/page1"},
			query:    "https://example.com/page1",
			wantSeen: true,
		},
		{
			name:     "different URL returns false",
			urls:     []string{"https://example.com/page1"},
			query:    "https://example.com/page2",
			wantSeen: false,
		},
		{
			name:     "duplicate among many returns true",
			urls:     []string{"https://a.com", "https://b.com", "https://c.com"},
			query:    "https://b.com",
			wantSeen: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDeduplicator()
			for _, u := range tt.urls {
				d.SeenURL(u)
			}
			got := d.SeenURL(tt.query)
			assert.Equal(t, tt.wantSeen, got)
		})
	}
}

func TestDeduplicator_SeenContent(t *testing.T) {
	tests := []struct {
		name     string
		hashes   []string
		query    string
		wantSeen bool
	}{
		{
			name:     "first content returns false",
			hashes:   nil,
			query:    "sha256:abc123",
			wantSeen: false,
		},
		{
			name:     "duplicate content returns true",
			hashes:   []string{"sha256:abc123"},
			query:    "sha256:abc123",
			wantSeen: true,
		},
		{
			name:     "different content returns false",
			hashes:   []string{"sha256:abc123"},
			query:    "sha256:def456",
			wantSeen: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDeduplicator()
			for _, h := range tt.hashes {
				d.SeenContent(h)
			}
			got := d.SeenContent(tt.query)
			assert.Equal(t, tt.wantSeen, got)
		})
	}
}

func TestDeduplicator_Stats(t *testing.T) {
	d := NewDeduplicator()

	// Add unique URLs
	d.SeenURL("https://a.com")
	d.SeenURL("https://b.com")
	d.SeenURL("https://c.com")
	// Re-add a duplicate URL (does not increase URLsSeen)
	d.SeenURL("https://a.com")

	// Add unique content
	d.SeenContent("sha256:hash1")
	d.SeenContent("sha256:hash2")
	// Add duplicate content (increases ContentDups)
	d.SeenContent("sha256:hash1")
	d.SeenContent("sha256:hash1")

	stats := d.Stats()
	assert.Equal(t, 3, stats.URLsSeen)
	assert.Equal(t, 2, stats.ContentDups)
}

func TestDeduplicator_ConcurrentURLAccess(t *testing.T) {
	d := NewDeduplicator()

	const goroutines = 100
	const urlsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			for j := range urlsPerGoroutine {
				url := fmt.Sprintf("https://example.com/%d/%d", id, j)
				d.SeenURL(url)
			}
		}(i)
	}

	wg.Wait()

	stats := d.Stats()
	assert.Equal(t, goroutines*urlsPerGoroutine, stats.URLsSeen)
}

func TestDeduplicator_ConcurrentContentAccess(t *testing.T) {
	d := NewDeduplicator()

	const goroutines = 50
	// All goroutines insert the same hash, so only the first succeeds as "new"
	sharedHash := "sha256:shared"

	var wg sync.WaitGroup
	wg.Add(goroutines)

	seenCount := make([]bool, goroutines)

	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			seenCount[id] = d.SeenContent(sharedHash)
		}(i)
	}

	wg.Wait()

	// Exactly one goroutine should have been the first to see it (returned false)
	falseCount := 0
	trueCount := 0
	for _, seen := range seenCount {
		if seen {
			trueCount++
		} else {
			falseCount++
		}
	}
	assert.Equal(t, 1, falseCount, "exactly one goroutine should be the first to see the hash")
	assert.Equal(t, goroutines-1, trueCount, "all other goroutines should see the hash as duplicate")

	stats := d.Stats()
	assert.Equal(t, goroutines-1, stats.ContentDups)
}

func TestDeduplicator_ConcurrentMixedAccess(t *testing.T) {
	d := NewDeduplicator()

	const goroutines = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Half do URL checks, half do content checks
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			d.SeenURL(fmt.Sprintf("https://example.com/%d", id))
		}(i)
		go func(id int) {
			defer wg.Done()
			d.SeenContent(fmt.Sprintf("sha256:hash%d", id))
		}(i)
	}

	wg.Wait()

	stats := d.Stats()
	assert.Equal(t, goroutines, stats.URLsSeen)
	assert.Equal(t, 0, stats.ContentDups)
}
