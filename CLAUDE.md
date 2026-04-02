# docs-crawler

Go CLI tool that crawls documentation websites and produces clean Markdown + metadata JSON for LLM/RAG ingestion.

## Build & Test

```bash
make build        # Build binary
make test         # Run tests with race detector
make test-cover   # Run tests with coverage report
make vet          # Run go vet
make lint         # Run golangci-lint
```

## Architecture

Plugin framework over a channel-based pipeline:
- **Discoverer** — finds URLs (sitemap, link-following)
- **Fetcher** — retrieves content (HTTP, headless browser)
- **Extractor** — HTML to clean Markdown (readability, CSS selectors)
- **Writer** — outputs results (Markdown + metadata JSON)

Core engine orchestrates with goroutine pools per stage connected by buffered channels.

## Conventions

- All packages under `internal/` (not a library)
- Immutable data — constructors return new structs, no mutation
- Functions < 50 lines, files < 800 lines
- TDD: write tests first
- `slog` for structured logging
- Table-driven tests with `testify`
- Error handling: wrap with context, never swallow
