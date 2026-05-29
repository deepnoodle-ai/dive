package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/ollama"
)

func main() {
	var (
		provider     string
		modelName    string
		sessionDir   string
		ticks        int
		parallelism  int
		tickMinutes  int
		villagers    int
		reflectEvery int
		seedParty    bool
		httpAddr     string
		tickDelay    time.Duration
		jsonOutput   bool
		recordPath   string
	)
	flag.StringVar(&provider, "provider", "auto", "model provider: auto, ollama, or scripted")
	flag.StringVar(&modelName, "model", ollama.ModelLlama32_3B, "model name for the ollama provider")
	flag.StringVar(&sessionDir, "sessions", ".noodleville/sessions", "directory for per-villager file sessions")
	flag.IntVar(&ticks, "ticks", 3, "number of town ticks to run")
	flag.IntVar(&parallelism, "parallelism", 2, "maximum simultaneous LLM turns")
	flag.IntVar(&tickMinutes, "tick-minutes", 10, "in-world minutes advanced per tick")
	flag.IntVar(&villagers, "villagers", 12, "number of villagers to seed, up to 25")
	flag.IntVar(&reflectEvery, "reflect-every", 6, "run reflection and session compaction every N ticks; 0 disables")
	flag.BoolVar(&seedParty, "party", true, "seed Maya with a Saturday noodle party goal")
	flag.StringVar(&httpAddr, "http", "", "optional HTTP address for the embedded browser view, for example :8080")
	flag.DurationVar(&tickDelay, "tick-delay", time.Second, "delay between ticks when -http is enabled")
	flag.BoolVar(&jsonOutput, "json", false, "emit one JSON report per tick")
	flag.StringVar(&recordPath, "record", "", "optional JSONL path for recording the run")
	flag.Parse()
	modelExplicit := flagWasSet("model")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	provider, modelName, err := resolveProvider(ctx, provider, modelName, modelExplicit)
	if err != nil {
		log.Fatal(err)
	}
	model, err := buildModel(provider, modelName)
	if err != nil {
		log.Fatal(err)
	}
	town, err := NewTown(ctx, TownOptions{
		Model:              model,
		SessionDir:         sessionDir,
		Parallelism:        parallelism,
		TickMinutes:        tickMinutes,
		VillagerCount:      villagers,
		SeedParty:          seedParty,
		ReflectionInterval: reflectEvery,
	})
	if err != nil {
		log.Fatal(err)
	}

	if !jsonOutput {
		fmt.Fprintf(os.Stdout, "NoodleVille provider=%s model=%s villagers=%d parallelism=%d reflect_every=%d party=%t sessions=%s\n\n",
			provider, model.Name(), len(town.Snapshot().Villagers), parallelism, reflectEvery, seedParty, sessionDir)
	}
	recorder, err := newRecorder(recordPath, RunMetadata{
		Provider:           provider,
		Model:              model.Name(),
		Villagers:          len(town.Snapshot().Villagers),
		Parallelism:        parallelism,
		TickMinutes:        tickMinutes,
		ReflectionInterval: reflectEvery,
		SeedParty:          seedParty,
		SessionDir:         sessionDir,
	}, town.Snapshot(), jsonOutput)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := recorder.Close(); err != nil {
			log.Printf("record close error: %v", err)
		}
	}()
	if httpAddr != "" {
		if err := runWithWeb(ctx, town, httpAddr, ticks, tickDelay, jsonOutput, recorder); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := town.RunWithCallback(ctx, ticks, os.Stdout, jsonOutput, recorder.RecordTick); err != nil {
		log.Fatal(err)
	}
	if !jsonOutput {
		fmt.Fprintf(os.Stdout, "memory_counts=%v\n", town.MemoryCounts(ctx))
	}
}

func newRecorder(path string, metadata RunMetadata, initial WorldSnapshot, jsonOutput bool) (*RunRecorder, error) {
	if path == "" {
		return nil, nil
	}
	recorder, err := NewRunRecorder(path, metadata, initial)
	if err != nil {
		return nil, err
	}
	if !jsonOutput {
		fmt.Fprintf(os.Stdout, "recording=%s\n", path)
	}
	return recorder, nil
}

