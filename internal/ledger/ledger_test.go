package ledger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	w, err := NewWriter(path)
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}

	entry := Entry{
		Provider:     "openai",
		Endpoint:     "/v1/chat/completions",
		Model:        "gpt-4o",
		InputTokens:  100,
		OutputTokens: 50,
		CostUSD:      0.001,
		CostKnown:    true,
		Streaming:    false,
	}

	if err := w.Write(entry); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	w.Close()

	// Read and verify
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var parsed Entry
	if err := json.Unmarshal(data[:len(data)-1], &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if parsed.Provider != "openai" {
		t.Errorf("Provider = %q, want openai", parsed.Provider)
	}
	if parsed.Model != "gpt-4o" {
		t.Errorf("Model = %q, want gpt-4o", parsed.Model)
	}
	if parsed.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", parsed.InputTokens)
	}
}

func TestAggregator(t *testing.T) {
	agg := NewAggregator()

	agg.Add(Entry{
		Model:        "gpt-4o",
		InputTokens:  100,
		OutputTokens: 50,
		CostUSD:      0.01,
		CostKnown:    true,
	})

	agg.Add(Entry{
		Model:         "gpt-4o",
		InputTokens:   200,
		OutputTokens:  100,
		CostKnown:     false,
		UnknownReason: "missing usage",
	})

	agg.Add(Entry{
		Model:        "claude-3-opus",
		InputTokens:  50,
		OutputTokens: 25,
		CostUSD:      0.005,
		CostKnown:    true,
	})

	s := agg.Summary()

	if s.TotalCalls != 3 {
		t.Errorf("TotalCalls = %d, want 3", s.TotalCalls)
	}
	if s.KnownCostCalls != 2 {
		t.Errorf("KnownCostCalls = %d, want 2", s.KnownCostCalls)
	}
	if s.UnknownCostCalls != 1 {
		t.Errorf("UnknownCostCalls = %d, want 1", s.UnknownCostCalls)
	}
	if s.TotalInputTokens != 350 {
		t.Errorf("TotalInputTokens = %d, want 350", s.TotalInputTokens)
	}
	if s.TotalOutputTokens != 175 {
		t.Errorf("TotalOutputTokens = %d, want 175", s.TotalOutputTokens)
	}

	// Check model breakdown
	gpt4Stats := s.ModelBreakdown["gpt-4o"]
	if gpt4Stats.Calls != 2 {
		t.Errorf("gpt-4o calls = %d, want 2", gpt4Stats.Calls)
	}

	// Check unknown reasons
	if s.UnknownReasons["missing usage"] != 1 {
		t.Errorf("missing usage count = %d, want 1", s.UnknownReasons["missing usage"])
	}
}

func TestWriteSummary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "summary.json")

	s := Summary{
		TotalCalls:        5,
		TotalKnownCostUSD: 0.123,
	}

	if err := WriteSummary(path, s); err != nil {
		t.Fatalf("WriteSummary failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var parsed Summary
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if parsed.TotalCalls != 5 {
		t.Errorf("TotalCalls = %d, want 5", parsed.TotalCalls)
	}
}
