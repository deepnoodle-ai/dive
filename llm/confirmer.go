package llm

import (
	"context"
	"fmt"
)

// ConfirmationRequest represents a request for user confirmation, with optional
// details and data.
type ConfirmationRequest struct {
	Prompt  string      // Main prompt or question
	Details string      // Additional details or context (optional)
	Data    interface{} // Arbitrary data to display to the user (optional)
	Tool    Tool        // Tool that is requesting confirmation (optional)
}

// Confirmer abstracts user confirmation prompts.
type Confirmer interface {
	// Confirm presents a request to the user and returns true if the user
	// confirms, false otherwise.
	Confirm(ctx context.Context, req ConfirmationRequest) (bool, error)
}

// AutoApproveConfirmer always approves confirmation requests.
type AutoApproveConfirmer struct{}

func (a *AutoApproveConfirmer) Confirm(ctx context.Context, req ConfirmationRequest) (bool, error) {
	return true, nil
}

// DenyAllConfirmer always denies confirmation requests.
type DenyAllConfirmer struct{}

func (d *DenyAllConfirmer) Confirm(ctx context.Context, req ConfirmationRequest) (bool, error) {
	return false, nil
}

// NewConfirmer returns a Confirmer implementation based on the mode string.
// Supported modes: "auto", "deny"
func NewConfirmer(mode string) (Confirmer, error) {
	switch mode {
	case "auto":
		return &AutoApproveConfirmer{}, nil
	case "deny":
		return &DenyAllConfirmer{}, nil
	default:
		return nil, fmt.Errorf("invalid confirmer mode: %s", mode)
	}
}