func runWithWeb(ctx context.Context, town *Town, httpAddr string, ticks int, tickDelay time.Duration, jsonOutput bool, recorder *RunRecorder) error {
	broker := NewReportBroker()
	handler, err := NewWebHandler(ctx, town, broker)
	if err != nil {
		return err
	}
	server := &http.Server{Addr: httpAddr, Handler: handler}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("http server error: %v", err)
		}
	}()
	fmt.Fprintf(os.Stdout, "browser=http://localhost%s\n\n", httpAddr)

	for i := 0; ticks == 0 || i < ticks; i++ {
		report, err := town.RunTick(ctx)
		if err != nil {
			return err
		}
		if err := recorder.RecordTick(report); err != nil {
			return err
		}
		broker.Publish(report)
		if jsonOutput {
			if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
				return err
			}
		} else {
			report.Print(os.Stdout)
		}
		if tickDelay > 0 && (ticks == 0 || i < ticks-1) {
			select {
			case <-time.After(tickDelay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return nil
}

func buildModel(provider, modelName string) (llm.LLM, error) {
	switch provider {
	case "ollama":
		return ollama.New(
			ollama.WithModel(modelName),
			ollama.WithMaxTokens(1024),
		), nil
	case "scripted":
		return NewScriptedPlanner(25 * time.Millisecond), nil
	default:
		return nil, fmt.Errorf("unknown provider %q", provider)
	}
}

func flagWasSet(name string) bool {
	var found bool
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func resolveProvider(ctx context.Context, provider, modelName string, modelExplicit bool) (string, string, error) {
	tagsURL, err := ollamaTagsURL()
	if err != nil {
		return "", "", err
	}
	return resolveProviderWithTagsURL(ctx, provider, modelName, modelExplicit, tagsURL)
}

func resolveProviderWithTagsURL(ctx context.Context, provider, modelName string, modelExplicit bool, tagsURL string) (string, string, error) {
	switch provider {
	case "auto":
		models, err := availableOllamaModelsURL(ctx, tagsURL)
		if err != nil || len(models) == 0 {
			return "scripted", modelName, nil
		}
		if hasModel(models, modelName) {
			return "ollama", modelName, nil
		}
		if modelExplicit {
			return "", "", missingOllamaModelError(modelName, models)
		}
		if model := preferredOllamaModel(models); model != "" {
			return "ollama", model, nil
		}
		return "scripted", modelName, nil
	case "ollama":
		if err := checkOllamaModelURL(ctx, tagsURL, modelName); err != nil {
			return "", "", err
		}
		return "ollama", modelName, nil
	case "scripted":
		return "scripted", modelName, nil
	default:
		return "", "", fmt.Errorf("unknown provider %q", provider)
	}
}

func checkOllamaModel(ctx context.Context, modelName string) error {
	tagsURL, err := ollamaTagsURL()
	if err != nil {
		return err
	}
	return checkOllamaModelURL(ctx, tagsURL, modelName)
}

func ollamaTagsURL() (string, error) {
	endpoint, err := url.Parse(ollama.DefaultEndpoint)
	if err != nil {
		return "", err
	}
	endpoint.Path = "/api/tags"
	endpoint.RawQuery = ""
	return endpoint.String(), nil
}

func checkOllamaModelURL(ctx context.Context, tagsURL, modelName string) error {
	models, err := availableOllamaModelsURL(ctx, tagsURL)
	if err != nil {
		return err
	}
	if hasModel(models, modelName) {
		return nil
	}
	return missingOllamaModelError(modelName, models)
}

func availableOllamaModelsURL(ctx context.Context, tagsURL string) ([]string, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, tagsURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama is not reachable at %s; start Ollama or use `go run . -provider scripted`", tagsURL)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama model check failed at %s with status %s; use `go run . -provider scripted` to run without Ollama", tagsURL, resp.Status)
	}

	var tags struct {
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, err
	}
	models := make([]string, 0, len(tags.Models))
	for _, model := range tags.Models {
		if model.Name != "" {
			models = append(models, model.Name)
			continue
		}
		if model.Model != "" {
			models = append(models, model.Model)
		}
	}
	return models, nil
}

func hasModel(models []string, modelName string) bool {
	for _, model := range models {
		if model == modelName {
			return true
		}
	}
	return false
}

func preferredOllamaModel(models []string) string {
	for _, preferred := range []string{
		ollama.ModelLlama32_3B,
		"llama3.2:latest",
		"llama3.2",
		"mistral:7b",
		"qwen3:4b",
	} {
		if hasModel(models, preferred) {
			return preferred
		}
	}
	if len(models) == 0 {
		return ""
	}
	return models[0]
}

func missingOllamaModelError(modelName string, models []string) error {
	return fmt.Errorf("ollama model %q is not installed; installed models include %s; run `ollama pull %s` or use `go run . -provider scripted`", modelName, summarizeModels(models, 5), modelName)
}

func summarizeModels(models []string, limit int) string {
	if len(models) == 0 {
		return "none"
	}
	if len(models) <= limit {
		return fmt.Sprintf("%q", models)
	}
	return fmt.Sprintf("%q and %d more", models[:limit], len(models)-limit)
}
