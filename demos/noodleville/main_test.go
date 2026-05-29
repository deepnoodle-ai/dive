package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestCheckOllamaModelURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, r.URL.Path, "/api/tags")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"llama3.2:3b","model":"llama3.2:3b"}]}`))
	}))
	defer server.Close()

	assert.NoError(t, checkOllamaModelURL(context.Background(), server.URL+"/api/tags", "llama3.2:3b"))
}

func TestCheckOllamaModelURLReportsMissingModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"mistral:7b","model":"mistral:7b"}]}`))
	}))
	defer server.Close()

	err := checkOllamaModelURL(context.Background(), server.URL+"/api/tags", "llama3.2:3b")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `ollama model "llama3.2:3b" is not installed`)
	assert.Contains(t, err.Error(), "ollama pull llama3.2:3b")
}

func TestResolveProviderAutoUsesInstalledOllamaModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"llama3.2:latest","model":"llama3.2:latest"}]}`))
	}))
	defer server.Close()

	provider, model, err := resolveProviderWithTagsURL(context.Background(), "auto", "llama3.2:3b", false, server.URL+"/api/tags")
	assert.NoError(t, err)
	assert.Equal(t, provider, "ollama")
	assert.Equal(t, model, "llama3.2:latest")
}

func TestResolveProviderAutoFallsBackToScriptedWhenOllamaIsUnavailable(t *testing.T) {
	provider, model, err := resolveProviderWithTagsURL(context.Background(), "auto", "llama3.2:3b", false, "http://127.0.0.1:1/api/tags")
	assert.NoError(t, err)
	assert.Equal(t, provider, "scripted")
	assert.Equal(t, model, "llama3.2:3b")
}
