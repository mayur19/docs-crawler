# docs-crawler v1.0 — Zero-Dependency AI Pipeline

> Crawl any documentation site into a searchable AI knowledge base. One binary. Zero dependencies.

**Date:** 2026-04-02
**Status:** Draft
**Author:** mayur19

---

## 1. Vision

A single Go binary that takes a documentation URL and produces a fully searchable, embeddable knowledge base — offline, no API keys, no Docker, no Python. Install and run in under 10 seconds.

### Positioning

- "Firecrawl requires a hosted API. Crawl4AI requires Python + dozens of deps. docs-crawler is a single binary that works offline."
- The only end-to-end docs-to-AI tool in Go. Zero competition in the single-binary, no-runtime-dependency space.

### Target audiences

1. RAG pipeline builders tired of Python dependency hell
2. AI coding assistant users who want local docs for context
3. Chatbot/knowledge-base builders who need a quick start
4. ML engineers building training datasets from documentation

### Non-goals

- Not an API/SaaS
- Not an MCP server
- Not a platform with plugin registries
- Not competing on hosted/cloud features

---

## 2. Pipeline Architecture

The current 4-stage pipeline extends to 7 stages. Same channel-based design, same plugin interface pattern.

```
crawl:   Discover → Fetch → Extract → Write
            (2)      (10)     (5)      (3)

ingest:  Discover → Fetch → Extract → Chunk → Embed → Index
            (2)      (10)     (5)      (3)     (2)     (1)
```

### Existing stages (unchanged)

| Stage | Plugins | Status |
|-------|---------|--------|
| Discover | sitemap, link-follower | Complete |
| Fetch | HTTP (retries + rate limiting), headless browser (rod) | Complete |
| Extract | readability, CSS selector | Complete |
| Write | Markdown + metadata JSON | Complete, becomes one export format |

### New stages

| Stage | Responsibility | Workers |
|-------|---------------|---------|
| Chunker | Split documents into semantic chunks preserving structure | 3 |
| Embedder | Convert chunks to vector embeddings | 2 (CPU-heavy, batches internally) |
| Indexer | Store embeddings in searchable index | 1 (single-writer for consistency) |

### New interfaces

```go
type Chunker interface {
    Name() string
    Chunk(ctx context.Context, doc Document) ([]Chunk, error)
}

type Embedder interface {
    Name() string
    Embed(ctx context.Context, chunks []Chunk) ([]EmbeddedChunk, error)
    Dimensions() int
}

type Indexer interface {
    Name() string
    Index(ctx context.Context, chunks []EmbeddedChunk) error
    Search(ctx context.Context, query string, topK int) ([]SearchResult, error)
    Close() error
}
```

### Channel flow

```
discoverCh → fetchCh → extractCh → chunkCh → embedCh → indexCh
```

Each channel is buffered at 2x pool size. Each stage closes the next channel when complete (existing cascade pattern).

### Command routing

- `docs-crawler crawl` — runs Discover → Fetch → Extract → Write (existing behavior, unchanged)
- `docs-crawler ingest` — runs Discover → Fetch → Extract → Chunk → Embed → Index (no Markdown Write stage; the SQLite index is the primary output; use `export --format markdown` to get Markdown files)
- No breaking changes to existing commands.

---

## 3. Structure-Aware Chunking

Documentation isn't prose. It has headings, code blocks, tables, admonitions, and cross-references. Our chunker understands this structure.

### Strategy: heading-boundary splitting

1. Parse Markdown into a tree of sections (split on `##` and `###` headings)
2. Each section becomes a candidate chunk
3. If a section exceeds `max_chunk_tokens` (default: 512), split further at paragraph boundaries
4. Never split mid-code-block, mid-table, or mid-list
5. Each chunk inherits its heading hierarchy as context

### Chunk type

```go
type Chunk struct {
    ID          string   // deterministic hash of source URL + heading path
    Content     string   // the chunk text
    SourceURL   string   // origin page
    Title       string   // page title
    HeadingPath []string // e.g., ["Authentication", "API Keys", "Rotating Keys"]
    ChunkIndex  int      // position within the page
    TotalChunks int      // total chunks from this page
    TokenCount  int      // for context window budgeting
    ContentHash string   // SHA-256 for dedup across incremental crawls
}
```

