package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

// ComparisonResult holds the result of running a prompt against a single provider
type ComparisonResult struct {
	Provider        string
	Model           string
	Response        *llm.Response // Latest response for display
	Latency         time.Duration
	Error           error
	InputTokens     int
	OutputTokens    int
	TotalTokens     int
	Cost            float64
	TokensPerSecond float64
	ResponseLength  int
	ResponseQuality float64

	// Statistical data from multiple runs
	RunCount          int
	Latencies         []time.Duration
	Costs             []float64
	TokensPerSeconds  []float64
	ResponseQualities []float64

	// Computed statistics
	AvgLatency            time.Duration
	AvgCost               float64
	AvgTokensPerSecond    float64
	AvgResponseQuality    float64
	StdDevLatency         time.Duration
	StdDevCost            float64
	StdDevTokensPerSecond float64
	StdDevResponseQuality float64
}

// MetricType represents the sorting metric for comparison
type MetricType string

const (
	MetricSpeed   MetricType = "speed"
	MetricQuality MetricType = "quality"
	MetricCost    MetricType = "cost"
	MetricTokens  MetricType = "tokens"
)

// ProviderPricing holds pricing information for different providers
type ProviderPricing struct {
	InputTokenPrice  float64 // USD per 1K tokens
	OutputTokenPrice float64 // USD per 1K tokens
	Currency         string
}

// pricingMap contains pricing information for supported providers
var pricingMap = map[string]ProviderPricing{
	"anthropic":  {0.003, 0.015, "USD"},    // Claude pricing (approximate)
	"openai":     {0.0015, 0.002, "USD"},   // GPT pricing (approximate)
	"groq":       {0.0005, 0.0008, "USD"},  // Groq pricing (approximate)
	"grok":       {0.0005, 0.0008, "USD"},  // Grok pricing (approximate)
	"google":     {0.00025, 0.0005, "USD"}, // Gemini pricing (approximate)
	"ollama":     {0.0, 0.0, "USD"},        // Local, no cost
	"openrouter": {0.001, 0.002, "USD"},    // Approximate pricing
}

// calculateCost computes the estimated cost for a provider based on token usage
func calculateCost(provider string, inputTokens, outputTokens int) float64 {
	pricing, exists := pricingMap[provider]
	if !exists {
		return 0.0 // Unknown provider, no cost estimate
	}

	inputCost := float64(inputTokens) / 1000.0 * pricing.InputTokenPrice
	outputCost := float64(outputTokens) / 1000.0 * pricing.OutputTokenPrice
	return inputCost + outputCost
}

// calculateTokensPerSecond computes tokens per second throughput
func calculateTokensPerSecond(totalTokens int, latency time.Duration) float64 {
	if latency.Seconds() == 0 {
		return 0
	}
	return float64(totalTokens) / latency.Seconds()
}

// calculateResponseQuality provides a basic quality score based on response characteristics
func calculateResponseQuality(response *llm.Response, latency time.Duration) float64 {
	if response == nil {
		return 0.0
	}

	score := 0.0

	// Extract text content
	var textContent string
	for _, content := range response.Content {
		if text, ok := content.(*llm.TextContent); ok {
			textContent = text.Text
			break
		}
	}

	// Length score (prefer substantial responses)
	textLength := len(textContent)
	if textLength > 0 {
		score += 0.3 * min(1.0, float64(textLength)/500.0) // Max score at 500 chars
	}

	// Efficiency score (tokens per second, normalized)
	totalTokens := response.Usage.InputTokens + response.Usage.OutputTokens
	tokensPerSec := calculateTokensPerSecond(totalTokens, latency)
	score += 0.4 * min(1.0, tokensPerSec/100.0) // Max score at 100 tokens/sec

	// Completeness score (avoid very short responses)
	if textLength > 10 {
		score += 0.3
	}

	return score
}

