package llm

// CacheControlType is used to control how the LLM caches responses.
type CacheControlType string

const (
	CacheControlTypeEphemeral CacheControlType = "ephemeral"
)

func (c CacheControlType) String() string {
	return string(c)
}
