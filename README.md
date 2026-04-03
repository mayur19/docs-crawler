# docs-crawler

Crawl any documentation site into a searchable AI knowledge base. One binary. Zero dependencies.

- Single static binary — no Node, no Python, no Docker required
- Offline-capable — local embeddings via Ollama or built-in TF-IDF fallback
- Instant semantic search over any crawled documentation
- Full RAG export to JSONL, Parquet, or CSV

## Quick Start

```bash
go install github.com/mayur19/docs-crawler@latest
docs-crawler ingest https://docs.example.com
docs-crawler search "how to authenticate"
```

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

## Commands

| Command | Description |
|---------|-------------|
| `crawl` | Crawl a documentation site and save clean Markdown |
| `discover` | List URLs discoverable from a site without fetching content |
| `ingest` | Crawl, chunk, embed, and index a documentation site |
| `search` | Semantic search over an ingested knowledge base |
| `export` | Export indexed content to JSONL, Parquet, or CSV |
| `init` | Generate a starter config file |

## Crawl

Crawl a documentation site and save clean Markdown with metadata JSON:

```bash
docs-crawler crawl https://docs.example.com
```

With options:

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
| `--user-agent` | `docs-crawler/1.0.0` | Custom User-Agent |
| `--timeout` | `30s` | Per-request timeout |
| `--resume` | `false` | Resume interrupted crawl |
| `--verbose`, `-v` | `false` | Verbose logging |

## Discover

List discovered URLs without fetching content. Useful for previewing what will be crawled.

```bash
docs-crawler discover https://docs.example.com
```

### `docs-crawler discover [url]`

Outputs one URL per line to stdout. Accepts the same scope flags as `crawl` (`--include`, `--exclude`, `--max-depth`).

## Ingest

Crawl, chunk, embed, and index a documentation site in one step:

```bash
docs-crawler ingest https://docs.example.com
```

With options:

```bash
docs-crawler ingest https://docs.example.com \
  --chunk-strategy paragraph \
  --max-tokens 512 \
  --embedder ollama \
  --embedding-model nomic-embed-text \
  --embedding-batch 32 \
  --config ./docs-crawler.yaml
```

### `docs-crawler ingest [url]`

| Flag | Default | Description |
|------|---------|-------------|
| `--chunk-strategy` | `paragraph` | Chunking strategy: `paragraph`, `sentence`, `fixed` |
| `--max-tokens` | `512` | Maximum tokens per chunk |
| `--embedder` | `ollama` | Embedding provider: `ollama`, `tfidf`, `openai`, `cohere` |
| `--embedding-model` | provider default | Model name passed to the embedding provider |
| `--embedding-batch` | `32` | Number of chunks per embedding request |
| `--config` | none | Path to config file |
| `--output`, `-o` | `./docs-output` | Output directory for raw Markdown |
| `--workers` | `10` | Concurrent fetch workers |
| `--rate-limit` | auto-detect | Requests per second (0 = auto) |
| `--verbose`, `-v` | `false` | Verbose logging |

## Search

Semantic search over an ingested knowledge base:

```bash
docs-crawler search "how to authenticate"
```

With options:

```bash
docs-crawler search "rate limiting" \
  --top 10 \
  --source ./docs-output \
  --format json
```

### `docs-crawler search [query]`

| Flag | Default | Description |
|------|---------|-------------|
| `--top` | `5` | Number of results to return |
| `--source` | `./docs-output` | Directory containing the indexed knowledge base |
| `--format` | `text` | Output format: `text`, `json` |

## Export

Export indexed content for use in external RAG pipelines:

```bash
docs-crawler export --format jsonl -o ./export.jsonl
```

With options:

```bash
docs-crawler export \
  --format parquet \
  --include-vectors \
  --source ./docs-output \
  -o ./export.parquet
```

### `docs-crawler export`

| Flag | Default | Description |
|------|---------|-------------|
| `--format` | `jsonl` | Export format: `jsonl`, `parquet`, `csv` |
| `--include-vectors` | `false` | Include embedding vectors in the export |
| `-o` | `./export.jsonl` | Output file path |
| `--source` | `./docs-output` | Directory containing the indexed knowledge base |

## Config File

Generate a starter config:

```bash
docs-crawler init
```

This writes `docs-crawler.yaml` to the current directory:

```yaml
crawl:
  workers: 10
  max_depth: 0
  rate_limit: 0
  use_browser: false
  timeout: 30s

ingest:
  chunk_strategy: paragraph
  max_tokens: 512
  embedder: ollama
  embedding_model: nomic-embed-text
  embedding_batch: 32

output: ./docs-output
```

Pass the config file to any command with `--config ./docs-crawler.yaml`.

## Embedding Providers

| Provider | Requires | Notes |
|----------|----------|-------|
| `ollama` | Ollama running locally | Default. Fully offline. |
| `tfidf` | Nothing | Pure Go fallback. No external service needed. |
| `openai` | `OPENAI_API_KEY` env var | Uses `text-embedding-3-small` by default. |
| `cohere` | `COHERE_API_KEY` env var | Uses `embed-english-v3.0` by default. |

## Output Format

```
docs-output/
├── manifest.json              # Crawl metadata and stats
├── index/
│   ├── chunks.db              # SQLite index of chunks and vectors
│   └── vocab.json             # TF-IDF vocabulary (tfidf embedder only)
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

### Crawl pipeline

```
Discoverer -> Fetcher -> Extractor -> Writer
(sitemap,     (HTTP,      (readability, (markdown +
 links)        browser)    selector)     meta JSON)
```

### Ingest pipeline

```
Crawl pipeline -> Chunker -> Embedder -> Indexer
                  (paragraph, (ollama,    (SQLite
                   sentence,   tfidf,      vector
                   fixed)      openai,     store)
                               cohere)
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
