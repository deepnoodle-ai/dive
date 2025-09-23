package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/internal/tablewriter"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/llm/pricing"
	"github.com/spf13/cobra"
)

// ComparisonResult holds the result of running a prompt against a single provider
type ComparisonResult struct {
	Provider      string
	Model         string
	Response      *llm.Response
	AvgLatency    time.Duration
	Error         error
	InputTokens   int
	OutputTokens  int
	TotalTokens   int
	EstCost       float64                // Estimated cost in USD
	CostBreakdown *pricing.CostBreakdown // Detailed cost breakdown
	Runs          int
}

// calculateCost calculates precise cost using the pricing library
func calculateCost(provider, model string, inputTokens, outputTokens int) (float64, *pricing.CostBreakdown) {
	calc := pricing.NewCalculator()
	breakdown, err := calc.CalculateTextCost(provider, model, inputTokens, outputTokens)
	if err != nil {
		// Fallback to simple estimation
		inputCost := float64(inputTokens) / 1000.0 * 0.002
		outputCost := float64(outputTokens) / 1000.0 * 0.006
		return inputCost + outputCost, nil
	}
	return breakdown.TotalCost, breakdown
}

// averageDuration calculates average of durations
func averageDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	total := time.Duration(0)
	for _, d := range durations {
		total += d
	}
	return total / time.Duration(len(durations))
}

// ExportResult represents the structure for JSON export
type ExportResult struct {
	Timestamp     time.Time              `json:"timestamp"`
	Prompt        string                 `json:"prompt"`
	Runs          int                    `json:"runs"`
	Provider      string                 `json:"provider"`
	Model         string                 `json:"model"`
	Status        string                 `json:"status"`
	Error         string                 `json:"error,omitempty"`
	AvgLatency    float64                `json:"avg_latency_seconds"`
	EstCost       float64                `json:"estimated_cost_usd"`
	CostBreakdown *pricing.CostBreakdown `json:"cost_breakdown,omitempty"`
	InputTokens   int                    `json:"input_tokens"`
	OutputTokens  int                    `json:"output_tokens"`
	TotalTokens   int                    `json:"total_tokens"`
	Response      string                 `json:"response_preview"`
}

// exportResults exports comparison results to JSON
func exportResults(results []*ComparisonResult, prompt string, runs int, filename string) error {
	var exportData []ExportResult
	timestamp := time.Now()

	for _, result := range results {
		exportResult := ExportResult{
			Timestamp:     timestamp,
			Prompt:        prompt,
			Runs:          runs,
			Provider:      result.Provider,
			Model:         result.Model,
			AvgLatency:    result.AvgLatency.Seconds(),
			EstCost:       result.EstCost,
			CostBreakdown: result.CostBreakdown,
			InputTokens:   result.InputTokens,
			OutputTokens:  result.OutputTokens,
			TotalTokens:   result.TotalTokens,
			Response:      extractResponsePreview(result.Response),
		}

		if result.Error != nil {
			exportResult.Status = "error"
			exportResult.Error = result.Error.Error()
		} else {
			exportResult.Status = "success"
		}

		exportData = append(exportData, exportResult)
	}

	return exportToJSON(exportData, filename)
}

// exportToJSON exports results to JSON format
func exportToJSON(data []ExportResult, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	fmt.Printf("✓ Results exported to %s\n", filename)
	return nil
}

// formatError provides a clean error message
func formatError(provider string, err error) string {
	errStr := err.Error()

	// Common error patterns
	if strings.Contains(errStr, "api key") || strings.Contains(errStr, "authentication") {
		return fmt.Sprintf("%s: API key missing/invalid", provider)
	}
	if strings.Contains(errStr, "timeout") {
		return fmt.Sprintf("%s: Request timed out", provider)
	}
	if strings.Contains(errStr, "rate limit") {
		return fmt.Sprintf("%s: Rate limited", provider)
	}
	if strings.Contains(errStr, "quota") {
		return fmt.Sprintf("%s: Quota exceeded", provider)
	}

	// Keep it short
	return fmt.Sprintf("%s: %s", provider, errStr)
}