// min returns the minimum of two float64 values
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// calculateAverageDuration calculates the average of duration slices
func calculateAverageDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	total := time.Duration(0)
	for _, d := range durations {
		total += d
	}
	return total / time.Duration(len(durations))
}

// calculateAverageFloat64 calculates the average of float64 slices
func calculateAverageFloat64(values []float64) float64 {
	if len(values) == 0 {
		return 0.0
	}
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total / float64(len(values))
}

// calculateStdDevFloat64 calculates the standard deviation of float64 slices
func calculateStdDevFloat64(values []float64) float64 {
	if len(values) <= 1 {
		return 0.0
	}
	mean := calculateAverageFloat64(values)
	sumSquares := 0.0
	for _, v := range values {
		diff := v - mean
		sumSquares += diff * diff
	}
	return math.Sqrt(sumSquares / float64(len(values)-1))
}

// calculateStdDevDuration calculates the standard deviation of duration slices
func calculateStdDevDuration(durations []time.Duration) time.Duration {
	if len(durations) <= 1 {
		return 0
	}
	mean := calculateAverageDuration(durations)
	sumSquares := float64(0)
	for _, d := range durations {
		diff := float64(d - mean)
		sumSquares += diff * diff
	}
	return time.Duration(math.Sqrt(sumSquares / float64(len(durations)-1)))
}

// hasMultipleRuns checks if any result has multiple runs for statistical analysis
func hasMultipleRuns(results []*ComparisonResult) bool {
	for _, result := range results {
		if result.RunCount > 1 {
			return true
		}
	}
	return false
}

// displayStatisticalSummary shows detailed statistical analysis when multiple runs were performed
func displayStatisticalSummary(results []*ComparisonResult) {
	fmt.Println()
	fmt.Println(boldStyle.Sprint("Statistical Summary (Multiple Runs):"))
	fmt.Println()

	// Filter out failed results
	var successfulResults []*ComparisonResult
	for _, result := range results {
		if result.Error == nil && result.RunCount > 1 {
			successfulResults = append(successfulResults, result)
		}
	}

	if len(successfulResults) == 0 {
		fmt.Println(warningStyle.Sprint("⚠ No statistical data available (all results are from single runs or failed)"))
		return
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"Provider", "Runs", "Avg Latency", "Latency ±σ", "Avg Cost ($)", "Cost ±σ", "Avg Tokens/sec", "Tokens/sec ±σ", "Avg Quality", "Quality ±σ"})

	for _, result := range successfulResults {
		avgLatencyStr := fmt.Sprintf("%.2fs", result.AvgLatency.Seconds())
		latencyStdDevStr := fmt.Sprintf("±%.2fs", result.StdDevLatency.Seconds())
		avgCostStr := fmt.Sprintf("$%.4f", result.AvgCost)
		costStdDevStr := fmt.Sprintf("±$%.4f", result.StdDevCost)
		avgTokensStr := fmt.Sprintf("%.1f", result.AvgTokensPerSecond)
		tokensStdDevStr := fmt.Sprintf("±%.1f", result.StdDevTokensPerSecond)
		avgQualityStr := fmt.Sprintf("%.2f", result.AvgResponseQuality)
		qualityStdDevStr := fmt.Sprintf("±%.2f", result.StdDevResponseQuality)

		table.Append([]string{
			result.Provider,
			fmt.Sprintf("%d", result.RunCount),
			avgLatencyStr,
			latencyStdDevStr,
			avgCostStr,
			costStdDevStr,
			avgTokensStr,
			tokensStdDevStr,
			avgQualityStr,
			qualityStdDevStr,
		})
	}

	table.Close()
	fmt.Println()
	fmt.Println(warningStyle.Sprint("💡 Higher quality scores indicate better response characteristics (length, efficiency, completeness)"))
	fmt.Println(warningStyle.Sprint("💡 Standard deviation (±σ) shows result variability - lower values indicate more consistent performance"))
}

