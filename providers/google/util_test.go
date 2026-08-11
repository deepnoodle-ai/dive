package google

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"google.golang.org/genai"
)

func TestGenerateToolCallIDUnique(t *testing.T) {
	// The same tool called repeatedly must get distinct IDs, since tool
	// result matching is keyed by ID.
	seen := map[string]bool{}
	for i := 0; i < 10; i++ {
		id := generateToolCallID("calculator")
		assert.False(t, seen[id])
		seen[id] = true
	}
}

func TestConvertUsageMetadataDisjointInputBuckets(t *testing.T) {
	metadata := &genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount:        100,
		CachedContentTokenCount: 70,
		CandidatesTokenCount:    25,
		ThoughtsTokenCount:      10,
		ToolUsePromptTokenCount: 5,
		TotalTokenCount:         140,
		PromptTokensDetails:     []*genai.ModalityTokenCount{{Modality: genai.MediaModalityText, TokenCount: 100}},
		CacheTokensDetails:      []*genai.ModalityTokenCount{{Modality: genai.MediaModalityText, TokenCount: 70}},
		CandidatesTokensDetails: []*genai.ModalityTokenCount{{Modality: genai.MediaModalityText, TokenCount: 25}},
		ToolUsePromptTokensDetails: []*genai.ModalityTokenCount{{
			Modality: genai.MediaModalityText, TokenCount: 5,
		}},
	}

	usage, err := convertUsageMetadata(metadata)
	assert.NoError(t, err)
	assert.Equal(t, 35, usage.InputTokens)
	assert.Equal(t, 70, usage.CacheReadInputTokens)
	assert.Equal(t, 105, usage.TotalInputTokens())
	assert.Equal(t, 35, usage.OutputTokens)
	assert.Equal(t, 10, usage.ReasoningTokens)
	assert.Equal(t, 5, usage.ToolUseInputTokens)
	assert.Equal(t, int(metadata.TotalTokenCount), usage.TotalInputTokens()+usage.OutputTokens)
	assert.True(t, usage.CacheCreationInputTokensUnavailable)
	assert.Equal(t, llm.ModalityTokenUsage{
		InputTokens: 35, OutputTokens: 35, CacheReadInputTokens: 70,
	}, usage.ModalityTokens["text"])
}

func TestConvertUsageMetadataRejectsInconsistentProviderCounts(t *testing.T) {
	tests := []struct {
		name     string
		metadata *genai.GenerateContentResponseUsageMetadata
		wantErr  string
	}{
		{
			name: "cached above prompt",
			metadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount: 10, CachedContentTokenCount: 20,
			},
			wantErr: "cached token count",
		},
		{
			name: "negative prompt",
			metadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount: -10,
			},
			wantErr: "negative Gemini prompt",
		},
		{
			name: "modality total mismatch",
			metadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount: 10,
				TotalTokenCount:  10,
				PromptTokensDetails: []*genai.ModalityTokenCount{{
					Modality: genai.MediaModalityText, TokenCount: 9,
				}},
			},
			wantErr: "modality tokens do not reconcile",
		},
		{
			name: "nil modality detail",
			metadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:    10,
				TotalTokenCount:     10,
				PromptTokensDetails: []*genai.ModalityTokenCount{nil},
			},
			wantErr: "invalid Gemini prompt modality token detail",
		},
		{
			name: "cached modality above prompt modality",
			metadata: &genai.GenerateContentResponseUsageMetadata{
				PromptTokenCount:        10,
				CachedContentTokenCount: 5,
				TotalTokenCount:         10,
				PromptTokensDetails: []*genai.ModalityTokenCount{
					{Modality: genai.MediaModalityText, TokenCount: 4},
					{Modality: genai.MediaModalityAudio, TokenCount: 6},
				},
				CacheTokensDetails: []*genai.ModalityTokenCount{
					{Modality: genai.MediaModalityText, TokenCount: 5},
				},
			},
			wantErr: "invalid Gemini text cached token count",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := convertUsageMetadata(tt.metadata)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestConvertUsageMetadataDegradesAggregateMismatch(t *testing.T) {
	usage, err := convertUsageMetadata(&genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount:     10,
		CandidatesTokenCount: 5,
		TotalTokenCount:      99,
	})
	assert.NoError(t, err)
	assert.Equal(t, 10, usage.InputTokens)
	assert.Equal(t, 5, usage.OutputTokens)
	assert.True(t, usage.CostEstimateUnavailable)
}

