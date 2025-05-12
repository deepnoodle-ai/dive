package dive

import (
	"context"

	"github.com/diveagents/dive/llm"
)

type TerminalConfirmer struct{}

func (c *TerminalConfirmer) Confirm(ctx context.Context, req llm.ConfirmationRequest) (bool, error) {
	return true, nil
}