// displayOverallSummary shows overall comparison statistics and tips
func displayOverallSummary(results []*ComparisonResult) {
	fmt.Println()
	fmt.Println(boldStyle.Sprint("Overall Summary:"))

	totalProviders := len(results)
	successfulProviders := 0
	totalRuns := 0
	totalTokens := 0
	totalCost := 0.0

	for _, result := range results {
		if result.Error == nil {
			successfulProviders++
			totalRuns += result.RunCount
			totalTokens += result.TotalTokens
			totalCost += result.AvgCost
		}
	}

	fmt.Printf("• Total providers tested: %d\n", totalProviders)
	fmt.Printf("• Successful providers: %d/%d (%.1f%%)\n", successfulProviders, totalProviders, float64(successfulProviders)/float64(totalProviders)*100)

	if successfulProviders > 0 {
		fmt.Printf("• Total runs completed: %d\n", totalRuns)
		fmt.Printf("• Total tokens used: %d\n", totalTokens)
		fmt.Printf("• Total estimated cost: $%.4f\n", totalCost)

		if successfulProviders > 1 {
			// Find best performers
			bestSpeed := ""
			bestCost := ""
			bestQuality := ""
			minLatency := time.Hour
			minCost := 999999.0
			maxQuality := 0.0

			for _, result := range results {
				if result.Error == nil {
					if result.AvgLatency < minLatency {
						minLatency = result.AvgLatency
						bestSpeed = result.Provider
					}
					if result.AvgCost < minCost {
						minCost = result.AvgCost
						bestCost = result.Provider
					}
					if result.AvgResponseQuality > maxQuality {
						maxQuality = result.AvgResponseQuality
						bestQuality = result.Provider
					}
				}
			}

			fmt.Printf("• Fastest provider: %s (%.2fs avg)\n", bestSpeed, minLatency.Seconds())
			fmt.Printf("• Most cost-effective: %s ($%.4f avg)\n", bestCost, minCost)
			fmt.Printf("• Highest quality: %s (%.2f score)\n", bestQuality, maxQuality)
		}
	}

	fmt.Println()
	fmt.Println(warningStyle.Sprint("💡 Tips:"))
	fmt.Println("  • Use --runs N for statistical reliability (recommended: 3-5 runs)")
	fmt.Println("  • Export results with --export json/csv for further analysis")
	fmt.Println("  • Different metrics help you choose the best provider for your use case")
	fmt.Println("  • Cost estimates are approximate - check provider pricing for exact rates")
}

// ExportResult represents the structure for JSON/CSV export
type ExportResult struct {
	Timestamp             time.Time `json:"timestamp"`
	Prompt                string    `json:"prompt"`
	Runs                  int       `json:"runs"`
	Metric                string    `json:"metric"`
	Provider              string    `json:"provider"`
	Model                 string    `json:"model"`
	Status                string    `json:"status"`
	Error                 string    `json:"error,omitempty"`
	AvgLatency            float64   `json:"avg_latency_seconds"`
	StdDevLatency         float64   `json:"stddev_latency_seconds"`
	AvgCost               float64   `json:"avg_cost_usd"`
	StdDevCost            float64   `json:"stddev_cost_usd"`
	AvgTokensPerSecond    float64   `json:"avg_tokens_per_second"`
	StdDevTokensPerSecond float64   `json:"stddev_tokens_per_second"`
	AvgResponseQuality    float64   `json:"avg_response_quality"`
	StdDevResponseQuality float64   `json:"stddev_response_quality"`
	InputTokens           int       `json:"input_tokens"`
	OutputTokens          int       `json:"output_tokens"`
	TotalTokens           int       `json:"total_tokens"`
	ResponseLength        int       `json:"response_length"`
	ResponsePreview       string    `json:"response_preview"`
}