func runComparison(ctx context.Context, prompt string, providers []string, runs int, quiet, showResponses bool) ([]*ComparisonResult, error) {
	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers specified")
	}

	if !quiet {
		fmt.Printf("Testing %d provider(s) with %d run(s) each...\n\n", len(providers), runs)
	}

	results := make([]*ComparisonResult, 0, len(providers))

	// Run prompt against each provider
	for _, providerName := range providers {
		if !quiet {
			fmt.Printf("Testing %s... ", providerName)
		}

		// Create the LLM model for this provider
		model, err := config.GetModel(providerName, "")
		if err != nil {
			errorMsg := formatError(providerName, err)
			if !quiet {
				fmt.Println(errorStyle.Sprintf("✗ %s\n", errorMsg))
			}
			results = append(results, &ComparisonResult{
				Provider: providerName,
				Error:    err,
			})
			continue
		}

		// Run multiple times and average
		var allLatencies []time.Duration
		var lastResponse *llm.Response
		var lastError error

		successfulRuns := 0
		for run := 0; run < runs; run++ {
			// Measure latency
			startTime := time.Now()

			// Create user message
			userMessage := llm.NewUserMessage(llm.NewTextContent(prompt))
			options := []llm.Option{llm.WithMessages(userMessage)}

			// Generate response
			response, err := model.Generate(ctx, options...)
			latency := time.Since(startTime)

			if err != nil {
				lastError = err
				continue
			}

			allLatencies = append(allLatencies, latency)
			lastResponse = response
			successfulRuns++
		}

		if successfulRuns == 0 {
			errorMsg := formatError(providerName, lastError)
			if !quiet {
				fmt.Println(errorStyle.Sprintf("✗ %s", errorMsg))
			}
			results = append(results, &ComparisonResult{
				Provider: providerName,
				Error:    lastError,
			})
			continue
		}

		if !quiet {
			fmt.Println(successStyle.Sprintf("✓ Success"))
		}

		// Calculate averages
		avgLatency := averageDuration(allLatencies)
		inputTokens := lastResponse.Usage.InputTokens
		outputTokens := lastResponse.Usage.OutputTokens
		totalTokens := inputTokens + outputTokens
		estCost, costBreakdown := calculateCost(providerName, lastResponse.Model, inputTokens, outputTokens)

		result := &ComparisonResult{
			Provider:      providerName,
			Model:         lastResponse.Model,
			Response:      lastResponse,
			AvgLatency:    avgLatency,
			InputTokens:   inputTokens,
			OutputTokens:  outputTokens,
			TotalTokens:   totalTokens,
			EstCost:       estCost,
			CostBreakdown: costBreakdown,
			Runs:          successfulRuns,
		}
		results = append(results, result)
	}

	// Sort results by latency (fastest first)
	sort.Slice(results, func(i, j int) bool {
		// Errors go to end
		if results[i].Error != nil && results[j].Error == nil {
			return false
		}
		if results[i].Error == nil && results[j].Error != nil {
			return true
		}
		if results[i].Error != nil && results[j].Error != nil {
			return results[i].Provider < results[j].Provider
		}
		// Sort by latency (fastest first)
		return results[i].AvgLatency < results[j].AvgLatency
	})

	// Display results
	displayResults(results, showResponses)

	return results, nil
}

func displayResults(results []*ComparisonResult, showResponses bool) {
	fmt.Println()
	fmt.Println("Provider Comparison Results (sorted by speed):")
	fmt.Println()

	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"Rank", "Provider", "Model", "Avg Time", "Est Cost", "Price/1M", "Tokens", "Status"})

	for i, result := range results {
		rank := fmt.Sprintf("#%d", i+1)
		var timeStr, costStr, priceStr, tokensStr, statusStr string
		modelName := result.Model
		if modelName == "" {
			modelName = "-"
		}

		if result.Error != nil {
			timeStr = "-"
			costStr = "-"
			priceStr = "-"
			tokensStr = "-"
			statusStr = errorStyle.Sprint("Error")
		} else {
			timeStr = fmt.Sprintf("%.2fs", result.AvgLatency.Seconds())

			// Enhanced cost display with estimation indicator
			if result.CostBreakdown != nil && result.CostBreakdown.EstimatedCost {
				costStr = fmt.Sprintf("~$%.4f*", result.EstCost)
				priceStr = "est."
			} else if result.CostBreakdown != nil {
				costStr = fmt.Sprintf("$%.4f", result.EstCost)
				priceStr = result.CostBreakdown.PricePerUnit
				if len(priceStr) > 30 {
					// Truncate long price strings for table display
					priceStr = priceStr[:27] + "..."
				}
			} else {
				costStr = fmt.Sprintf("~$%.4f*", result.EstCost)
				priceStr = "est."
			}

			tokensStr = fmt.Sprintf("%d", result.TotalTokens)
			statusStr = successStyle.Sprint("Success")
		}

		table.Append([]string{
			rank,
			result.Provider,
			modelName,
			timeStr,
			costStr,
			priceStr,
			tokensStr,
			statusStr,
		})
	}

	table.Render()

	// Show pricing note
	fmt.Println()
	fmt.Println("* Estimated pricing (model not found in pricing database)")

	// Show simple summary
	displaySimpleSummary(results)

	// Show responses if requested
	if showResponses {
		displayResponsePreviews(results)
	}
}

