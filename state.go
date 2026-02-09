package dive

// Well-known keys for HookContext.Values used by experimental packages.
// Using these constants instead of raw strings prevents typos and makes
// cross-package dependencies explicit.
const (
	// StateKeyCompactionEvent stores the compaction event for post-generation hooks.
	StateKeyCompactionEvent = "compaction_event"
)
