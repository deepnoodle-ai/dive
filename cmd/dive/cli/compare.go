package cli

import (
	"context"
	"fmt"
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
	Provider     string
	Model        string
	Response     *llm.Response
	Latency      time.Duration
	Error        error
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// MetricType represents the sorting metric for comparison
type MetricType string

const (
	MetricSpeed   MetricType = "speed"
	MetricQuality MetricType = "quality"
)

func runComparison(ctx context.Context, prompt string, providers []string, metric MetricType) error {
	if len(providers) == 0 {
		return fmt.Errorf("no providers specified")
	}

	fmt.Println(boldStyle.Sprintf("Running prompt across %d provider(s)...", len(providers)))
	fmt.Println()

	results := make([]*ComparisonResult, 0, len(providers))

	// Run prompt against each provider
	for _, providerName := range providers {
		fmt.Printf("Testing %s... ", providerName)
		
		// Create the LLM model for this provider
		model, err := config.GetModel(providerName, "")
		if err != nil {
			fmt.Printf(errorStyle.Sprintf("✗ Failed to create provider: %v\n", err))
			results = append(results, &ComparisonResult{
				Provider: providerName,
				Error:    fmt.Errorf("failed to create provider: %w", err),
			})
			continue
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
			fmt.Printf(errorStyle.Sprintf("✗ Failed: %v\n", err))
			results = append(results, &ComparisonResult{
				Provider: providerName,
				Latency:  latency,
				Error:    fmt.Errorf("generation failed: %w", err),
			})
			continue
		}

		fmt.Printf(successStyle.Sprint("✓ Success\n"))

		// Extract metrics
		result := &ComparisonResult{
			Provider:     providerName,
			Model:        response.Model,
			Response:     response,
			Latency:      latency,
			InputTokens:  response.Usage.InputTokens,
			OutputTokens: response.Usage.OutputTokens,
			TotalTokens:  response.Usage.InputTokens + response.Usage.OutputTokens,
		}
		results = append(results, result)
	}

	// Sort results based on metric
	sortResults(results, metric)

	// Display comparison table
	displayComparisonTable(results, metric)

	return nil
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
			return results[i].Latency < results[j].Latency
		case MetricQuality:
			// For quality, sort by output tokens (more comprehensive responses first)
			// This is a simple heuristic; in practice, quality would need human evaluation
			return results[i].OutputTokens > results[j].OutputTokens
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
	table.Header([]string{"Rank", "Provider", "Model", "Latency", "Input Tokens", "Output Tokens", "Total Tokens", "Status", "Response Preview"})

	for i, result := range results {
		rank := fmt.Sprintf("#%d", i+1)
		
		var latencyStr, inputTokensStr, outputTokensStr, totalTokensStr, statusStr, responsePreview string
		
		if result.Error != nil {
			latencyStr = "-"
			inputTokensStr = "-"
			outputTokensStr = "-"
			totalTokensStr = "-"
			statusStr = errorStyle.Sprint("Error")
			responsePreview = result.Error.Error()
		} else {
			latencyStr = fmt.Sprintf("%.2fs", result.Latency.Seconds())
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
			inputTokensStr,
			outputTokensStr,
			totalTokensStr,
			statusStr,
			responsePreview,
		})
	}

	table.Close()

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
	Long: `Run the same prompt across multiple providers and compare their responses, latencies, and costs.

Examples:
  # Compare speed across multiple providers
  dive compare --prompt "What is 2+2?" --providers "anthropic,openai,groq" --metric speed
  
  # Compare quality (response comprehensiveness) 
  dive compare --prompt "Explain quantum computing" --providers "anthropic,openai" --metric quality
  
  # Test all available providers
  dive compare --prompt "Hello world" --providers "anthropic,openai,groq,grok,google,ollama"

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
		case "":
			metric = MetricSpeed // Default to speed
		default:
			fmt.Println(errorStyle.Sprintf("Invalid metric '%s'. Valid options: speed, quality", metricStr))
			os.Exit(1)
		}

		// Run comparison
		if err := runComparison(ctx, prompt, providers, metric); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(compareCmd)

	compareCmd.Flags().StringP("prompt", "p", "", "Prompt text to run across all providers (required)")
	compareCmd.Flags().StringP("providers", "", "", "Comma-separated list of providers to compare (required)")
	compareCmd.Flags().StringP("metric", "", "speed", "Metric to sort by: 'speed' or 'quality' (default: speed)")
	
	compareCmd.MarkFlagRequired("prompt")
	compareCmd.MarkFlagRequired("providers")
}