package config

import "sync"

// DedupStats holds deduplication statistics.
type DedupStats struct {
	URLsSeen    int
	ContentDups int
}

// Deduplicator provides thread-safe URL and content hash deduplication.
type Deduplicator struct {
	mu          sync.Mutex
	urls        map[string]struct{}
	contentSeen map[string]struct{}
	contentDups int
}

// NewDeduplicator creates a new, empty Deduplicator.
func NewDeduplicator() *Deduplicator {
	return &Deduplicator{
		urls:        make(map[string]struct{}),
		contentSeen: make(map[string]struct{}),
	}
}

// SeenURL performs an atomic check-and-set for the given normalized URL.
// Returns true if the URL was already seen, false if this is the first time.
func (d *Deduplicator) SeenURL(normalized string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.urls[normalized]; exists {
		return true
	}
	d.urls[normalized] = struct{}{}
	return false
}

// SeenContent performs an atomic check-and-set for the given content hash.
// Returns true if the content hash was already seen, false if this is the first time.
func (d *Deduplicator) SeenContent(hash string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.contentSeen[hash]; exists {
		d.contentDups++
		return true
	}
	d.contentSeen[hash] = struct{}{}
	return false
}

// Stats returns a snapshot of the current deduplication statistics.
func (d *Deduplicator) Stats() DedupStats {
	d.mu.Lock()
	defer d.mu.Unlock()

	return DedupStats{
		URLsSeen:    len(d.urls),
		ContentDups: d.contentDups,
	}
}
