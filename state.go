package dive

// Well-known keys for HookContext.Values used by experimental packages.
// Using these constants instead of raw strings prevents typos and makes
// cross-package dependencies explicit.
const (
	// StateKeySessionID identifies the session for session hooks.
	StateKeySessionID = "session_id"

	// StateKeyUserID identifies the user for session hooks.
	StateKeyUserID = "user_id"

	// StateKeySession stores the loaded *session.Session for the Saver hook.
	StateKeySession = "session"

	// StateKeyCompactionEvent stores the compaction event for post-generation hooks.
	StateKeyCompactionEvent = "compaction_event"
)
