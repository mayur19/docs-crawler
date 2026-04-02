# docs-crawler Design Spec

**Date:** 2026-04-02
**Language:** Go
**Purpose:** Crawl documentation websites and produce clean Markdown for LLM/RAG ingestion

---

## Problem

Documentation sites are the primary knowledge source for LLM/RAG systems, but there's no ergonomic tool that crawls them intelligently — handling JS-rendered content, respecting rate limits automatically, and outputting clean Markdown with structured metadata.

## Goals

- Crawl documentation from any URL with minimal configuration
- Produce clean Markdown + metadata JSON optimized for LLM/RAG pipelines
- Auto-detect and respect rate limits from response headers
- Support both static HTML and JavaScript-rendered documentation sites
- Extensible plugin architecture for custom discovery, fetching, extraction, and output

## Non-Goals

- General-purpose web scraping (e-commerce, social media, etc.)
- Full-text search indexing (though output is compatible)
- Real-time monitoring or change detection

---

## Architecture

Plugin framework over a channel-based pipeline. Four plugin interfaces define behavior at each stage; the core engine orchestrates execution with goroutine pools and Go channels.

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  Discoverer  │────>│   Fetcher    │────>│  Extractor   │────>│   Writer     │
│  (plugin)    │ ch  │  (plugin)    │ ch  │  (plugin)    │ ch  │  (plugin)    │
└─────────────┘     └─────────────┘     └─────────────┘     └─────────────┘
                          │
                    ┌─────┴─────┐
                    │ Rate Limiter│
                    │ Retry Logic │
                    │ robots.txt  │
                    └───────────┘
```

### Pipeline Execution

- Each stage runs a configurable goroutine pool (Discovery=2, Fetch=10, Extract=5, Write=3)
- Buffered channels between stages provide natural backpressure
- Graceful shutdown via `context.Context` cancellation — drains in-flight work
- URL deduplication at the engine level (normalized URL set + content hash)

---

## Plugin Interfaces

```go
// Discoverer finds URLs to crawl
type Discoverer interface {
    Name() string
    Discover(ctx context.Context, seed *url.URL) (<-chan *CrawlURL, error)
}

// Fetcher retrieves page content
type Fetcher interface {
    Name() string
    CanFetch(u *CrawlURL) bool
    Fetch(ctx context.Context, u *CrawlURL) (*FetchResult, error)
}

// Extractor converts raw HTML to structured content
type Extractor interface {
    Name() string
    Extract(ctx context.Context, result *FetchResult) (*Document, error)
}

// Writer outputs the final documents
type Writer interface {
    Name() string
    Write(ctx context.Context, doc *Document) error
    Close() error
}
```

### Core Data Types

- **CrawlURL** — URL + depth + source (sitemap/link) + discovered-by
- **FetchResult** — URL + raw HTML + HTTP status + headers + content-type
- **Document** — URL + title + clean markdown + headings hierarchy + metadata + outbound links

### Built-in Plugins (v1)

| Stage | Plugin | Description |
|-------|--------|-------------|
| Discoverer | `sitemap` | Parses sitemap.xml and sitemap index files |
| Discoverer | `crawler` | Follows `<a>` links within scope boundaries |
| Fetcher | `http` | Standard HTTP client with connection pooling and retries |
| Fetcher | `browser` | Headless Chrome via `rod` for JS-rendered content |
| Extractor | `readability` | Mozilla Readability-style main content extraction |
| Extractor | `selector` | CSS selector-based extraction for custom layouts |
| Writer | `markdown` | Markdown files + per-page metadata JSON |

---

## Intelligent Rate Limiting

Priority hierarchy (highest wins):

1. **Explicit CLI flag** (`--rate-limit N`) — overrides everything
2. **Auto-detected from response headers:**
   - `X-RateLimit-Limit` / `X-RateLimit-Remaining` / `X-RateLimit-Reset`
   - `RateLimit-Limit` / `RateLimit-Remaining` / `RateLimit-Reset` (IETF draft)
   - `Retry-After` on 429/503 responses
3. **robots.txt** `Crawl-delay` directive
4. **Default** — 5 requests/second per domain

### Adaptive Behavior

- Starts at default, adjusts dynamically as headers are observed
- On 429: immediately backs off, reads `Retry-After`, adjusts limiter downward
- As `X-RateLimit-Remaining` approaches 0: proactively slows before hitting the wall
- Logs rate limit adjustments so user has visibility

---

## Scope Control

- `--url-prefix` — stay within URL path prefix (default: seed URL path)
- `--include` — glob patterns for URLs to include (repeatable)
- `--exclude` — glob patterns for URLs to exclude (repeatable)
- `--max-depth` — maximum link depth from seed URL (0 = unlimited)
- `--same-domain` — restrict to same domain (default: true)
- Respects `robots.txt` disallow rules
- In-memory visited set with normalized URLs
- Content hash deduplication for pages with identical content but different URLs

---

## CLI Interface

```bash
# Basic
docs-crawler crawl https://docs.example.com/v2/

