package session

import (
	"context"
	"time"

	"github.com/deepnoodle-ai/dive"
)

func getSessionID(state *dive.HookContext) string {
	if v, ok := state.Values[dive.StateKeySessionID].(string); ok {
		return v
	}
	return ""
}

func getUserID(state *dive.HookContext) string {
	if v, ok := state.Values[dive.StateKeyUserID].(string); ok {
		return v
	}
	return ""
}

// Hooks returns PreGeneration and PostGeneration hooks that implement
// session loading and saving using the provided Repository.
//
// The PreGeneration hook loads session history and prepends it to the messages.
// The PostGeneration hook saves the session with the new messages.
//
// Example:
//
//	repo := session.NewMemoryRepository()
//	preHook, postHook := session.Hooks(repo)
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    SystemPrompt: "You are a helpful assistant.",
//	    Model:        model,
//	    Hooks: dive.Hooks{
//	        PreGeneration:  []dive.PreGenerationHook{preHook},
//	        PostGeneration: []dive.PostGenerationHook{postHook},
//	    },
//	})
//
//	// Conversations are tracked by session ID set in Values
//	//	state.Values[dive.StateKeySessionID] = "my-session"
//	resp1, _ := agent.CreateResponse(ctx, dive.WithInput("Hello"))
//	resp2, _ := agent.CreateResponse(ctx, dive.WithInput("Tell me more"))
func Hooks(repo Repository) (dive.PreGenerationHook, dive.PostGenerationHook) {
	return Loader(repo), Saver(repo)
}

// Loader returns a PreGenerationHook that loads session history from the repository.
//
// If a session exists with the given session ID (from state.Values[dive.StateKeySessionID]),
// its messages are prepended to state.Messages. If the session doesn't exist,
// the hook does nothing.
//
// This hook stores the loaded session in state.Values[dive.StateKeySession] for use
// by the Saver hook.
func Loader(repo Repository) dive.PreGenerationHook {
	return func(ctx context.Context, state *dive.HookContext) error {
		sessionID := getSessionID(state)
		if sessionID == "" {
			return nil
		}

		session, err := repo.GetSession(ctx, sessionID)
		if err == ErrSessionNotFound {
			// New session, nothing to load
			return nil
		}
		if err != nil {
			return err
		}

		// Store session for the Saver hook
		if state.Values == nil {
			state.Values = map[string]any{}
		}
		state.Values[dive.StateKeySession] = session

		// Prepend existing messages to the new messages
		if len(session.Messages) > 0 {
			state.Messages = append(session.Messages, state.Messages...)
		}

		return nil
	}
}

// Saver returns a PostGenerationHook that saves the session to the repository.
//
// If a session was loaded by the Loader hook (stored in state.Values[dive.StateKeySession]),
// it updates that session. Otherwise, it creates a new session.
//
// The session includes all messages (history + new input + output messages).
func Saver(repo Repository) dive.PostGenerationHook {
	return func(ctx context.Context, state *dive.HookContext) error {
		sessionID := getSessionID(state)
		if sessionID == "" {
			return nil
		}

		// Get or create session
		var session *Session
		if existing, ok := state.Values[dive.StateKeySession].(*Session); ok {
			session = existing
		} else {
			session = &Session{
				ID:        sessionID,
				UserID:    getUserID(state),
				CreatedAt: time.Now(),
			}
		}

		// Update session
		session.UpdatedAt = time.Now()
		session.Messages = append(state.Messages, state.OutputMessages...)

		return repo.PutSession(ctx, session)
	}
}

// LoaderWithOptions provides additional configuration for session loading.
type LoaderWithOptions struct {
	// Repository is the session storage backend.
	Repository Repository

	// OnSessionLoaded is called when a session is successfully loaded.
	// Use this for logging or custom processing.
	OnSessionLoaded func(ctx context.Context, session *Session)
}

// Build returns a PreGenerationHook with the configured options.
func (o LoaderWithOptions) Build() dive.PreGenerationHook {
	return func(ctx context.Context, state *dive.HookContext) error {
		sessionID := getSessionID(state)
		if sessionID == "" {
			return nil
		}

		session, err := o.Repository.GetSession(ctx, sessionID)
		if err == ErrSessionNotFound {
			return nil
		}
		if err != nil {
			return err
		}

		if state.Values == nil {
			state.Values = map[string]any{}
		}
		state.Values[dive.StateKeySession] = session

		if len(session.Messages) > 0 {
			state.Messages = append(session.Messages, state.Messages...)
		}

		if o.OnSessionLoaded != nil {
			o.OnSessionLoaded(ctx, session)
		}

		return nil
	}
}

// SaverWithOptions provides additional configuration for session saving.
type SaverWithOptions struct {
	// Repository is the session storage backend.
	Repository Repository

	// OnSessionSaved is called after a session is successfully saved.
	// Use this for logging or custom processing.
	OnSessionSaved func(ctx context.Context, session *Session)

	// AgentID is set on new sessions.
	AgentID string

	// AgentName is set on new sessions.
	AgentName string
}

// Build returns a PostGenerationHook with the configured options.
func (o SaverWithOptions) Build() dive.PostGenerationHook {
	return func(ctx context.Context, state *dive.HookContext) error {
		sessionID := getSessionID(state)
		if sessionID == "" {
			return nil
		}

		var session *Session
		if existing, ok := state.Values[dive.StateKeySession].(*Session); ok {
			session = existing
		} else {
			session = &Session{
				ID:        sessionID,
				UserID:    getUserID(state),
				AgentID:   o.AgentID,
				AgentName: o.AgentName,
				CreatedAt: time.Now(),
			}
		}

		session.UpdatedAt = time.Now()
		session.Messages = append(state.Messages, state.OutputMessages...)

		if err := o.Repository.PutSession(ctx, session); err != nil {
			return err
		}

		if o.OnSessionSaved != nil {
			o.OnSessionSaved(ctx, session)
		}

		return nil
	}
}
