# Changelog

All notable changes to Plarix Scan will be documented in this file.

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
