// Package ledger handles recording and aggregating LLM API call data.
//
// Purpose: Write per-call records to JSONL and aggregate totals.
// Public API: Entry, Writer, Summary, Aggregator
// Usage: Create a Writer to record entries, then aggregate for summary.
package ledger

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// Entry represents a single LLM API call record.
// Raw usage fields are preserved; cost is computed externally.
type Entry struct {
	Timestamp     string                 `json:"ts"`
	Provider      string                 `json:"provider"`
	Endpoint      string                 `json:"endpoint"`
	Model         string                 `json:"model"`
	InputTokens   int                    `json:"input_tokens,omitempty"`
	OutputTokens  int                    `json:"output_tokens,omitempty"`
	RawUsage      map[string]interface{} `json:"raw_usage,omitempty"`
	CostUSD       float64                `json:"cost_usd,omitempty"`
	CostKnown     bool                   `json:"cost_known"`
	UnknownReason string                 `json:"unknown_reason,omitempty"`
	RequestID     string                 `json:"request_id,omitempty"`
	Streaming     bool                   `json:"streaming"`
}

// Summary holds aggregated statistics from all entries.
type Summary struct {
	TotalCalls        int                   `json:"total_calls"`
	KnownCostCalls    int                   `json:"known_cost_calls"`
	UnknownCostCalls  int                   `json:"unknown_cost_calls"`
	TotalKnownCostUSD float64               `json:"total_known_cost_usd"`
	TotalInputTokens  int                   `json:"total_input_tokens"`
	TotalOutputTokens int                   `json:"total_output_tokens"`
	ModelBreakdown    map[string]ModelStats `json:"model_breakdown"`
	UnknownReasons    map[string]int        `json:"unknown_reasons"`
	Warnings          []string              `json:"warnings,omitempty"`
}

// ModelStats holds per-model statistics.
type ModelStats struct {
	Calls        int     `json:"calls"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	KnownCostUSD float64 `json:"known_cost_usd"`
}

// Writer writes entries to a JSONL file.
type Writer struct {
	file *os.File
	mu   sync.Mutex
}

// NewWriter creates a new ledger writer.
// Returns error if file cannot be created.
func NewWriter(path string) (*Writer, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &Writer{file: f}, nil
}

// Write appends an entry to the ledger file.
func (w *Writer) Write(e Entry) error {
	if e.Timestamp == "" {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = w.file.Write(append(data, '\n'))
	return err
}

// Close closes the underlying file.
func (w *Writer) Close() error {
	return w.file.Close()
}

// Aggregator collects entries and computes summary statistics.
type Aggregator struct {
	entries []Entry
	mu      sync.Mutex
}

// NewAggregator creates a new aggregator.
func NewAggregator() *Aggregator {
	return &Aggregator{
		entries: make([]Entry, 0),
	}
}

// Add records an entry for aggregation.
func (a *Aggregator) Add(e Entry) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, e)
}

// Summary computes aggregated statistics from all entries.
func (a *Aggregator) Summary() Summary {
	a.mu.Lock()
	defer a.mu.Unlock()

	s := Summary{
		ModelBreakdown: make(map[string]ModelStats),
		UnknownReasons: make(map[string]int),
	}

	for _, e := range a.entries {
		s.TotalCalls++
		s.TotalInputTokens += e.InputTokens
		s.TotalOutputTokens += e.OutputTokens

		if e.CostKnown {
			s.KnownCostCalls++
			s.TotalKnownCostUSD += e.CostUSD
		} else {
			s.UnknownCostCalls++
			if e.UnknownReason != "" {
				s.UnknownReasons[e.UnknownReason]++
			}
		}

		// Update model breakdown
		ms := s.ModelBreakdown[e.Model]
		ms.Calls++
		ms.InputTokens += e.InputTokens
		ms.OutputTokens += e.OutputTokens
		if e.CostKnown {
			ms.KnownCostUSD += e.CostUSD
		}
		s.ModelBreakdown[e.Model] = ms
	}

	return s
}

// Entries returns a copy of all entries.
func (a *Aggregator) Entries() []Entry {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := make([]Entry, len(a.entries))
	copy(result, a.entries)
	return result
}

// WriteSummary writes the summary to a JSON file.
func WriteSummary(path string, s Summary) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