### Token counting

Use whitespace-based approximation (`words * 1.3`) rather than pulling in a tokenizer dependency. Good enough for chunking decisions. Keeps the zero-dependency promise.

### Why this matters for RAG quality

- Heading path gives the LLM context about where the chunk sits in the docs hierarchy
- Preserving code blocks intact means retrieved chunks have working examples, not truncated snippets
- Deterministic chunk IDs enable incremental re-indexing — only changed chunks get re-embedded

---

## 4. Local Embedding

### Provider priority (runtime detection)

| Priority | Provider | How it works | When used |
|----------|----------|-------------|-----------|
| 1 | **Ollama** (default) | HTTP call to `localhost:11384/api/embed` | Auto-detected if Ollama is running |
| 2 | **TF-IDF fallback** | Pure Go, no external deps, sparse vectors | Ollama unavailable, no API keys set |
| 3 | **OpenAI** (opt-in) | `--embedder openai`, reads `OPENAI_API_KEY` | User explicitly opts in |
| 4 | **Cohere** (opt-in) | `--embedder cohere`, reads `COHERE_API_KEY` | User explicitly opts in |

### Key design decisions

- **No bundled model.** Bundling ONNX would add 50MB+ to the binary and require CGO. Ollama is the standard for local AI — we just make HTTP calls.
- **TF-IDF fallback is critical.** If Ollama is not running and no API keys are set, the tool still works. Users get keyword search instead of semantic search. `ingest` never fails due to missing embedding infrastructure.
- **Batch embedding.** Send chunks in batches of 64 to Ollama/API to maximize throughput.
- **Embedding dimensions stored in index metadata** so the search command knows how to query regardless of which provider was used.

### Default model

Ollama with `nomic-embed-text`: good quality, fast, 768 dimensions, widely available.

### UX on first run

```
$ docs-crawler ingest https://docs.stripe.com
✓ Discovered 142 pages (sitemap)
✓ Fetched 142 pages (12.3s)
✓ Extracted 142 documents
✓ Chunked into 1,847 chunks (avg 380 tokens)
⚠ Ollama not detected — using TF-IDF embeddings (keyword search)
  Hint: install Ollama for semantic search → https://ollama.com
✓ Indexed 1,847 chunks (0.8s)
✓ Knowledge base saved to ./docs-output/

Search with: docs-crawler search "your query"
```

---

## 5. Local Vector Index & Search

### Storage: Pure Go SQLite

Use `modernc.org/sqlite` (pure Go, no CGO). For dense vectors (Ollama/OpenAI), store embeddings as blobs and compute cosine similarity at query time. For TF-IDF, use SQLite's FTS5 for keyword search.

### Why brute-force cosine similarity?

For documentation-scale datasets (typically <100K chunks), brute-force is fast enough. A 50K vector search at 768 dimensions takes ~50ms in Go. No need for approximate nearest neighbor (ANN) libraries that would require CGO.

### Database schema

```sql
CREATE TABLE chunks (
    id TEXT PRIMARY KEY,
    source_url TEXT NOT NULL,
    title TEXT NOT NULL,
    heading_path TEXT NOT NULL,  -- JSON array
    content TEXT NOT NULL,
    token_count INTEGER NOT NULL,
    content_hash TEXT NOT NULL
);

CREATE TABLE embeddings (
    chunk_id TEXT PRIMARY KEY REFERENCES chunks(id),
    vector BLOB NOT NULL,       -- float32 array as bytes
    dimensions INTEGER NOT NULL
);

CREATE TABLE meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE VIRTUAL TABLE chunks_fts USING fts5(content, title, heading_path);
```

### Search strategy

- If dense embeddings exist → cosine similarity, re-ranked with BM25 from FTS5 (hybrid search)
- If only TF-IDF → FTS5 keyword search with BM25 ranking
- Hybrid always wins on quality: semantic understands meaning, BM25 catches exact terms

