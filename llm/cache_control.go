package llm

// CacheControlType is used to control how the LLM caches responses.
type CacheControlType string

const (
	CacheControlTypeEphemeral CacheControlType = "ephemeral"
)

func (c CacheControlType) String() string {
	return string(c)
}

// Cache TTL values accepted by providers that support an extended cache
// duration. The default (empty) TTL is the provider's standard 5-minute cache;
// CacheTTL1h requests the 1-hour extended cache where supported.
const (
	CacheTTL5m = "5m"
	CacheTTL1h = "1h"
)
