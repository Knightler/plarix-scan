// Package main implements the plarix-scan CLI.
//
// Purpose: Entry point for the Plarix Scan GitHub Action CLI.
// Public API: `run` subcommand with --command flag.
// Usage: plarix-scan run --command "pytest -q"
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"plarix-action/internal/action"
	"plarix-action/internal/ledger"
	"plarix-action/internal/pricing"
	"plarix-action/internal/proxy"
)

const version = "0.4.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		if err := runCmd(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "version", "--version", "-v":
		fmt.Printf("plarix-scan v%s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Usage: plarix-scan <command> [options]

Commands:
  run       Run a command with LLM API cost tracking
  version   Print version information
  help      Show this help message

Run Options:
  --command <string>   Command to execute (required)
  --pricing <path>     Path to custom pricing JSON
  --fail-on-cost <float>   Exit non-zero if cost exceeds threshold (USD)
  --providers <csv>    Providers to intercept (default: openai,anthropic,openrouter)
  --comment <mode>     Comment mode: pr, summary, both (default: both)
  --enable-openai-stream-usage-injection <bool>   Opt-in for OpenAI stream usage (default: false)`)
}

func runCmd(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)

	command := fs.String("command", "", "Command to execute (required)")
	pricingPath := fs.String("pricing", "", "Path to custom pricing JSON")
	failOnCost := fs.Float64("fail-on-cost", 0, "Exit non-zero if cost exceeds threshold (USD)")
	providers := fs.String("providers", "openai,anthropic,openrouter", "Providers to intercept")
	commentMode := fs.String("comment", "both", "Comment mode: pr, summary, both")
	_ = fs.Bool("enable-openai-stream-usage-injection", false, "Opt-in for OpenAI stream usage")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Get command from flag or env
	if *command == "" {
		if envCmd := os.Getenv("INPUT_COMMAND"); envCmd != "" {
			*command = envCmd
		} else {
			return fmt.Errorf("--command is required")
		}
	}

	// Load pricing
	prices, err := loadPricing(*pricingPath)
	if err != nil {
		return fmt.Errorf("load pricing: %w", err)
	}

	// Create aggregator and writer
	agg := ledger.NewAggregator()
	writer, err := ledger.NewWriter("plarix-ledger.jsonl")
	if err != nil {
		return fmt.Errorf("create ledger writer: %w", err)
	}
	defer writer.Close()

	// Start proxy
	proxyConfig := proxy.Config{
		Providers: strings.Split(*providers, ","),
		OnEntry: func(e ledger.Entry) {
			// Compute cost
			if e.CostKnown && e.Model != "" {
				result := prices.ComputeCost(e.Model, e.InputTokens, e.OutputTokens)
				if result.Known {
					e.CostUSD = result.CostUSD
				} else {
					e.CostKnown = false
					e.UnknownReason = result.UnknownReason
				}
			}

			// Record
			agg.Add(e)
			if err := writer.Write(e); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to write ledger entry: %v\n", err)
			}
		},
	}

	server := proxy.NewServer(proxyConfig)
	port, err := server.Start()
	if err != nil {
		return fmt.Errorf("start proxy: %w", err)
	}
	defer server.Stop()

	fmt.Printf("Plarix proxy started on port %d\n", port)

	// Set environment variables for provider SDKs
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	envVars := map[string]string{
		"OPENAI_BASE_URL":     baseURL + "/openai",
		"OPENAI_API_BASE":     baseURL + "/openai",
		"ANTHROPIC_BASE_URL":  baseURL + "/anthropic",
		"OPENROUTER_BASE_URL": baseURL + "/openrouter",
	}

	// Run command
	cmdErr := runUserCommand(*command, envVars)

	// Get summary
	summary := agg.Summary()

	// Add staleness warning if applicable
	if w := prices.StaleWarning(); w != "" {
		summary.Warnings = append(summary.Warnings, w)
	}

	// Write summary file
	if err := ledger.WriteSummary("plarix-summary.json", summary); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write summary: %v\n", err)
	}

	// Generate report
	report := generateReport(summary, prices.AsOf)

	// Output based on comment mode
	if *commentMode == "summary" || *commentMode == "both" {
		if err := action.WriteStepSummary(report); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write step summary: %v\n", err)
		}
	}

	// Post PR comment if in PR context
	if *commentMode == "pr" || *commentMode == "both" {
		if pr := action.GetPRInfo(); pr != nil {
			if err := action.PostComment(pr, report); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to post PR comment: %v\n", err)
			} else {
				fmt.Println("Posted/updated PR comment")
			}
		} else {
			fmt.Println("Not in PR context, skipping PR comment")
		}
	}

	fmt.Println(report)

	// Check cost threshold
	if *failOnCost > 0 && summary.TotalKnownCostUSD > *failOnCost {
		return fmt.Errorf("cost threshold exceeded: $%.4f > $%.4f", summary.TotalKnownCostUSD, *failOnCost)
	}

	// Return command error if any
	if cmdErr != nil {
		return fmt.Errorf("command failed: %w", cmdErr)
	}

	return nil
}

func loadPricing(customPath string) (*pricing.Prices, error) {
	path := customPath
	if path == "" {
		// Try to find bundled prices.json
		exe, _ := os.Executable()
		candidates := []string{
			filepath.Join(filepath.Dir(exe), "prices", "prices.json"),
			filepath.Join(filepath.Dir(exe), "..", "prices", "prices.json"),
			"prices/prices.json",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				path = c
				break
			}
		}
	}
	if path == "" {
		return nil, fmt.Errorf("pricing file not found")
	}
	return pricing.Load(path)
}

func runUserCommand(command string, envVars map[string]string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Copy current env and add overrides
	cmd.Env = os.Environ()
	for k, v := range envVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	return cmd.Run()
}

func generateReport(s ledger.Summary, pricesAsOf string) string {
	var b strings.Builder

	b.WriteString("## 💰 Plarix Scan Cost Report\n\n")
	fmt.Fprintf(&b, "**Total Known Cost:** $%.4f USD\n", s.TotalKnownCostUSD)
	fmt.Fprintf(&b, "**Calls Observed:** %d\n", s.TotalCalls)
	fmt.Fprintf(&b, "**Tokens:** %d in / %d out\n\n", s.TotalInputTokens, s.TotalOutputTokens)

	if s.UnknownCostCalls > 0 {
		fmt.Fprintf(&b, "⚠️ **Unknown Cost Calls:** %d\n", s.UnknownCostCalls)
		if len(s.UnknownReasons) > 0 {
			for reason, count := range s.UnknownReasons {
				fmt.Fprintf(&b, "  - %s: %d\n", reason, count)
			}
		}
		b.WriteString("\n")
	}

	if s.TotalCalls == 0 {
		b.WriteString("ℹ️ No real provider calls observed. Tests may be stubbed.\n\n")
	}

	// Model breakdown table (top 6)
	if len(s.ModelBreakdown) > 0 {
		b.WriteString("| Model | Calls | Tokens (in/out) | Known Cost |\n")
		b.WriteString("|-------|-------|-----------------|------------|\n")

		count := 0
		for model, stats := range s.ModelBreakdown {
			if count >= 6 {
				break
			}
			fmt.Fprintf(&b, "| %s | %d | %d / %d | $%.4f |\n",
				model, stats.Calls, stats.InputTokens, stats.OutputTokens, stats.KnownCostUSD)
			count++
		}
		b.WriteString("\n")
	}

	// Warnings
	for _, w := range s.Warnings {
		b.WriteString(w + "\n")
	}

	// Footer
	fmt.Fprintf(&b, "\n---\n*Plarix Scan v%s | Prices as of %s | %s*\n",
		version, pricesAsOf, time.Now().UTC().Format("2006-01-02 15:04 UTC"))

	return b.String()
}
