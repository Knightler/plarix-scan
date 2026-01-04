// Package pricing handles LLM model pricing data.
//
// Purpose: Load pricing table, compute costs, check staleness.
// Public API: Prices, Load, ComputeCost, IsStale
// Usage: Load prices.json, then call ComputeCost for each model.
package pricing

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Prices holds the pricing table for all supported models.
type Prices struct {
	AsOf   string                `json:"as_of"`
	Models map[string]ModelPrice `json:"models"`
}

// ModelPrice holds per-1K token prices for a model.
type ModelPrice struct {
	InputPer1K  float64 `json:"input_per_1k"`
	OutputPer1K float64 `json:"output_per_1k"`
}

// CostResult holds the computed cost and status.
type CostResult struct {
	CostUSD       float64
	Known         bool
	UnknownReason string
}

// Load reads and parses a pricing JSON file.
func Load(path string) (*Prices, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pricing file: %w", err)
	}

	var p Prices
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse pricing file: %w", err)
	}

	if p.Models == nil {
		p.Models = make(map[string]ModelPrice)
	}

	return &p, nil
}

// ComputeCost calculates the cost for a model based on token counts.
// Returns unknown if model is not in pricing table.
func (p *Prices) ComputeCost(model string, inputTokens, outputTokens int) CostResult {
	mp, ok := p.Models[model]
	if !ok {
		return CostResult{
			Known:         false,
			UnknownReason: fmt.Sprintf("model %q not in pricing table", model),
		}
	}

	cost := (float64(inputTokens)*mp.InputPer1K + float64(outputTokens)*mp.OutputPer1K) / 1000.0

	return CostResult{
		CostUSD: cost,
		Known:   true,
	}
}

// IsStale returns true if pricing data is older than the given duration.
// Also returns true if as_of date cannot be parsed.
func (p *Prices) IsStale(maxAge time.Duration) bool {
	asOf, err := time.Parse("2006-01-02", p.AsOf)
	if err != nil {
		return true
	}
	return time.Since(asOf) > maxAge
}

// StaleWarning returns a warning message if prices are stale.
func (p *Prices) StaleWarning() string {
	if p.IsStale(60 * 24 * time.Hour) { // 60 days
		return fmt.Sprintf("Pricing table may be stale (as_of: %s)", p.AsOf)
	}
	return ""
}
