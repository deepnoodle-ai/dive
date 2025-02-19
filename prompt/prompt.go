package prompt

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/getstingrai/agents/llm"
)

type Prompt struct {
	System   string
	Messages []*llm.Message
}

// Template represents a complete prompt configuration
type Template struct {
	system         []string
	messages       []llm.Message
	directives     []string
	context        []string
	examples       []string
	expectedOutput string
	params         map[string]interface{}
}

// Option is a functional option for configuring a Template
type Option func(*Template)

// New creates a new prompt template with the given options
func New(opts ...Option) *Template {
	p := &Template{
		params: make(map[string]interface{}),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// WithSystemMessage appends to the system message
func WithSystemMessage(content ...string) Option {
	return func(t *Template) {
		t.system = append(t.system, content...)
	}
}

// WithMessage appends one or more messages to the prompt
func WithMessage(messages ...llm.Message) Option {
	return func(t *Template) {
		t.messages = append(t.messages, messages...)
	}
}

// WithUserMessage adds a user message with the given text to the prompt
func WithUserMessage(content string) Option {
	return func(t *Template) {
		t.messages = append(t.messages, llm.Message{
			Role:    llm.User,
			Content: []llm.Content{{Type: llm.ContentTypeText, Text: content}},
		})
	}
}

// WithAssistantMessage adds an assistant message with the given text to the prompt
func WithAssistantMessage(content string) Option {
	return func(t *Template) {
		t.messages = append(t.messages, llm.Message{
			Role:    llm.Assistant,
			Content: []llm.Content{{Type: llm.ContentTypeText, Text: content}},
		})
	}
}

// WithImage adds an image to the prompt
func WithImage(mediaType string, encodedData string) Option {
	return func(t *Template) {
		t.messages = append(t.messages, llm.Message{
			Role: llm.User,
			Content: []llm.Content{{
				Type:      llm.ContentTypeImage,
				MediaType: mediaType,
				Data:      encodedData,
			}},
		})
	}
}

// WithExpectedOutput adds an expected output to the prompt
func WithExpectedOutput(expectedOutput string) Option {
	return func(t *Template) {
		t.expectedOutput = expectedOutput
	}
}

// WithDirective adds one or more directives to the prompt
func WithDirective(directive ...string) Option {
	return func(t *Template) {
		t.directives = append(t.directives, directive...)
	}
}

// WithContext adds one or more context items to the prompt
func WithContext(context ...string) Option {
	return func(t *Template) {
		t.context = append(t.context, context...)
	}
}

// WithExample adds one or more examples to the prompt
func WithExample(examples ...string) Option {
	return func(t *Template) {
		t.examples = append(t.examples, examples...)
	}
}

// Build compiles the prompt and returns messages and options for LLM generation
func (t *Template) Build(params ...map[string]any) (*Prompt, error) {
	allParams := map[string]any{}
	for _, p := range params {
		for k, v := range p {
			allParams[k] = v
		}
	}

	var messages []*llm.Message
	var systemContent []string

	if len(t.system) > 0 {
		systemText, err := renderTemplate(strings.Join(t.system, "\n\n"), allParams)
		if err != nil {
			return nil, err
		}
		systemContent = append(systemContent, systemText)
	}

	if len(t.directives) > 0 {
		directivesText, err := renderTemplate(bulletedList(t.directives), allParams)
		if err != nil {
			return nil, err
		}
		directivesText = fmt.Sprintf("Directives:\n%s", directivesText)
		systemContent = append(systemContent, directivesText)
	}

	for i, ex := range t.examples {
		exampleText, err := renderTemplate(ex, allParams)
		if err != nil {
			return nil, err
		}
		exampleText = fmt.Sprintf("Example %d:\n%s", i+1, exampleText)
		messages = append(messages, &llm.Message{
			Role:    llm.User,
			Content: []llm.Content{{Type: llm.ContentTypeText, Text: exampleText}},
		})
	}

	for i, context := range t.context {
		contextContent := fmt.Sprintf("Context %d:\n%s", i+1, context)
		messages = append(messages, &llm.Message{
			Role:    llm.User,
			Content: []llm.Content{{Type: llm.ContentTypeText, Text: contextContent}},
		})
	}

	for _, msg := range t.messages {
		var contents []llm.Content
		for _, content := range msg.Content {
			var text string
			if content.Type == llm.ContentTypeText {
				renderedContent, err := renderTemplate(content.Text, allParams)
				if err != nil {
					return nil, err
				}
				text = renderedContent
			} else {
				text = content.Text
			}
			contents = append(contents, llm.Content{
				ID:        content.ID,
				Name:      content.Name,
				Type:      content.Type,
				Text:      text,
				Data:      content.Data,
				MediaType: content.MediaType,
				Input:     content.Input,
			})
		}
		messages = append(messages, &llm.Message{
			Role:    msg.Role,
			Content: contents,
		})
	}

	return &Prompt{
		System:   strings.Join(systemContent, "\n\n"),
		Messages: messages,
	}, nil
}

// renderTemplate applies template parameters to text
func renderTemplate(text string, params map[string]any) (string, error) {
	tmpl, err := template.New("prompt").Parse(text)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// bulletedList joins strings with newlines
func bulletedList(items []string) string {
	var buf bytes.Buffer
	for _, item := range items {
		buf.WriteString("- ")
		buf.WriteString(item)
		buf.WriteString("\n")
	}
	return buf.String()
}
