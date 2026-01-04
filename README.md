# Plarix Scan (GitHub Action)

**Free CI cost recorder for LLM API usage.**
Records tokens and costs from *real* provider responses (no estimation) during your workflow.

## Features
- **Accurate**: Uses official `usage` fields from OpenAI/Anthropic/OpenRouter.
- **Real-time**: Intercepts traffic during `run`, ensuring even streaming responses are captured.
- **Zero-Config**: Works with standard env vars (`OPENAI_BASE_URL` injection). (Note: Requires your SDK to respect base URL overrides).

---

## Quick Start via GitHub Action

Add this to your `.github/workflows/cost.yml`:

```yaml
name: LLM Cost Check
on: [pull_request]

permissions:
  pull-requests: write # Required for PR comments

jobs:
  scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - uses: plarix-ai/scan@v0.6.0
        with:
          command: "pytest -v" # The command that runs your LLM tests
          fail_on_cost_usd: 1.0 # Optional: fail if > $1.00
        env:
          OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
```

## How It Works
1. **Wraps** your command (`pytest`, `npm test`, etc.).
2. **Injects** local proxy variables (e.g. `OPENAI_BASE_URL=http://127.0.0.1:port/openai`).
3. **Intercepts** traffic to extract token usage from response bodies.
4. **Calculates** cost using a bundled, versioned pricing table.
5. **Reports** results via PR comment and Job Summary.

### Supported Environmental Overrides
Plarix Scan automatically sets these. Your tests/app must pick them up:

| Env Var | Target Provider |
|---------|-----------------|
| `OPENAI_BASE_URL` | OpenAI |
| `ANTHROPIC_BASE_URL` | Anthropic |
| `OPENROUTER_BASE_URL` | OpenRouter |

---

## Artifacts & Outputs

### `plarix-ledger.jsonl`
A JSONL file containing every intercepted request.
```json
{"ts":"...","provider":"openai","model":"gpt-4o","input_tokens":50,"output_tokens":120,"cost_usd":0.001325,"cost_known":true}
```

### `plarix-summary.json`
Aggregated statistics used for the report.
```json
{
  "total_calls": 5,
  "total_known_cost_usd": 0.045
}
```

---

## Accuracy Contract

1.  **Strict Usage Reporting**: We only calculate cost if the provider returns a valid `usage` field.
2.  **No Estimation**: We do not count tokens ourselves (tokenizer-free). If usage is missing, cost is `UNKNOWN`.
3.  **Real Pricing**: We use a snapshot of pricing data (Jan 4, 2026). If a model is not found, cost is `UNKNOWN`.
4.  **Streaming**: We parse SSE streams to find usage chunks (e.g., OpenAI `stream_options`).

---

## Local Development (Testing the Action)

To verify the scanner locally:

1.  Results are written to `plarix-ledger.jsonl` in the current directory.
2.  CLI Usage:
    ```bash
    go run ./cmd/plarix-scan run --command "curl -s http://.../v1/chat/completions"
    ```