### Search output

```
$ docs-crawler search "how to handle webhooks"

┌─────────────────────────────────────────────────────┐
│ 1. Webhook Endpoints (0.94)                         │
│    https://docs.stripe.com/webhooks/endpoints       │
│    Authentication > Webhooks > Endpoints             │
│                                                      │
│    To handle webhooks, create an endpoint that       │
│    accepts POST requests. Verify the signature       │
│    using your webhook signing secret...              │
├─────────────────────────────────────────────────────┤
│ 2. Webhook Events (0.89)                            │
│    https://docs.stripe.com/webhooks/events          │
│    ...                                               │
└─────────────────────────────────────────────────────┘
```

### Incremental re-crawl

```bash
docs-crawler ingest https://docs.stripe.com  # first run: full crawl
docs-crawler ingest https://docs.stripe.com  # second run: only changed pages
```

Content hashes (already in the codebase) detect which pages changed. Only changed chunks get re-embedded and re-indexed. Deleted pages get pruned from the index.

---

## 6. Export & Output Formats

### Export command

```bash
# JSONL with embeddings
docs-crawler export --format jsonl --include-vectors -o chunks.jsonl

# JSONL without vectors (smaller)
docs-crawler export --format jsonl -o chunks.jsonl

# CSV for exploration
docs-crawler export --format csv -o chunks.csv

# Markdown (backwards compatible with current output)
docs-crawler export --format markdown -o ./docs-output/
```

### JSONL record structure

```json
{
  "id": "a1b2c3",
  "content": "To handle webhooks, create an endpoint...",
  "source_url": "https://docs.stripe.com/webhooks",
  "title": "Webhook Endpoints",
  "heading_path": ["Authentication", "Webhooks", "Endpoints"],
  "chunk_index": 2,
  "total_chunks": 5,
  "token_count": 384,
  "content_hash": "sha256:abc123...",
  "crawled_at": "2026-04-02T10:30:00Z",
  "vector": [0.0123, -0.0456, ...]
}
```

The `vector` field is omitted unless `--include-vectors` is passed. This JSONL is directly importable into Qdrant, Pinecone, Weaviate, ChromaDB, and pgvector with minimal glue code.

---

## 7. CLI Surface

### Commands

| Command | Description |
|---------|-------------|
| `crawl [url]` | Crawl to Markdown + metadata (existing, unchanged) |
| `discover [url]` | List discovered URLs (existing, unchanged) |
| `ingest [url]` | Full pipeline: crawl → chunk → embed → index |
| `search [query]` | Search a local knowledge base |
| `export` | Export chunks/vectors as JSONL, CSV, or Markdown |
| `init [url]` | Generate a starter config file |

### New flags for `ingest`

| Flag | Default | Description |
|------|---------|-------------|
| `--chunk-strategy` | `heading` | Chunking strategy: heading, fixed, paragraph |
| `--max-tokens` | `512` | Maximum tokens per chunk |
| `--embedder` | `auto` | Embedding provider: auto, ollama, openai, cohere, tfidf |
| `--embedding-model` | `nomic-embed-text` | Model name for the embedding provider |
| `--embedding-batch` | `64` | Batch size for embedding requests |
| `--config` | none | Path to YAML config file |

### Flags for `search`

| Flag | Default | Description |
|------|---------|-------------|
| `--top` | `5` | Number of results |
| `--source` | `./docs-output` | Path to knowledge base |
| `--format` | `pretty` | Output format: pretty, json |

### Flags for `export`

| Flag | Default | Description |
|------|---------|-------------|
| `--format` | `jsonl` | Export format: jsonl, csv, markdown |
| `--include-vectors` | `false` | Include embedding vectors in output |
| `-o` | stdout | Output path |
| `--source` | `./docs-output` | Path to knowledge base |

---

## 8. Config File

Optional YAML config for reproducible pipelines.

