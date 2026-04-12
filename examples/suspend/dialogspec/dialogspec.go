// Package dialogspec is a small convention shared by the suspend/* examples
// for carrying structured prompt information through a dive.SuspendResult.
//
// SuspendResult already exposes a free-form Metadata map[string]any. This
// package standardises one key ("dialog") so that tools, callers, and
// external notifiers all agree on the shape of the payload. A Spec captures
// the JSON-safe subset of dive.DialogInput — enough to render a confirm,
// select, multi-select, or free-form input prompt, or to ship as a webhook
// payload. Runtime-only DialogInput fields (Validate, Tool, Call) are
// deliberately omitted since they cannot round-trip through a FileStore.
package dialogspec

import "github.com/deepnoodle-ai/dive"

// Kind identifies the interaction style. The empty kind means "no user
// interaction expected — this suspend is just an async wait".
type Kind string

const (
	KindConfirm     Kind = "confirm"
	KindSelect      Kind = "select"
	KindMultiSelect Kind = "multi_select"
	KindInput       Kind = "input"
)

// Option is one choice in a select / multi-select prompt.
type Option struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Spec is the serializable shape of a dialog prompt.
type Spec struct {
	Kind    Kind     `json:"kind,omitempty"`
	Title   string   `json:"title,omitempty"`
	Message string   `json:"message,omitempty"`
	Options []Option `json:"options,omitempty"`
	Default string   `json:"default,omitempty"`
}

// metaKey is the reserved Metadata key under which Spec is stored.
const metaKey = "dialog"

// NewSuspend returns a *dive.ToolResult whose Suspend field carries s as
// metadata. The SuspendResult.Prompt string falls back to Message then Title
// so consumers that only read Prompt still see something sensible.
func NewSuspend(s Spec) *dive.ToolResult {
	prompt := s.Message
	if prompt == "" {
		prompt = s.Title
	}
	return dive.NewSuspendResult(prompt, map[string]any{metaKey: s.toMap()})
}

// FromPending extracts a Spec from a PendingToolCall. Returns nil if the
// pending call does not carry a dialog spec.
func FromPending(p *dive.PendingToolCall) *Spec {
	if p == nil || p.Metadata == nil {
		return nil
	}
	raw, ok := p.Metadata[metaKey].(map[string]any)
	if !ok {
		return nil
	}
	return fromMap(raw)
}

// ToDialogInput converts the Spec into a *dive.DialogInput suitable for
// rendering with any dive.Dialog implementation. Returns nil when Kind is
// empty (no interaction expected).
func (s *Spec) ToDialogInput() *dive.DialogInput {
	if s == nil || s.Kind == "" {
		return nil
	}
	in := &dive.DialogInput{Title: s.Title, Message: s.Message, Default: s.Default}
	switch s.Kind {
	case KindConfirm:
		in.Confirm = true
	case KindSelect, KindMultiSelect:
		in.MultiSelect = s.Kind == KindMultiSelect
		for _, o := range s.Options {
			in.Options = append(in.Options, dive.DialogOption{
				Value: o.Value, Label: o.Label, Description: o.Description,
			})
		}
	}
	return in
}

// Metadata round-trips through JSON (e.g. via FileStore), so nested values
// come back as map[string]any rather than Spec. toMap/fromMap normalise both
// directions.

func (s Spec) toMap() map[string]any {
	m := map[string]any{}
	if s.Kind != "" {
		m["kind"] = string(s.Kind)
	}
	if s.Title != "" {
		m["title"] = s.Title
	}
	if s.Message != "" {
		m["message"] = s.Message
	}
	if s.Default != "" {
		m["default"] = s.Default
	}
	if len(s.Options) > 0 {
		opts := make([]any, 0, len(s.Options))
		for _, o := range s.Options {
			om := map[string]any{"value": o.Value, "label": o.Label}
			if o.Description != "" {
				om["description"] = o.Description
			}
			opts = append(opts, om)
		}
		m["options"] = opts
	}
	return m
}

func fromMap(m map[string]any) *Spec {
	s := &Spec{}
	if v, ok := m["kind"].(string); ok {
		s.Kind = Kind(v)
	}
	if v, ok := m["title"].(string); ok {
		s.Title = v
	}
	if v, ok := m["message"].(string); ok {
		s.Message = v
	}
	if v, ok := m["default"].(string); ok {
		s.Default = v
	}
	if raw, ok := m["options"].([]any); ok {
		for _, r := range raw {
			om, ok := r.(map[string]any)
			if !ok {
				continue
			}
			opt := Option{}
			if v, ok := om["value"].(string); ok {
				opt.Value = v
			}
			if v, ok := om["label"].(string); ok {
				opt.Label = v
			}
			if v, ok := om["description"].(string); ok {
				opt.Description = v
			}
			s.Options = append(s.Options, opt)
		}
	}
	return s
}
