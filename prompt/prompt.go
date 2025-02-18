package prompt

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/getstingrai/agents/llm"
)

// Prompt represents a complete prompt configuration
type Prompt struct {
	system         []string
	messages       []llm.Message
	directives     []string
	context        []string
	examples       []string
	expectedOutput string
	params         map[string]interface{}
}

// Option is a functional option for configuring a Prompt
type Option func(*Prompt)

// New creates a new Prompt with the given options
func New(opts ...Option) *Prompt {
	p := &Prompt{
		params: make(map[string]interface{}),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// WithSystem appends to the system message
func WithSystem(content ...string) Option {
	return func(p *Prompt) {
		p.system = append(p.system, content...)
	}
}

// WithMessage adds a user or assistant message
func WithMessage(role llm.Role, content string) Option {
	return func(p *Prompt) {
		p.messages = append(p.messages, llm.Message{
			Role:    role,
			Content: []llm.Content{{Type: llm.ContentTypeText, Text: content}},
		})
	}
}

func WithUserMessage(content string) Option {
	return func(p *Prompt) {
		p.messages = append(p.messages, llm.Message{
			Role:    llm.User,
			Content: []llm.Content{{Type: llm.ContentTypeText, Text: content}},
		})
	}
}

func WithImage(mediaType string, encodedData string) Option {
	return func(p *Prompt) {
		p.messages = append(p.messages, llm.Message{
			Role: llm.User,
			Content: []llm.Content{{
				Type:      llm.ContentTypeImage,
				MediaType: mediaType,
				Data:      encodedData,
			}},
		})
	}
}

func WithExpectedOutput(expectedOutput string) Option {
	return func(p *Prompt) {
		p.expectedOutput = expectedOutput
	}
}

// WithDirective adds a directive to the prompt
func WithDirective(directive string) Option {
	return func(p *Prompt) {
		p.directives = append(p.directives, directive)
	}
}

// WithContext adds context information to the prompt
func WithContext(context string) Option {
	return func(p *Prompt) {
		p.context = append(p.context, context)
	}
}

// WithExample adds an example conversation
func WithExample(examples ...string) Option {
	return func(p *Prompt) {
		p.examples = append(p.examples, examples...)
	}
}

// Build compiles the prompt and returns messages and options for LLM generation
func (p *Prompt) Build(params map[string]any) ([]llm.Message, error) {
	var messages []llm.Message

	// Build system message with stable instructions only
	var systemContent []string

	// Add original system message if present
	if len(p.system) > 0 {
		msg, err := renderTemplate(strings.Join(p.system, "\n\n"), params)
		if err != nil {
			return nil, err
		}
		systemContent = append(systemContent, msg)
	}

	// Add directives
	if len(p.directives) > 0 {
		systemContent = append(systemContent,
			fmt.Sprintf("Directives:\n\n%s\n", joinStrings(p.directives)),
		)
	}

	// Add combined system message if we have any system content
	if len(systemContent) > 0 {
		systemText := strings.Join(systemContent, "\n\n")
		messages = append(messages, llm.Message{
			Role:    llm.System,
			Content: []llm.Content{{Type: llm.ContentTypeText, Text: systemText}},
		})
	}

	// Add context as a separate user message if present
	if len(p.context) > 0 {
		contextContent := "Here is the relevant context:\n" + joinStrings(p.context)
		messages = append(messages, llm.Message{
			Role:    llm.User,
			Content: []llm.Content{{Type: llm.ContentTypeText, Text: contextContent}},
		})
	}

	// Add examples
	for _, ex := range p.examples {
		exampleText, err := renderTemplate(ex, params)
		if err != nil {
			return nil, err
		}
		messages = append(messages, llm.Message{
			Role:    llm.User,
			Content: []llm.Content{{Type: llm.ContentTypeText, Text: exampleText}},
		})
	}

	// Add conversation messages
	for _, msg := range p.messages {
		renderedMsg, err := renderTemplate(msg.Content[0].Text, params)
		if err != nil {
			return nil, err
		}
		messages = append(messages, llm.Message{
			Role:    msg.Role,
			Content: []llm.Content{{Type: llm.ContentTypeText, Text: renderedMsg}},
		})
	}

	return messages, nil
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

// joinStrings joins strings with newlines
func joinStrings(items []string) string {
	var buf bytes.Buffer
	for _, item := range items {
		buf.WriteString("- ")
		buf.WriteString(item)
		buf.WriteString("\n")
	}
	return buf.String()
}
