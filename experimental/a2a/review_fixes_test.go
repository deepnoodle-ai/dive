package a2a_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deepnoodle-ai/dive/experimental/a2a"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// Part.Validate enforces mutual exclusivity (issue #4 in review).
func TestPartValidate(t *testing.T) {
	cases := []struct {
		name    string
		part    a2a.Part
		wantErr bool
	}{
		{"text-only", a2a.NewTextPart("hi"), false},
		{"raw-only", a2a.NewRawPart("AAAA", "image/png"), false},
		{"data-only", a2a.NewDataPart(map[string]any{"k": "v"}), false},
		{"url-only", a2a.NewURLPart("https://x", "image/png"), false},
		{"empty", a2a.Part{}, true},
		{"text+url", a2a.Part{Text: "hi", URL: "https://x"}, true},
		{"text+data", a2a.Part{Text: "hi", Data: map[string]any{"k": "v"}}, true},
		{"raw+url", a2a.Part{Raw: "AAAA", URL: "https://x"}, true},
		{"all", a2a.Part{Text: "hi", Raw: "AAAA", Data: map[string]any{"k": "v"}, URL: "https://x"}, true},
	}
	for _, c := range cases {
		err := c.part.Validate()
		if c.wantErr {
			assert.Error(t, err, "case %s: expected error", c.name)
		} else {
			assert.NoError(t, err, "case %s: unexpected error", c.name)
		}
	}
}

func TestSendMessageParamsValidateRejectsMixedPart(t *testing.T) {
	params := &a2a.SendMessageParams{
		Message: &a2a.Message{
			MessageID: "m1",
			Role:      a2a.RoleUser,
			Parts: []a2a.Part{
				{Text: "hi", URL: "https://x"},
			},
		},
	}
	err := params.Validate()
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "exactly one"))
}

// Server.Card returns a deep copy (issue #5).
func TestServerCardIsDeepCopy(t *testing.T) {
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		return textResponse("ok"), nil
	}}
	agent := buildAgent(t, model)

	server, err := a2a.NewServer(a2a.ServerOptions{
		Agent:   agent,
		BaseURL: "https://agent.example.com",
		Card: a2a.AgentCard{
			Skills: []a2a.AgentSkill{{ID: "chat", Name: "chat", Description: "x", Tags: []string{"a"}}},
		},
	})
	assert.NoError(t, err)

	a := server.Card()
	a.Name = "MUTATED"
	a.Skills[0].Name = "MUTATED"
	a.Skills[0].Tags[0] = "MUTATED"
	a.SupportedInterfaces[0].URL = "MUTATED"

	b := server.Card()
	assert.True(t, b.Name != "MUTATED", "Server.Card() must return a deep copy")
	assert.True(t, b.Skills[0].Name != "MUTATED", "Skills slice must be copied")
	assert.True(t, b.Skills[0].Tags[0] != "MUTATED", "Tags slice must be copied")
	assert.True(t, b.SupportedInterfaces[0].URL != "MUTATED", "SupportedInterfaces slice must be copied")
}

// Server enforces a request body size limit (issue #1).
func TestServerRejectsOversizedRequest(t *testing.T) {
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		return textResponse("ok"), nil
	}}
	agent := buildAgent(t, model)

	server, err := a2a.NewServer(a2a.ServerOptions{
		Agent:           agent,
		MaxRequestBytes: 256,
	})
	assert.NoError(t, err)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	body := bytes.Repeat([]byte("a"), 4096)
	resp, err := http.Post(ts.URL+"/", "application/json", bytes.NewReader(body))
	assert.NoError(t, err)
	defer resp.Body.Close()

	var env struct {
		Error *a2a.RPCError `json:"error"`
	}
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&env))
	assert.True(t, env.Error != nil, "expected RPC error envelope on oversized body")
	assert.Equal(t, env.Error.Code, a2a.ErrorCodeParseError)
}

// MemoryTaskStore.Put rejects invalid records (issue #9).
func TestMemoryTaskStorePutRejectsInvalidRecord(t *testing.T) {
	store := a2a.NewMemoryTaskStore()
	ctx := context.Background()

	assert.True(t, errors.Is(store.Put(ctx, nil), a2a.ErrInvalidTaskRecord))
	assert.True(t, errors.Is(store.Put(ctx, &a2a.TaskRecord{}), a2a.ErrInvalidTaskRecord))
	assert.True(t, errors.Is(store.Put(ctx, &a2a.TaskRecord{Task: &a2a.Task{}}), a2a.ErrInvalidTaskRecord))

	assert.NoError(t, store.Put(ctx, &a2a.TaskRecord{Task: &a2a.Task{ID: "t1"}}))
	rec, ok, err := store.Get(ctx, "t1")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, rec.Task.ID, "t1")
}

// RemoteAgent.RefreshCard re-fetches and updates the cache (issue #6).
func TestRemoteAgentRefreshCard(t *testing.T) {
	var version atomic.Value
	version.Store("1")

	mux := http.NewServeMux()
	mux.HandleFunc(a2a.DefaultAgentCardPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w,
			`{"name":"r","description":"d","supportedInterfaces":[],"version":%q,`+
				`"defaultInputModes":[],"defaultOutputModes":[],"skills":[],"capabilities":{}}`,
			version.Load().(string))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client, err := a2a.NewClient(a2a.ClientOptions{Endpoint: ts.URL + "/"})
	assert.NoError(t, err)
	remote := a2a.NewRemoteAgent(client)

	card, err := remote.Card(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, card.Version, "1")

	// Server bumps its version. The cached card should still report 1.
	version.Store("2")
	card2, err := remote.Card(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, card2.Version, "1")

	// RefreshCard pulls the new version.
	card3, err := remote.RefreshCard(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, card3.Version, "2")
}