// exportResults exports comparison results to the specified format
func exportResults(results []*ComparisonResult, prompt string, runs int, metric MetricType, format string, filename string) error {
	var exportData []ExportResult
	timestamp := time.Now()

	for _, result := range results {
		exportResult := ExportResult{
			Timestamp:             timestamp,
			Prompt:                prompt,
			Runs:                  runs,
			Metric:                string(metric),
			Provider:              result.Provider,
			Model:                 result.Model,
			AvgLatency:            result.AvgLatency.Seconds(),
			StdDevLatency:         result.StdDevLatency.Seconds(),
			AvgCost:               result.AvgCost,
			StdDevCost:            result.StdDevCost,
			AvgTokensPerSecond:    result.AvgTokensPerSecond,
			StdDevTokensPerSecond: result.StdDevTokensPerSecond,
			AvgResponseQuality:    result.AvgResponseQuality,
			StdDevResponseQuality: result.StdDevResponseQuality,
			InputTokens:           result.InputTokens,
			OutputTokens:          result.OutputTokens,
			TotalTokens:           result.TotalTokens,
			ResponseLength:        result.ResponseLength,
			ResponsePreview:       extractResponsePreview(result.Response),
		}

		if result.Error != nil {
			exportResult.Status = "error"
			exportResult.Error = result.Error.Error()
		} else {
			exportResult.Status = "success"
		}

		exportData = append(exportData, exportResult)
	}

	switch strings.ToLower(format) {
	case "json":
		return exportToJSON(exportData, filename)
	case "csv":
		return exportToCSV(exportData, filename)
	default:
		return fmt.Errorf("unsupported export format: %s (supported: json, csv)", format)
	}
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

	fmt.Printf(successStyle.Sprintf("✓ Results exported to %s\n"), filename)
	return nil
}