func displaySimpleSummary(results []*ComparisonResult) {
	fmt.Println()

	successfulResults := 0
	var fastestProvider string
	var cheapestProvider string
	minLatency := time.Hour
	minCost := 999999.0
	totalCost := 0.0

	for _, result := range results {
		if result.Error == nil {
			successfulResults++
			totalCost += result.EstCost
			if result.AvgLatency < minLatency {
				minLatency = result.AvgLatency
				fastestProvider = result.Provider
			}
			if result.EstCost < minCost {
				minCost = result.EstCost
				cheapestProvider = result.Provider
			}
		}
	}

	if successfulResults > 1 {
		fmt.Printf("• Fastest: %s (%.2fs)\n", fastestProvider, minLatency.Seconds())
		fmt.Printf("• Cheapest: %s (~$%.4f)\n", cheapestProvider, minCost)
		fmt.Printf("• Total estimated cost: ~$%.4f\n", totalCost)
	} else if successfulResults == 1 {
		fmt.Printf("• Estimated cost: ~$%.4f\n", totalCost)
	} else {
		fmt.Println("⚠ No successful responses")
	}
	fmt.Println()
}

func displayResponsePreviews(results []*ComparisonResult) {
	fmt.Println("Response Previews:")
	fmt.Println()

	for i, result := range results {
		if result.Error != nil {
			continue
		}

		fmt.Printf("#%d %s:\n", i+1, result.Provider)
		fmt.Println(strings.Repeat("-", 40))
		fmt.Println(extractResponsePreview(result.Response))
		fmt.Println()
	}
}

func extractResponsePreview(response *llm.Response) string {
	if response == nil {
		return "No response"
	}
	for _, content := range response.Content {
		if textContent, ok := content.(*llm.TextContent); ok {
			text := textContent.Text
			if len(text) > 300 {
				return text[:297] + "..."
			}
			return text
		}
	}
	return "No text content"
}

var compareCmd = &cobra.Command{
	Use:   "compare",
	Short: "Compare responses from multiple LLM providers",
	Long: `Run the same prompt across multiple providers and compare their responses, speed, and estimated costs.

Examples:
  # Basic comparison
  dive compare --prompt "What is 2+2?" --providers "anthropic,openai,grok"

  # Multiple runs for averaging
  dive compare --prompt "Explain AI" --providers "anthropic,openai" --runs 3

  # Quiet mode (table only)
  dive compare --prompt "Test" --providers "anthropic,openai" --quiet

  # Show response previews
  dive compare --prompt "Write code" --providers "anthropic,openai" --show-responses

  # Export to JSON
  dive compare --prompt "Analysis" --providers "anthropic,openai" --export results.json

Available providers: anthropic, openai, openai-completions, groq, grok, google, ollama, openrouter`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()

		// Get prompt
		prompt, err := cmd.Flags().GetString("prompt")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		if prompt == "" {
			fmt.Println(errorStyle.Sprint("Prompt is required. Use --prompt flag"))
			os.Exit(1)
		}

		// Get providers
		providersStr, err := cmd.Flags().GetString("providers")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		if providersStr == "" {
			fmt.Println(errorStyle.Sprint("Providers are required. Use --providers flag"))
			os.Exit(1)
		}

		providers := strings.Split(providersStr, ",")
		for i, provider := range providers {
			providers[i] = strings.TrimSpace(provider)
		}

		// Get runs
		runs, err := cmd.Flags().GetInt("runs")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		if runs < 1 {
			runs = 1
		}

		// Get display options
		quiet, err := cmd.Flags().GetBool("quiet")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		showResponses, err := cmd.Flags().GetBool("show-responses")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		// Get export file
		exportFile, err := cmd.Flags().GetString("export")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		// Run comparison
		results, err := runComparison(ctx, prompt, providers, runs, quiet, showResponses)
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		// Export results if requested
		if exportFile != "" {
			if err := exportResults(results, prompt, runs, exportFile); err != nil {
				fmt.Println(errorStyle.Sprintf("Export failed: %v", err))
				os.Exit(1)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(compareCmd)

	compareCmd.Flags().StringP("prompt", "p", "", "Prompt text to run across all providers (required)")
	compareCmd.Flags().StringP("providers", "", "", "Comma-separated list of providers to compare (required)")
	compareCmd.Flags().IntP("runs", "r", 1, "Number of runs per provider for averaging (default: 1)")
	compareCmd.Flags().BoolP("quiet", "q", false, "Show only results table")
	compareCmd.Flags().BoolP("show-responses", "s", false, "Show response previews")
	compareCmd.Flags().StringP("export", "e", "", "Export results to JSON file")

	compareCmd.MarkFlagRequired("prompt")
	compareCmd.MarkFlagRequired("providers")
}
