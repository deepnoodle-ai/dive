package prompt

import (
	"context"

	"github.com/getstingrai/dive/llm"
)

func Execute(ctx context.Context, model llm.LLM, opts ...Option) (*llm.Response, error) {
	template := New(opts...)
	prompt, err := template.Build()
	if err != nil {
		return nil, err
	}
	return model.Generate(ctx, prompt.Messages, llm.WithSystemPrompt(prompt.System))
}