```yaml
# docs-crawler.yaml
source:
  url: https://docs.stripe.com
  include: ["/api/*", "/guides/*"]
  exclude: ["/changelog/*"]
  max_depth: 5
  use_browser: false

crawl:
  workers: 20
  rate_limit: 0  # auto-detect
  timeout: 30s
  user_agent: "docs-crawler/1.0"

chunking:
  strategy: heading
  max_tokens: 512
  preserve_code_blocks: true

embedding:
  provider: ollama
  model: nomic-embed-text
  batch_size: 64

output:
  dir: ./stripe-docs
```

Usage:

```bash
docs-crawler ingest --config docs-crawler.yaml
docs-crawler ingest --config docs-crawler.yaml --workers 50  # CLI flags override
docs-crawler init https://docs.stripe.com > docs-crawler.yaml  # generate starter config
```

The `init` command auto-detects sitemap, suggests include/exclude patterns based on discovered paths, and picks sensible defaults.

---

## 9. Existing Codebase Fixes

### Must fix (blocking for v1.0)

| Issue | Current State | Fix |
|-------|--------------|-----|
| Resume not functional | `--resume` flag accepted but ignored, state structs exist but not wired in | Integrate save/load into engine — checkpoint on interrupt, reload on restart |
| robots.txt ignored | `internal/scope/robots.go` exists but never called | Wire into fetch pipeline, populate `RobotsCrawlDelay` in rate limiter |
| Browser fetcher never closed | `BrowserFetcher.Close()` never called on shutdown | Add cleanup in engine shutdown path |
| Only first extractor used | Multi-extractor accepted but `extractors[0]` hardcoded | Route by content type or let user select via config |

### Should fix (quality of life)

| Issue | Fix |
|-------|-----|
| No crawl-level timeout | Add `--max-time` flag for total crawl duration |
| Discover command doesn't show count | Print total at the end |
| Manifest missing run duration | Add `duration_seconds` field |
| No dry-run mode | Add `--dry-run` flag to show what would be crawled without fetching |

---

## 10. New Dependencies

| Dependency | Purpose | CGO required? |
|------------|---------|---------------|
| `modernc.org/sqlite` | Pure Go SQLite driver | No |
| `gopkg.in/yaml.v3` | YAML config parsing | No (already indirect dep) |

All other functionality (Ollama HTTP calls, TF-IDF, cosine similarity, JSONL/CSV export) is implemented in pure Go with no additional dependencies. The binary stays self-contained.

---

## 11. New Package Layout

```
internal/
├── chunk/
│   ├── chunker.go          # HeadingChunker implementation
│   ├── chunker_test.go
│   ├── fixed.go            # FixedChunker (token-count based)
│   ├── fixed_test.go
│   └── tokens.go           # Token counting utilities
├── embed/
│   ├── ollama.go           # Ollama HTTP client
│   ├── ollama_test.go
│   ├── tfidf.go            # Pure Go TF-IDF
│   ├── tfidf_test.go
│   ├── openai.go           # OpenAI embeddings client
│   ├── openai_test.go
│   ├── cohere.go           # Cohere embeddings client
│   └── cohere_test.go
├── index/
│   ├── sqlite.go           # SQLite storage + vector search
│   ├── sqlite_test.go
│   ├── search.go           # Hybrid search logic
│   └── search_test.go
├── export/
│   ├── jsonl.go            # JSONL exporter
│   ├── csv.go              # CSV exporter
│   └── export_test.go
├── config/
│   ├── config.go           # (existing) add chunking/embedding/index fields
│   ├── yaml.go             # YAML config loader
│   └── yaml_test.go
└── ... (existing packages unchanged)
```

---

## 12. Success Metrics

For a 30K+ star trajectory, the tool needs:

| Metric | Target | How |
|--------|--------|-----|
| Time to first search | < 60 seconds from install | `go install` + `ingest` + `search` |
| Zero-config success rate | 100% | TF-IDF fallback ensures it always works |
| Crawl performance | 100+ pages/minute on default settings | Existing parallel pipeline handles this |
| Search latency | < 100ms for 50K chunks | Brute-force cosine is fast enough |
| Binary size | < 30MB | No bundled models, pure Go deps |
| README demo | Copy-paste to working search in 3 commands | The hook that earns stars |
