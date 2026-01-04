# Changelog

All notable changes to Plarix Scan will be documented in this file.

## [0.6.0] - 2026-01-04

### Added
- Docker support: `Dockerfile`, `docker-compose.yaml`, and `plarix-scan proxy` daemon mode
- Real-time streaming token usage capture (stream_options injection support)
- Upstream override support (PLARIX_UPSTREAM_*)
- Pricing update (Jan 2026) with sources
- Documentation overhaul (README, Go docs)

## [0.5.0] - 2026-01-03

### Added
- Transparent streaming (SSE) support: passes through chunks without buffering
- Automatic detection of streaming responses (marked as unknown cost by default)
- Optional `enable_openai_stream_usage_injection` input to force usage reporting in OpenAI streams
- Request body modification proxy logic for injection

## [0.4.0] - 2026-01-03

### Added
- `internal/action` package: GitHub Actions integration with PR comment upsert
- Marker-based comment idempotency (`<!-- plarix-scan -->`)
- Step Summary writing via action package
- PR context detection from GitHub event payload
- Graceful handling of non-PR events (skips comment, keeps summary)

## [0.3.0] - 2026-01-03

### Added
- `internal/proxy` package: HTTP forward proxy with provider routing
- `internal/providers/openai` package: response parser for usage extraction
- Full CLI integration: proxy starts, runs user command, computes costs, writes ledger + summary
- Environment variable injection for SDK base URLs (OPENAI_BASE_URL, etc.)
- Integration test with mock OpenAI server

## [0.2.0] - 2026-01-03

### Added
- `internal/ledger` package: JSONL writer, entry types, aggregator for cost summaries
- `internal/pricing` package: pricing table loader with staleness checks
- `prices/prices.json` with OpenAI, Anthropic, and OpenRouter model pricing
- Unit tests for ledger and pricing packages

## [0.1.0] - 2026-01-03

### Added
- Initial repo skeleton with Go module structure
- `action.yml` composite action with all input definitions
- Minimal CLI with `run` subcommand that writes GitHub Step Summary
- VERSION and CHANGELOG files
- README with project description and usage snippet
