package pricing

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prices.json")

	content := `{
  "as_of": "2026-01-01",
  "models": {
    "gpt-4o": { "input_per_1k": 0.0025, "output_per_1k": 0.01 },
    "claude-3-opus": { "input_per_1k": 0.015, "output_per_1k": 0.075 }
  }
}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if p.AsOf != "2026-01-01" {
		t.Errorf("AsOf = %q, want 2026-01-01", p.AsOf)
	}

	if len(p.Models) != 2 {
		t.Errorf("len(Models) = %d, want 2", len(p.Models))
	}

	mp := p.Models["gpt-4o"]
	if mp.InputPer1K != 0.0025 {
		t.Errorf("gpt-4o InputPer1K = %f, want 0.0025", mp.InputPer1K)
	}
}

func TestComputeCost(t *testing.T) {
	p := &Prices{
		AsOf: "2026-01-01",
		Models: map[string]ModelPrice{
			"gpt-4o": {InputPer1K: 0.0025, OutputPer1K: 0.01},
		},
	}

	// Known model
	r := p.ComputeCost("gpt-4o", 1000, 500)
	if !r.Known {
		t.Error("Expected cost to be known for gpt-4o")
	}
	// cost = (1000 * 0.0025 + 500 * 0.01) / 1000 = (2.5 + 5) / 1000 = 0.0075
	expected := 0.0075
	if r.CostUSD != expected {
		t.Errorf("CostUSD = %f, want %f", r.CostUSD, expected)
	}

	// Unknown model
	r = p.ComputeCost("unknown-model", 100, 50)
	if r.Known {
		t.Error("Expected cost to be unknown for unknown-model")
	}
	if r.UnknownReason == "" {
		t.Error("Expected UnknownReason to be set")
	}
}

func TestIsStale(t *testing.T) {
	// Recent date - not stale
	p := &Prices{AsOf: time.Now().Format("2006-01-02")}
	if p.IsStale(60 * 24 * time.Hour) {
		t.Error("Expected recent prices to not be stale")
	}

	// Old date - stale
	p = &Prices{AsOf: "2020-01-01"}
	if !p.IsStale(60 * 24 * time.Hour) {
		t.Error("Expected old prices to be stale")
	}

	// Invalid date - treated as stale
	p = &Prices{AsOf: "invalid"}
	if !p.IsStale(60 * 24 * time.Hour) {
		t.Error("Expected invalid date to be treated as stale")
	}
}

func TestStaleWarning(t *testing.T) {
	// Recent - no warning
	p := &Prices{AsOf: time.Now().Format("2006-01-02")}
	if w := p.StaleWarning(); w != "" {
		t.Errorf("Expected no warning for recent prices, got %q", w)
	}

	// Old - warning
	p = &Prices{AsOf: "2020-01-01"}
	if w := p.StaleWarning(); w == "" {
		t.Error("Expected warning for old prices")
	}
}