func TestConvertUsageMetadataMarksOmittedAndUnspecifiedModalityDetailsIncomplete(t *testing.T) {
	t.Run("cache details without prompt details", func(t *testing.T) {
		usage, err := convertUsageMetadata(&genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:        10,
			CachedContentTokenCount: 2,
			TotalTokenCount:         10,
			CacheTokensDetails: []*genai.ModalityTokenCount{{
				Modality: genai.MediaModalityText, TokenCount: 2,
			}},
		})
		assert.NoError(t, err)
		assert.True(t, usage.InputModalityTokenDetailsIncomplete)
		assert.True(t, usage.CacheReadModalityTokenDetailsIncomplete)
		assert.Nil(t, usage.ModalityTokens)
	})

	t.Run("omitted", func(t *testing.T) {
		usage, err := convertUsageMetadata(&genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:        10,
			CachedContentTokenCount: 2,
			CandidatesTokenCount:    3,
			TotalTokenCount:         13,
		})
		assert.NoError(t, err)
		assert.True(t, usage.InputModalityTokenDetailsIncomplete)
		assert.True(t, usage.CacheReadModalityTokenDetailsIncomplete)
		assert.True(t, usage.OutputModalityTokenDetailsIncomplete)
		assert.Nil(t, usage.ModalityTokens)
	})

	t.Run("unspecified", func(t *testing.T) {
		usage, err := convertUsageMetadata(&genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount: 10,
			TotalTokenCount:  10,
			PromptTokensDetails: []*genai.ModalityTokenCount{{
				Modality: genai.MediaModalityUnspecified, TokenCount: 10,
			}},
		})
		assert.NoError(t, err)
		assert.True(t, usage.InputModalityTokenDetailsIncomplete)
		assert.True(t, usage.CacheReadModalityTokenDetailsIncomplete)
		assert.Nil(t, usage.ModalityTokens)
	})
}

func TestConvertGoogleResponsePreservesContentWhenUsageTotalDoesNotReconcile(t *testing.T) {
	response, err := convertGoogleResponse(&genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content:      &genai.Content{Parts: []*genai.Part{{Text: "completed answer"}}},
			FinishReason: genai.FinishReasonStop,
		}},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     10,
			CandidatesTokenCount: 5,
			TotalTokenCount:      99,
		},
	}, ModelGemini25Pro)
	assert.NoError(t, err)
	assert.Equal(t, "completed answer", response.Message().Text())
	assert.Equal(t, 10, response.Usage.InputTokens)
	assert.Equal(t, 5, response.Usage.OutputTokens)
	assert.True(t, response.Usage.CostEstimateUnavailable)
	assert.True(t, strings.HasPrefix(response.ID, "google_"+ModelGemini25Pro+"_"))
}

func TestMessagesToContentsToolRoundTrip(t *testing.T) {
	// A tool use ID generated by the provider is carried on the message
	// content, so converting back to Google format matches results to calls
	// by the stored ID.
	id := generateToolCallID("calculator")
	messages := []*llm.Message{
		llm.NewUserTextMessage("What is 1+1?"),
		{
			Role: llm.Assistant,
			Content: []llm.Content{
				&llm.ToolUseContent{
					ID:    id,
					Name:  "calculator",
					Input: []byte(`{"expression":"1+1"}`),
					Metadata: llm.ProviderMetadata{
						googleThoughtSignatureMetadataKey: base64.StdEncoding.EncodeToString([]byte("sig-123")),
					},
				},
			},
		},
		llm.NewToolResultMessage(&llm.ToolResultContent{
			ToolUseID: id,
			Content:   "2",
		}),
	}
	contents, err := messagesToContents(messages)
	assert.NoError(t, err)
	assert.Len(t, contents, 3)
	assert.NotNil(t, contents[1].Parts[0].FunctionCall)
	assert.Equal(t, contents[1].Parts[0].FunctionCall.ID, id)
	assert.Equal(t, []byte("sig-123"), contents[1].Parts[0].ThoughtSignature)
	assert.NotNil(t, contents[2].Parts[0].FunctionResponse)
	assert.Equal(t, contents[2].Parts[0].FunctionResponse.ID, id)
	assert.Equal(t, contents[2].Parts[0].FunctionResponse.Name, "calculator")
}

