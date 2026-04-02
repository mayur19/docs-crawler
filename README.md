# docs-crawler

A fast, intelligent documentation crawler that produces clean Markdown for LLM/RAG pipelines.

Built in Go with a plugin architecture, parallel crawling, and automatic rate limit detection.

## Features

- **Intelligent rate limiting** — auto-detects limits from response headers (`X-RateLimit-*`, `Retry-After`), respects `robots.txt`, falls back to sensible defaults
- **Parallel pipeline** — concurrent goroutine pools for discovery, fetching, extraction, and writing
- **Plugin architecture** — extensible discoverers, fetchers, extractors, and writers
- **JavaScript support** — headless Chrome via `rod` for JS-rendered documentation sites
- **Clean output** — Markdown files with structured metadata JSON, optimized for RAG ingestion
- **Scope control** — URL prefix, include/exclude glob patterns, max depth, same-domain filtering
- **Resume support** — interrupted crawls can be resumed from saved state
- **Content deduplication** — skips pages with identical content via SHA-256 hashing

## Installation

```bash
go install github.com/mayur19/docs-crawler@latest
```

Or build from source:

```bash
git clone https://github.com/mayur19/docs-crawler.git
cd docs-crawler
make build
```

## Usage

### Crawl a documentation site

```bash
docs-crawler crawl https://docs.example.com
```

### With options

```bash
docs-crawler crawl https://docs.example.com \
  --output ./my-docs \
  --workers 20 \
  --max-depth 5 \
  --include "/api/*" --include "/guides/*" \
  --exclude "/changelog/*" \
  --rate-limit 10 \
  -v
```

### Discover URLs without crawling

```bash
docs-crawler discover https://docs.example.com
```

### Enable JavaScript rendering

```bash
docs-crawler crawl https://spa-docs.example.com --use-browser
```

## CLI Reference

### `docs-crawler crawl [url]`

| Flag | Default | Description |
|------|---------|-------------|
| `--output`, `-o` | `./docs-output` | Output directory |
| `--rate-limit` | auto-detect | Requests per second (0 = auto) |
| `--workers` | `10` | Concurrent fetch workers |
| `--max-depth` | `0` (unlimited) | Max link depth from seed |
| `--include` | none | URL include glob patterns (repeatable) |
| `--exclude` | none | URL exclude glob patterns (repeatable) |
| `--use-browser` | `false` | Enable headless Chrome |
| `--user-agent` | `docs-crawler/0.1.0` | Custom User-Agent |
| `--timeout` | `30s` | Per-request timeout |
| `--resume` | `false` | Resume interrupted crawl |
| `--verbose`, `-v` | `false` | Verbose logging |

### `docs-crawler discover [url]`

Lists discovered URLs without fetching content. Useful for previewing what will be crawled.

## Output Format

```
output/
├── manifest.json              # Crawl metadata and stats
├── pages/
│   ├── getting-started.md
│   ├── getting-started.meta.json
│   ├── api/
│   │   ├── authentication.md
│   │   └── authentication.meta.json
│   └── ...
└── state.json                 # Resume state
```

Each `.meta.json` contains:

```json
{
  "url": "https://docs.example.com/api/auth",
  "title": "Authentication",
  "description": "How to authenticate",
  "headings": ["Overview", "API Keys", "OAuth 2.0"],
  "word_count": 1250,
  "crawled_at": "2026-04-02T10:30:00Z",
  "content_hash": "sha256:abc123...",
  "links": ["../endpoints", "../rate-limits"]
}
```

## Architecture

```
Discoverer → Fetcher → Extractor → Writer
(sitemap,    (HTTP,     (readability, (markdown +
 links)      browser)   selector)     meta JSON)
```

Each stage runs in its own goroutine pool, connected by buffered channels with backpressure.

### Built-in Plugins

| Stage | Plugin | Description |
|-------|--------|-------------|
| Discoverer | sitemap | Parses sitemap.xml |
| Discoverer | link-follower | Follows `<a>` links within scope |
| Fetcher | http | HTTP client with retries and rate limiting |
| Fetcher | browser | Headless Chrome via rod |
| Extractor | readability | Readability-style content extraction |
| Extractor | selector | CSS selector-based extraction |
| Writer | markdown | Markdown files + metadata JSON |

## Rate Limiting

The crawler automatically detects rate limits with this priority:

1. **Explicit `--rate-limit` flag** — overrides everything
2. **Response headers** — `X-RateLimit-*`, `RateLimit-*` (IETF), `Retry-After`
3. **robots.txt** `Crawl-delay` directive
4. **Default** — 5 requests/second

On `429 Too Many Requests`, the crawler backs off immediately and adjusts its rate downward.

## Development

```bash
make build        # Build binary
make test         # Run tests with race detector
make test-cover   # Tests with coverage report
make vet          # Static analysis
make lint         # golangci-lint (if installed)
```