# Full options
docs-crawler crawl https://docs.example.com \
  --output ./output \
  --rate-limit 10 \
  --workers 20 \
  --max-depth 5 \
  --include "/api/*" --include "/guides/*" \
  --exclude "/changelog/*" \
  --use-browser \
  --format markdown

# Resume interrupted crawl
docs-crawler crawl --resume ./output

# Discover URLs without fetching
docs-crawler discover https://docs.example.com
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--output`, `-o` | `./docs-output` | Output directory |
| `--rate-limit` | auto-detect | Requests/sec (overrides auto-detection) |
| `--workers` | `10` | Concurrent fetch workers |
| `--max-depth` | `0` (unlimited) | Max link depth from seed |
| `--include` | `*` | URL include glob patterns (repeatable) |
| `--exclude` | none | URL exclude glob patterns (repeatable) |
| `--use-browser` | `false` | Enable headless Chrome for JS sites |
| `--user-agent` | `docs-crawler/1.0` | Custom User-Agent string |
| `--timeout` | `30s` | Per-request timeout |
| `--resume` | `false` | Resume from previous crawl state |
| `--verbose`, `-v` | `false` | Verbose logging |

---

## Output Format

```
output/
├── manifest.json            # Crawl metadata, stats, config used
├── pages/
│   ├── getting-started.md
│   ├── getting-started.meta.json
│   ├── api/
│   │   ├── authentication.md
│   │   ├── authentication.meta.json
│   │   ├── endpoints.md
│   │   └── endpoints.meta.json
│   └── ...
└── state.json               # Crawl state for resume support
```

### Per-page Metadata (`.meta.json`)

```json
{
  "url": "https://docs.example.com/v2/api/auth",
  "title": "Authentication",
  "description": "How to authenticate with the API",
  "headings": ["Overview", "API Keys", "OAuth 2.0"],
  "word_count": 1250,
  "crawled_at": "2026-04-02T10:30:00Z",
  "content_hash": "sha256:abc123...",
  "links": ["../endpoints", "../rate-limits"]
}
```

### Manifest (`manifest.json`)

```json
{
  "seed_url": "https://docs.example.com/v2/",
  "started_at": "2026-04-02T10:00:00Z",
  "completed_at": "2026-04-02T10:05:30Z",
  "pages_crawled": 142,
  "pages_skipped": 8,
  "errors": 2,
  "config": { "...flags used..." }
}
```

---

## Error Handling

- **Retries**: Exponential backoff (3 attempts) for transient errors (5xx, timeouts, connection resets)
- **Skip & log**: 404s and permanent errors logged, crawl continues
- **Resume**: Crawl state persisted to `state.json` — interrupted crawls can resume without re-fetching completed pages
- **Verbose mode**: Logs every request, response status, rate limit adjustments, and extraction results

---

## Technology Choices

| Component | Library | Rationale |
|-----------|---------|-----------|
| CLI | `cobra` | Standard Go CLI framework, subcommand support |
| HTTP client | `net/http` + custom transport | Connection pooling, retries, rate limiting at transport layer |
| HTML parsing | `goquery` | jQuery-like selectors, built on `net/html` |
| Headless browser | `rod` | Modern, well-maintained, auto-manages Chrome |
| Markdown conversion | `html-to-markdown` | Go lib for HTML→Markdown |
| Rate limiting | `golang.org/x/time/rate` | Stdlib-adjacent token bucket |
| URL normalization | `purell` or custom | Consistent URL dedup |
| Logging | `slog` | Stdlib structured logging (Go 1.21+) |

---

## Testing Strategy

- **Unit tests**: Each plugin interface tested independently with mock data
- **Integration tests**: HTTP test server simulating doc sites (static + rate-limited)
- **E2E tests**: Crawl a local test documentation site, verify output structure
- **Target**: 80%+ code coverage