// exportToCSV exports results to CSV format
func exportToCSV(data []ExportResult, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{
		"timestamp", "prompt", "runs", "metric", "provider", "model", "status", "error",
		"avg_latency_seconds", "stddev_latency_seconds", "avg_cost_usd", "stddev_cost_usd",
		"avg_tokens_per_second", "stddev_tokens_per_second", "avg_response_quality", "stddev_response_quality",
		"input_tokens", "output_tokens", "total_tokens", "response_length", "response_preview",
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write data rows
	for _, result := range data {
		row := []string{
			result.Timestamp.Format(time.RFC3339),
			result.Prompt,
			fmt.Sprintf("%d", result.Runs),
			result.Metric,
			result.Provider,
			result.Model,
			result.Status,
			result.Error,
			fmt.Sprintf("%.3f", result.AvgLatency),
			fmt.Sprintf("%.3f", result.StdDevLatency),
			fmt.Sprintf("%.6f", result.AvgCost),
			fmt.Sprintf("%.6f", result.StdDevCost),
			fmt.Sprintf("%.2f", result.AvgTokensPerSecond),
			fmt.Sprintf("%.2f", result.StdDevTokensPerSecond),
			fmt.Sprintf("%.3f", result.AvgResponseQuality),
			fmt.Sprintf("%.3f", result.StdDevResponseQuality),
			fmt.Sprintf("%d", result.InputTokens),
			fmt.Sprintf("%d", result.OutputTokens),
			fmt.Sprintf("%d", result.TotalTokens),
			fmt.Sprintf("%d", result.ResponseLength),
			result.ResponsePreview,
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	fmt.Printf(successStyle.Sprintf("✓ Results exported to %s\n"), filename)
	return nil
}

// enhanceErrorMessage provides actionable suggestions for common errors
func enhanceErrorMessage(provider string, err error) string {
	errStr := err.Error()

	switch provider {
	case "anthropic":
		if strings.Contains(errStr, "api key") || strings.Contains(errStr, "authentication") {
			return fmt.Sprintf("Anthropic API key missing or invalid. Set ANTHROPIC_API_KEY environment variable. Get your key from https://console.anthropic.com/")
		}
	case "openai":
		if strings.Contains(errStr, "api key") || strings.Contains(errStr, "authentication") {
			return fmt.Sprintf("OpenAI API key missing or invalid. Set OPENAI_API_KEY environment variable. Get your key from https://platform.openai.com/api-keys")
		}
	case "groq":
		if strings.Contains(errStr, "api key") || strings.Contains(errStr, "authentication") {
			return fmt.Sprintf("Groq API key missing or invalid. Set GROQ_API_KEY environment variable. Get your key from https://console.groq.com/keys")
		}
	case "grok":
		if strings.Contains(errStr, "api key") || strings.Contains(errStr, "authentication") {
			return fmt.Sprintf("Grok API key missing or invalid. Set GROK_API_KEY environment variable. Get your key from https://console.x.ai/")
		}
	case "google":
		if strings.Contains(errStr, "api key") || strings.Contains(errStr, "authentication") {
			return fmt.Sprintf("Google AI API key missing or invalid. Set GOOGLE_AI_API_KEY environment variable. Get your key from https://makersuite.google.com/app/apikey")
		}
	case "ollama":
		if strings.Contains(errStr, "model") && strings.Contains(errStr, "not found") {
			return fmt.Sprintf("Ollama model not found. Install and pull the model first: 'ollama pull llama3.2' or 'ollama pull mistral'")
		}
		if strings.Contains(errStr, "connection") {
			return fmt.Sprintf("Cannot connect to Ollama. Make sure Ollama is running: 'ollama serve'")
		}
	case "openrouter":
		if strings.Contains(errStr, "api key") || strings.Contains(errStr, "authentication") {
			return fmt.Sprintf("OpenRouter API key missing or invalid. Set OPENROUTER_API_KEY environment variable. Get your key from https://openrouter.ai/keys")
		}
	}

	// Generic error messages
	if strings.Contains(errStr, "timeout") {
		return fmt.Sprintf("%s provider timed out. Try again or check your internet connection", strings.Title(provider))
	}
	if strings.Contains(errStr, "rate limit") {
		return fmt.Sprintf("%s provider rate limited. Wait a moment before retrying", strings.Title(provider))
	}
	if strings.Contains(errStr, "quota") {
		return fmt.Sprintf("%s provider quota exceeded. Check your usage limits", strings.Title(provider))
	}

	// Default case
	return fmt.Sprintf("%s provider error: %v", strings.Title(provider), err)
}

func runComparison(ctx context.Context, prompt string, providers []string, metric MetricType, runs int) ([]*ComparisonResult, error) {
	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers specified")
	}

	fmt.Println(boldStyle.Sprintf("Running prompt across %d provider(s) with %d run(s) each...", len(providers), runs))
	fmt.Println()

	results := make([]*ComparisonResult, 0, len(providers))

	// Run prompt against each provider
	for _, providerName := range providers {
		fmt.Printf("Testing %s... ", providerName)

		// Create the LLM model for this provider
		model, err := config.GetModel(providerName, "")
		if err != nil {
			errorMsg := enhanceErrorMessage(providerName, err)
			fmt.Printf(errorStyle.Sprintf("✗ %s\n", errorMsg))
			results = append(results, &ComparisonResult{
				Provider: providerName,
				Error:    fmt.Errorf("failed to create provider: %w", err),
			})
			continue
		}

		// Run multiple times for statistical analysis
		var allLatencies []time.Duration
		var allCosts []float64
		var allTokensPerSeconds []float64
		var allResponseQualities []float64
		var lastResponse *llm.Response
		var lastError error

		successfulRuns := 0
		for run := 0; run < runs; run++ {
			if runs > 1 {
				fmt.Printf("Run %d/%d... ", run+1, runs)
			}

			// Measure latency
			startTime := time.Now()

			// Create user message
			userMessage := llm.NewUserMessage(llm.NewTextContent(prompt))
			options := []llm.Option{llm.WithMessages(userMessage)}

			// Generate response
			response, err := model.Generate(ctx, options...)
			latency := time.Since(startTime)

			if err != nil {
				if runs > 1 {
					fmt.Printf(errorStyle.Sprintf("✗ "))
				}
				lastError = fmt.Errorf("generation failed: %w", err)
				continue
			}

			if runs > 1 {
				fmt.Printf(successStyle.Sprint("✓ "))
			}

			// Extract metrics
			inputTokens := response.Usage.InputTokens
			outputTokens := response.Usage.OutputTokens
			totalTokens := inputTokens + outputTokens
			cost := calculateCost(providerName, inputTokens, outputTokens)
			tokensPerSecond := calculateTokensPerSecond(totalTokens, latency)
			qualityScore := calculateResponseQuality(response, latency)

			// Store metrics for this run
			allLatencies = append(allLatencies, latency)
			allCosts = append(allCosts, cost)
			allTokensPerSeconds = append(allTokensPerSeconds, tokensPerSecond)
			allResponseQualities = append(allResponseQualities, qualityScore)

			lastResponse = response
			successfulRuns++

			if runs > 1 {
				fmt.Printf("%.2fs ", latency.Seconds())
			}
		}

		if successfulRuns == 0 {
			fmt.Printf(errorStyle.Sprintf("✗ All runs failed: %v\n", lastError))
			results = append(results, &ComparisonResult{
				Provider: providerName,
				Error:    lastError,
			})
			continue
		}

		fmt.Printf(successStyle.Sprintf("✓ Success (%d/%d runs)\n", successfulRuns, runs))

		// Calculate statistics
		avgLatency := calculateAverageDuration(allLatencies)
		avgCost := calculateAverageFloat64(allCosts)
		avgTokensPerSecond := calculateAverageFloat64(allTokensPerSeconds)
		avgResponseQuality := calculateAverageFloat64(allResponseQualities)

		stdDevLatency := calculateStdDevDuration(allLatencies)
		stdDevCost := calculateStdDevFloat64(allCosts)
		stdDevTokensPerSecond := calculateStdDevFloat64(allTokensPerSeconds)
		stdDevResponseQuality := calculateStdDevFloat64(allResponseQualities)

		// Extract final metrics for display
		inputTokens := lastResponse.Usage.InputTokens
		outputTokens := lastResponse.Usage.OutputTokens
		totalTokens := inputTokens + outputTokens
		responseLength := 0
		for _, content := range lastResponse.Content {
			if text, ok := content.(*llm.TextContent); ok {
				responseLength = len(text.Text)
				break
			}
		}

		result := &ComparisonResult{
			Provider:        providerName,
			Model:           lastResponse.Model,
			Response:        lastResponse,
			Latency:         avgLatency, // Use average for single-run display
			InputTokens:     inputTokens,
			OutputTokens:    outputTokens,
			TotalTokens:     totalTokens,
			Cost:            avgCost,
			TokensPerSecond: avgTokensPerSecond,
			ResponseLength:  responseLength,
			ResponseQuality: avgResponseQuality,

			// Statistical data
			RunCount:          successfulRuns,
			Latencies:         allLatencies,
			Costs:             allCosts,
			TokensPerSeconds:  allTokensPerSeconds,
			ResponseQualities: allResponseQualities,

			// Computed statistics
			AvgLatency:            avgLatency,
			AvgCost:               avgCost,
			AvgTokensPerSecond:    avgTokensPerSecond,
			AvgResponseQuality:    avgResponseQuality,
			StdDevLatency:         stdDevLatency,
			StdDevCost:            stdDevCost,
			StdDevTokensPerSecond: stdDevTokensPerSecond,
			StdDevResponseQuality: stdDevResponseQuality,
		}
		results = append(results, result)
	}

	// Sort results based on metric
	sortResults(results, metric)

	// Display comparison table
	displayComparisonTable(results, metric)

	return results, nil
}

func sortResults(results []*ComparisonResult, metric MetricType) {
	sort.Slice(results, func(i, j int) bool {
		// Always put errors at the end
		if results[i].Error != nil && results[j].Error == nil {
			return false
		}
		if results[i].Error == nil && results[j].Error != nil {
			return true
		}
		if results[i].Error != nil && results[j].Error != nil {
			return results[i].Provider < results[j].Provider
		}

		switch metric {
		case MetricSpeed:
			return results[i].AvgLatency < results[j].AvgLatency
		case MetricQuality:
			return results[i].AvgResponseQuality > results[j].AvgResponseQuality
		case MetricCost:
			return results[i].AvgCost < results[j].AvgCost
		case MetricTokens:
			return results[i].AvgTokensPerSecond > results[j].AvgTokensPerSecond
		default:
			return results[i].Provider < results[j].Provider
		}
	})
}

func displayComparisonTable(results []*ComparisonResult, metric MetricType) {
	fmt.Println()
	fmt.Println(boldStyle.Sprintf("Provider Comparison Results (sorted by %s):", string(metric)))
	fmt.Println()

	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"Rank", "Provider", "Model", "Latency", "Cost ($)", "Tokens/sec", "Quality", "Input Tokens", "Output Tokens", "Total Tokens", "Status", "Response Preview"})

	for i, result := range results {
		rank := fmt.Sprintf("#%d", i+1)

		var latencyStr, costStr, tokensPerSecStr, qualityStr, inputTokensStr, outputTokensStr, totalTokensStr, statusStr, responsePreview string

		if result.Error != nil {
			latencyStr = "-"
			costStr = "-"
			tokensPerSecStr = "-"
			qualityStr = "-"
			inputTokensStr = "-"
			outputTokensStr = "-"
			totalTokensStr = "-"
			statusStr = errorStyle.Sprint("Error")
			responsePreview = result.Error.Error()
		} else {
			latencyStr = fmt.Sprintf("%.2fs", result.Latency.Seconds())
			costStr = fmt.Sprintf("$%.4f", result.Cost)
			tokensPerSecStr = fmt.Sprintf("%.1f", result.TokensPerSecond)
			qualityStr = fmt.Sprintf("%.2f", result.ResponseQuality)
			inputTokensStr = fmt.Sprintf("%d", result.InputTokens)
			outputTokensStr = fmt.Sprintf("%d", result.OutputTokens)
			totalTokensStr = fmt.Sprintf("%d", result.TotalTokens)
			statusStr = successStyle.Sprint("Success")

			// Extract first text content for preview
			responsePreview = extractResponsePreview(result.Response)
		}

		modelName := result.Model
		if modelName == "" {
			modelName = "-"
		}

		table.Append([]string{
			rank,
			result.Provider,
			modelName,
			latencyStr,
			costStr,
			tokensPerSecStr,
			qualityStr,
			inputTokensStr,
			outputTokensStr,
			totalTokensStr,
			statusStr,
			responsePreview,
		})
	}

	table.Close()

	// Show statistical summary if multiple runs were performed
	if hasMultipleRuns(results) {
		displayStatisticalSummary(results)
	}

	// Show detailed responses if there are successful results
	successfulResults := 0
	for _, result := range results {
		if result.Error == nil {
			successfulResults++
		}
	}

	if successfulResults > 0 {
		fmt.Println()
		fmt.Println(boldStyle.Sprint("Detailed Responses:"))
		fmt.Println()

		for i, result := range results {
			if result.Error != nil {
				continue
			}

			fmt.Printf("%s %s (%s)\n", boldStyle.Sprintf("#%d", i+1), result.Provider, result.Model)
			fmt.Println(strings.Repeat("-", 50))

			for _, content := range result.Response.Content {
				switch content := content.(type) {
				case *llm.TextContent:
					fmt.Println(content.Text)
				case *llm.ToolUseContent:
					fmt.Printf("[Tool Use: %s]\n%s\n", content.Name, string(content.Input))
				}
			}
			fmt.Println()
		}
	} else {
		fmt.Println()
		fmt.Println(warningStyle.Sprint("⚠ No successful responses to display"))
		fmt.Println("💡 Make sure your providers are properly configured with API keys")
	}

	// Show summary statistics
	displayOverallSummary(results)
}

