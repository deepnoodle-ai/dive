package memory

import (
	"context"
	"time"
)

// Message represents a single message in a conversation
type Message struct {
	ID        string                 `json:"id"`
	ThreadID  string                 `json:"thread_id"`
	Content   string                 `json:"content"`
	Role      string                 `json:"role"`
	CreatedAt time.Time              `json:"created_at"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Thread represents a conversation thread
type Thread struct {
	ID         string                 `json:"id"`
	ResourceID string                 `json:"resource_id"` // Usually the user ID
	Title      string                 `json:"title"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"` // Contains working memory
}

// MemoryConfig defines configuration options for memory retrieval
type MemoryConfig struct {
	// Number of most recent messages to retrieve
	LastMessages int `json:"last_messages,omitempty"`

	// Semantic recall configuration
	SemanticRecall *SemanticRecallConfig `json:"semantic_recall,omitempty"`

	// Working memory configuration
	WorkingMemory *WorkingMemoryConfig `json:"working_memory,omitempty"`
}

// SemanticRecallConfig defines settings for semantic search
type SemanticRecallConfig struct {
	Enabled      bool `json:"enabled"`
	TopK         int  `json:"top_k"` // Number of semantically similar messages to retrieve
	MessageRange struct {
		Before int `json:"before"` // Context messages before matches
		After  int `json:"after"`  // Context messages after matches
	} `json:"message_range"`
}

// WorkingMemoryConfig defines settings for working memory
type WorkingMemoryConfig struct {
	Enabled  bool   `json:"enabled"`
	Template string `json:"template,omitempty"` // Default template for working memory
}

// QueryOptions defines options for memory retrieval
type QueryOptions struct {
	// Thread ID to query
	ThreadID string

	// Resource ID (user ID) for cross-thread queries
	ResourceID string

	// Last N messages to retrieve
	Last int

	// Vector search query string
	VectorSearchString string

	// Whether to search across all threads for this resource
	CrossThread bool

	// Memory configuration
	Config *MemoryConfig
}

// Memory defines the interface for memory operations
type Memory interface {
	// Thread operations
	CreateThread(ctx context.Context, resourceID string, title string) (*Thread, error)
	GetThread(ctx context.Context, threadID string) (*Thread, error)
	GetThreadsByResource(ctx context.Context, resourceID string) ([]*Thread, error)
	UpdateThread(ctx context.Context, threadID string, title string, metadata map[string]interface{}) (*Thread, error)
	DeleteThread(ctx context.Context, threadID string) error

	// Message operations
	SaveMessage(ctx context.Context, message *Message) error
	SaveMessages(ctx context.Context, messages []*Message) error

	// Memory retrieval
	Query(ctx context.Context, opts QueryOptions) ([]*Message, error)

	// Working memory operations
	GetWorkingMemory(ctx context.Context, threadID string) (string, error)
	UpdateWorkingMemory(ctx context.Context, threadID string, memory string) error

	// Cross-thread memory operations
	GetUserProfile(ctx context.Context, resourceID string) (map[string]interface{}, error)
	MergeThreadMemories(ctx context.Context, resourceID string) (string, error)

	// System message generation
	GetSystemMessage(ctx context.Context, threadID string, config *MemoryConfig) (string, error)
}

// VectorStorage defines the interface for vector storage operations
type VectorStorage interface {
	CreateIndex(ctx context.Context, indexName string) error
	Upsert(ctx context.Context, indexName string, vectors [][]float32, metadata []map[string]interface{}) error
	Query(ctx context.Context, indexName string, queryVector []float32, topK int, filter map[string]interface{}) ([]VectorSearchResult, error)
	DeleteByFilter(ctx context.Context, indexName string, filter map[string]interface{}) error
}

// VectorSearchResult represents a result from vector search
type VectorSearchResult struct {
	ID       string                 `json:"id"`
	Score    float32                `json:"score"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Embedder defines the interface for text embedding operations
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}
