package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getstingrai/dive"
)

func main() {
	var yamlFile string
	var verbose bool
	var outputDir string
	var timeout string

	flag.StringVar(&yamlFile, "file", "", "Path to the YAML definition file")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose output")
	flag.StringVar(&outputDir, "output", "output", "Directory to save task results")
	flag.StringVar(&timeout, "timeout", "30m", "Timeout for the entire operation")
	flag.Parse()

	if yamlFile == "" {
		fmt.Println("Error: YAML file path is required")
		fmt.Println("Usage: yaml_runner -file=<path_to_yaml_file>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Parse timeout
	timeoutDuration, err := time.ParseDuration(timeout)
	if err != nil {
		log.Fatalf("Invalid timeout format: %v", err)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeoutDuration)
	defer cancel()

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Load and run the team
	fmt.Printf("Loading team definition from %s...\n", yamlFile)
	startTime := time.Now()

	results, err := dive.LoadAndRunTeam(ctx, yamlFile)
	if err != nil {
		log.Fatalf("Failed to run team: %v", err)
	}

	elapsedTime := time.Since(startTime)
	fmt.Printf("Team execution completed in %s\n", elapsedTime)

	// Save results to files
	for _, result := range results {
		taskName := result.Task.Name()
		sanitizedName := sanitizeFilename(taskName)
		filename := filepath.Join(outputDir, sanitizedName+".txt")

		if err := os.WriteFile(filename, []byte(result.Content), 0644); err != nil {
			log.Fatalf("Failed to write result to file: %v", err)
		}

		fmt.Printf("Task %q result saved to %s\n", taskName, filename)

		if verbose {
			fmt.Printf("\n--- Task %q Result ---\n", taskName)
			fmt.Println(result.Content)
			fmt.Println("------------------------")
		}
	}

	fmt.Printf("\nAll tasks completed successfully. Results saved to %s/\n", outputDir)
}

// sanitizeFilename removes or replaces characters that are invalid in filenames
func sanitizeFilename(filename string) string {
	// Replace invalid characters with underscores
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := filename

	for _, char := range invalid {
		result = strings.ReplaceAll(result, char, "_")
	}

	return result
}