func extractResponsePreview(response *llm.Response) string {
	for _, content := range response.Content {
		if textContent, ok := content.(*llm.TextContent); ok {
			text := textContent.Text
			if len(text) > 100 {
				return text[:97] + "..."
			}
			return text
		}
	}
	return "No text content"
}

var compareCmd = &cobra.Command{
	Use:   "compare",
	Short: "Compare responses from multiple LLM providers",
	Long: `Run the same prompt across multiple providers and compare their responses, latencies, costs, and performance metrics with statistical analysis.

Examples:
  # Compare speed across multiple providers (single run)
  dive compare --prompt "What is 2+2?" --providers "anthropic,openai,groq" --metric speed

  # Compare with statistical analysis (3 runs per provider)
  dive compare --prompt "Explain quantum computing" --providers "anthropic,openai" --runs 3 --metric speed

  # Compare cost efficiency with multiple runs for reliability
  dive compare --prompt "Write a summary" --providers "anthropic,openai,groq" --runs 5 --metric cost

  # Compare token throughput (tokens per second)
  dive compare --prompt "Write a short story" --providers "anthropic,openai" --metric tokens --runs 3

  # Compare response quality with statistical confidence
  dive compare --prompt "Analyze this data: [data]" --providers "anthropic,openai,google" --metric quality --runs 5

  # Test all available providers
  dive compare --prompt "Hello world" --providers "anthropic,openai,groq,grok,google,ollama" --runs 2

  # Export results to JSON for further analysis
  dive compare --prompt "Explain AI" --providers "anthropic,openai" --runs 3 --export json

  # Export to CSV with custom filename
  dive compare --prompt "Code review this function" --providers "anthropic,openai,groq" --runs 5 --export csv --output my_comparison.csv

Available providers: anthropic, openai, openai-completions, groq, grok, google, ollama, openrouter
Available metrics: speed, quality, cost, tokens
Export formats: json, csv`,
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

		// Get metric
		metricStr, err := cmd.Flags().GetString("metric")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		var metric MetricType
		switch strings.ToLower(metricStr) {
		case "speed":
			metric = MetricSpeed
		case "quality":
			metric = MetricQuality
		case "cost":
			metric = MetricCost
		case "tokens":
			metric = MetricTokens
		case "":
			metric = MetricSpeed // Default to speed
		default:
			fmt.Println(errorStyle.Sprintf("Invalid metric '%s'. Valid options: speed, quality, cost, tokens", metricStr))
			os.Exit(1)
		}

		// Get runs
		runs, err := cmd.Flags().GetInt("runs")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		if runs < 1 {
			runs = 1 // Minimum 1 run
		}

		// Get export options
		exportFormat, err := cmd.Flags().GetString("export")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		exportFile, err := cmd.Flags().GetString("output")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		// Run comparison
		results, err := runComparison(ctx, prompt, providers, metric, runs)
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		// Export results if requested
		if exportFormat != "" {
			if exportFile == "" {
				timestamp := time.Now().Format("2006-01-02_15-04-05")
				exportFile = fmt.Sprintf("dive_comparison_%s.%s", timestamp, exportFormat)
			}
			if err := exportResults(results, prompt, runs, metric, exportFormat, exportFile); err != nil {
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
	compareCmd.Flags().StringP("metric", "", "speed", "Metric to sort by: 'speed', 'quality', 'cost', or 'tokens' (default: speed)")
	compareCmd.Flags().IntP("runs", "r", 1, "Number of runs per provider for statistical analysis (default: 1)")
	compareCmd.Flags().StringP("export", "e", "", "Export format: 'json' or 'csv' (optional)")
	compareCmd.Flags().StringP("output", "o", "", "Output filename for export (optional, auto-generated if not specified)")

	compareCmd.MarkFlagRequired("prompt")
	compareCmd.MarkFlagRequired("providers")
}