func TestToolResultSurvivesJSONRoundTrip(t *testing.T) {
	// Session persistence and Message.Copy round-trip messages through JSON,
	// which turns typed []*dive.ToolResultContent tool result content into
	// generic []any. The converter must decode that shape rather than fail
	// with "unknown content type: []interface {}".
	id := generateToolCallID("bash")
	original := llm.NewToolResultMessage(&llm.ToolResultContent{
		ToolUseID: id,
		Content: []*dive.ToolResultContent{
			{Type: dive.ToolResultContentTypeText, Text: "line one"},
			{Type: dive.ToolResultContentTypeText, Text: "line two"},
		},
	})
	body, err := json.Marshal(original)
	assert.NoError(t, err)
	var replayed llm.Message
	assert.NoError(t, json.Unmarshal(body, &replayed))

	contents, err := messagesToContents([]*llm.Message{
		{
			Role: llm.Assistant,
			Content: []llm.Content{
				&llm.ToolUseContent{ID: id, Name: "bash", Input: []byte(`{"command":"ls"}`)},
			},
		},
		&replayed,
	})
	assert.NoError(t, err)
	assert.Len(t, contents, 2)
	response := contents[1].Parts[0].FunctionResponse
	assert.NotNil(t, response)
	assert.Equal(t, response.Response["output"], "line one\n\nline two")
}

func TestGoogleThoughtSignatureSurvivesMessageRoundTrip(t *testing.T) {
	signature := []byte("opaque-google-signature")
	resp := &genai.GenerateContentResponse{
		ResponseID:   "response-from-google",
		ModelVersion: "gemini-3.1-flash-lite-served",
		Candidates: []*genai.Candidate{
			{
				Index: 0,
				Content: &genai.Content{
					Role: "model",
					Parts: []*genai.Part{
						{
							FunctionCall: &genai.FunctionCall{
								ID:   "call_1",
								Name: "default_api:mobius",
								Args: map[string]any{"command": "status"},
							},
							ThoughtSignature: signature,
						},
					},
				},
			},
		},
	}

	converted, err := convertGoogleResponse(resp, "gemini-3.1-flash-lite")
	assert.NoError(t, err)
	assert.Equal(t, "response-from-google", converted.ID)
	assert.Equal(t, "gemini-3.1-flash-lite-served", converted.Model)
	assert.Len(t, converted.Content, 1)

	toolUse, ok := converted.Content[0].(*llm.ToolUseContent)
	assert.True(t, ok)
	assert.Equal(t, base64.StdEncoding.EncodeToString(signature), toolUse.Metadata[googleThoughtSignatureMetadataKey])

	body, err := json.Marshal(converted.Message())
	assert.NoError(t, err)
	var replayed llm.Message
	assert.NoError(t, json.Unmarshal(body, &replayed))

	contents, err := messagesToContents([]*llm.Message{
		&replayed,
		llm.NewToolResultMessage(&llm.ToolResultContent{
			ToolUseID: toolUse.ID,
			Content:   "ok",
		}),
	})
	assert.NoError(t, err)
	assert.Len(t, contents, 2)
	assert.Equal(t, signature, contents[0].Parts[0].ThoughtSignature)
}

func TestGoogleTextAndThinkingPartsSurviveMessageRoundTrip(t *testing.T) {
	thoughtSignature := []byte("opaque-thought-signature")
	textSignature := []byte("opaque-text-signature")
	emptySignature := []byte("opaque-empty-signature")
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{
				Role: "model",
				Parts: []*genai.Part{
					{Text: "working it out", Thought: true, ThoughtSignature: thoughtSignature},
					{Text: "answer", ThoughtSignature: textSignature},
					{ThoughtSignature: emptySignature},
				},
			},
		}},
	}

	converted, err := convertGoogleResponse(resp, ModelGemini36Flash)
	assert.NoError(t, err)
	assert.Len(t, converted.Content, 3)

	thought, ok := converted.Content[0].(*llm.ThinkingContent)
	assert.True(t, ok)
	assert.Equal(t, "working it out", thought.Thinking)
	assert.Equal(t, "true", thought.Metadata[googleThoughtMetadataKey])
	assert.Equal(t, base64.StdEncoding.EncodeToString(thoughtSignature),
		thought.Metadata[googleThoughtSignatureMetadataKey])

	answer, ok := converted.Content[1].(*llm.TextContent)
	assert.True(t, ok)
	assert.Equal(t, "answer", answer.Text)
	assert.Equal(t, base64.StdEncoding.EncodeToString(textSignature),
		answer.Metadata[googleThoughtSignatureMetadataKey])

	empty, ok := converted.Content[2].(*llm.TextContent)
	assert.True(t, ok)
	assert.Equal(t, "", empty.Text)
	assert.Equal(t, base64.StdEncoding.EncodeToString(emptySignature),
		empty.Metadata[googleThoughtSignatureMetadataKey])

	replayed := converted.Message().Copy()
	contents, err := messagesToContents([]*llm.Message{replayed})
	assert.NoError(t, err)
	assert.Len(t, contents, 1)
	assert.Len(t, contents[0].Parts, 3)
	assert.True(t, contents[0].Parts[0].Thought)
	assert.Equal(t, "working it out", contents[0].Parts[0].Text)
	assert.Equal(t, thoughtSignature, contents[0].Parts[0].ThoughtSignature)
	assert.False(t, contents[0].Parts[1].Thought)
	assert.Equal(t, "answer", contents[0].Parts[1].Text)
	assert.Equal(t, textSignature, contents[0].Parts[1].ThoughtSignature)
	assert.Equal(t, "", contents[0].Parts[2].Text)
	assert.Equal(t, emptySignature, contents[0].Parts[2].ThoughtSignature)
}

func TestGoogleSkipsThinkingFromAnotherProvider(t *testing.T) {
	contents, err := messagesToContents([]*llm.Message{{
		Role: llm.Assistant,
		Content: []llm.Content{
			&llm.ThinkingContent{Thinking: "anthropic thought", Signature: "anthropic-signature"},
			&llm.TextContent{Text: "answer"},
		},
	}})
	assert.NoError(t, err)
	assert.Len(t, contents, 1)
	assert.Len(t, contents[0].Parts, 1)
	assert.Equal(t, "answer", contents[0].Parts[0].Text)
	assert.Equal(t, 0, len(contents[0].Parts[0].ThoughtSignature))
}

func TestGoogleSkipsMessagesFilteredToNoParts(t *testing.T) {
	contents, err := messagesToContents([]*llm.Message{
		{
			Role: llm.Assistant,
			Content: []llm.Content{
				&llm.ThinkingContent{Thinking: "foreign thought", Signature: "foreign-signature"},
			},
		},
		llm.NewUserTextMessage("continue"),
	})
	assert.NoError(t, err)
	assert.Len(t, contents, 1)
	assert.Equal(t, "user", contents[0].Role)
	assert.Len(t, contents[0].Parts, 1)
	assert.Equal(t, "continue", contents[0].Parts[0].Text)
}

func TestGoogleRequestPathsRejectMessagesFilteredToEmpty(t *testing.T) {
	tests := []struct {
		name    string
		content llm.Content
	}{
		{
			name: "foreign thinking",
			content: &llm.ThinkingContent{
				Thinking:  "foreign thought",
				Signature: "foreign-signature",
			},
		},
		{
			name:    "redacted thinking",
			content: &llm.RedactedThinkingContent{Data: "opaque"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := New(WithAPIKey("test-key"))
			message := llm.NewAssistantMessage(tt.content)

			_, err := provider.Generate(context.Background(), llm.WithMessages(message))
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "no messages remain")

			_, err = provider.Stream(context.Background(), llm.WithMessages(message))
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "no messages remain")
		})
	}
}
