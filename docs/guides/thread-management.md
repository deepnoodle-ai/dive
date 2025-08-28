# Thread Management Guide

Thread management in Dive enables persistent conversations between users and agents, maintaining context across multiple interactions. This guide covers how to implement, store, and manage conversation threads effectively.

## 📋 Table of Contents

- [What is Thread Management?](#what-is-thread-management)
- [Basic Thread Usage](#basic-thread-usage)
- [Thread Repositories](#thread-repositories)
- [Multi-User Conversations](#multi-user-conversations)
- [Thread Lifecycle Management](#thread-lifecycle-management)
- [Advanced Thread Features](#advanced-thread-features)
- [Performance Optimization](#performance-optimization)
- [Best Practices](#best-practices)

## What is Thread Management?

Thread management provides:

- **Persistent Conversations** - Maintain context across multiple interactions
- **User Sessions** - Track individual user conversations separately
- **Context Preservation** - Remember previous messages and decisions
- **Multi-Turn Dialogues** - Support complex, ongoing conversations
- **Conversation History** - Access to complete interaction history

### Core Concepts

```go
type Thread struct {
    ID       string         `json:"id"`
    UserID   string         `json:"user_id,omitempty"`
    Messages []*llm.Message `json:"messages"`
    Metadata map[string]string `json:"metadata,omitempty"`
    CreatedAt time.Time     `json:"created_at"`
    UpdatedAt time.Time     `json:"updated_at"`
}
```

### Benefits

1. **Contextual Conversations** - Agents remember previous interactions
2. **Personalization** - Tailor responses based on conversation history
3. **Complex Problem Solving** - Handle multi-step reasoning tasks
4. **User Experience** - Seamless, natural conversation flow
5. **State Management** - Track conversation state and progress

## Basic Thread Usage

### Simple Thread Example

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/diveagents/dive"
    "github.com/diveagents/dive/agent"
    "github.com/diveagents/dive/llm/providers/anthropic"
    "github.com/diveagents/dive/objects"
)

func main() {
    // Create thread repository
    threadRepo := objects.NewInMemoryThreadRepository()

    // Create agent with thread support
    assistant, err := agent.New(agent.Options{
        Name: "Memory Assistant",
        Instructions: `You are a helpful assistant who remembers our conversation.
                      Reference previous messages when relevant and maintain context.`,
        Model:            anthropic.New(),
        ThreadRepository: threadRepo,
    })
    if err != nil {
        log.Fatal(err)
    }

    // Start a conversation thread
    threadID := "user-conversation-1"
    
    // First interaction
    fmt.Println("=== First Message ===")
    response1, err := assistant.CreateResponse(
        context.Background(),
        dive.WithThreadID(threadID),
        dive.WithUserID("alice"),
        dive.WithInput("Hi! My name is Alice and I'm working on a Go project about web APIs."),
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Assistant: %s\n\n", response1.Text())

    // Second interaction - agent should remember Alice and the project
    fmt.Println("=== Second Message ===")
    response2, err := assistant.CreateResponse(
        context.Background(),
        dive.WithThreadID(threadID),
        dive.WithInput("What are some best practices for error handling in Go web APIs?"),
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Assistant: %s\n\n", response2.Text())

    // Third interaction - continuing the conversation
    fmt.Println("=== Third Message ===")
    response3, err := assistant.CreateResponse(
        context.Background(),
        dive.WithThreadID(threadID),
        dive.WithInput("Can you show me an example of how to implement that?"),
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Assistant: %s\n", response3.Text())
}
```

### Interactive Chat with Memory

```go
func createInteractiveChatWithMemory() {
    threadRepo := objects.NewInMemoryThreadRepository()
    
    assistant, err := agent.New(agent.Options{
        Name: "Chat Assistant",
        Instructions: `You are a conversational AI assistant. Remember what users tell you
                      and reference previous parts of our conversation when helpful.`,
        Model:            anthropic.New(),
        ThreadRepository: threadRepo,
    })
    if err != nil {
        panic(err)
    }

    scanner := bufio.NewScanner(os.Stdin)
    fmt.Println("🤖 Persistent Chat Assistant (type 'quit' to exit)")
    fmt.Print("Enter your user ID: ")
    
    scanner.Scan()
    userID := strings.TrimSpace(scanner.Text())
    threadID := fmt.Sprintf("thread-%s", userID)

    for {
        fmt.Print("\nYou: ")
        if !scanner.Scan() {
            break
        }

        input := strings.TrimSpace(scanner.Text())
        if input == "quit" {
            break
        }

        response, err := assistant.CreateResponse(
            context.Background(),
            dive.WithThreadID(threadID),
            dive.WithUserID(userID),
            dive.WithInput(input),
        )
        if err != nil {
            fmt.Printf("Error: %v\n", err)
            continue
        }

        fmt.Printf("Assistant: %s\n", response.Text())
    }
}
```

## Thread Repositories

### In-Memory Repository

Best for development and testing:

```go
import "github.com/diveagents/dive/objects"

// Create in-memory repository
threadRepo := objects.NewInMemoryThreadRepository()

// Threads are stored in memory and lost when application restarts
agent, err := agent.New(agent.Options{
    ThreadRepository: threadRepo,
    // ... other options
})
```

### File-Based Repository

Persist threads to local files:

```go
import "github.com/diveagents/dive/objects"

// Create file-based repository
threadRepo := objects.NewFileThreadRepository("./threads")

// Threads are saved to ./threads directory
agent, err := agent.New(agent.Options{
    ThreadRepository: threadRepo,
    // ... other options
})
```

### Custom Database Repository

Implement persistent storage with databases:

```go
import (
    "database/sql"
    "encoding/json"
    "time"
    _ "github.com/lib/pq"
)

type PostgresThreadRepository struct {
    db *sql.DB
}

func NewPostgresThreadRepository(connectionString string) (*PostgresThreadRepository, error) {
    db, err := sql.Open("postgres", connectionString)
    if err != nil {
        return nil, err
    }

    // Create table if not exists
    _, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS threads (
            id VARCHAR PRIMARY KEY,
            user_id VARCHAR,
            messages JSONB NOT NULL,
            metadata JSONB,
            created_at TIMESTAMP DEFAULT NOW(),
            updated_at TIMESTAMP DEFAULT NOW()
        );
        
        CREATE INDEX IF NOT EXISTS idx_threads_user_id ON threads(user_id);
        CREATE INDEX IF NOT EXISTS idx_threads_updated_at ON threads(updated_at);
    `)
    if err != nil {
        return nil, err
    }

    return &PostgresThreadRepository{db: db}, nil
}

func (r *PostgresThreadRepository) GetThread(ctx context.Context, id string) (*dive.Thread, error) {
    var thread dive.Thread
    var messagesJSON, metadataJSON []byte

    err := r.db.QueryRowContext(ctx, `
        SELECT id, user_id, messages, metadata, created_at, updated_at
        FROM threads WHERE id = $1
    `, id).Scan(
        &thread.ID, &thread.UserID, &messagesJSON, &metadataJSON,
        &thread.CreatedAt, &thread.UpdatedAt,
    )

    if err == sql.ErrNoRows {
        return nil, dive.ErrThreadNotFound
    }
    if err != nil {
        return nil, err
    }

    // Unmarshal messages
    err = json.Unmarshal(messagesJSON, &thread.Messages)
    if err != nil {
        return nil, err
    }

    // Unmarshal metadata
    if len(metadataJSON) > 0 {
        err = json.Unmarshal(metadataJSON, &thread.Metadata)
        if err != nil {
            return nil, err
        }
    }

    return &thread, nil
}

func (r *PostgresThreadRepository) PutThread(ctx context.Context, thread *dive.Thread) error {
    messagesJSON, err := json.Marshal(thread.Messages)
    if err != nil {
        return err
    }

    metadataJSON, err := json.Marshal(thread.Metadata)
    if err != nil {
        return err
    }

    now := time.Now()
    if thread.CreatedAt.IsZero() {
        thread.CreatedAt = now
    }
    thread.UpdatedAt = now

    _, err = r.db.ExecContext(ctx, `
        INSERT INTO threads (id, user_id, messages, metadata, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6)
        ON CONFLICT (id) DO UPDATE SET
            messages = EXCLUDED.messages,
            metadata = EXCLUDED.metadata,
            updated_at = EXCLUDED.updated_at
    `, thread.ID, thread.UserID, messagesJSON, metadataJSON, 
       thread.CreatedAt, thread.UpdatedAt)

    return err
}

func (r *PostgresThreadRepository) DeleteThread(ctx context.Context, id string) error {
    _, err := r.db.ExecContext(ctx, "DELETE FROM threads WHERE id = $1", id)
    return err
}

// Additional methods for advanced functionality
func (r *PostgresThreadRepository) GetUserThreads(ctx context.Context, userID string) ([]*dive.Thread, error) {
    rows, err := r.db.QueryContext(ctx, `
        SELECT id, user_id, messages, metadata, created_at, updated_at
        FROM threads WHERE user_id = $1 
        ORDER BY updated_at DESC
    `, userID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var threads []*dive.Thread
    for rows.Next() {
        var thread dive.Thread
        var messagesJSON, metadataJSON []byte

        err := rows.Scan(
            &thread.ID, &thread.UserID, &messagesJSON, &metadataJSON,
            &thread.CreatedAt, &thread.UpdatedAt,
        )
        if err != nil {
            return nil, err
        }

        // Unmarshal JSON fields
        json.Unmarshal(messagesJSON, &thread.Messages)
        if len(metadataJSON) > 0 {
            json.Unmarshal(metadataJSON, &thread.Metadata)
        }

        threads = append(threads, &thread)
    }

    return threads, rows.Err()
}

func (r *PostgresThreadRepository) GetThreadsByDateRange(ctx context.Context, start, end time.Time) ([]*dive.Thread, error) {
    // Implementation for date range queries
    // Useful for analytics and cleanup
    return nil, nil
}
```

## Multi-User Conversations

### User Isolation

```go
func demonstrateUserIsolation() {
    threadRepo := objects.NewInMemoryThreadRepository()
    
    assistant, err := agent.New(agent.Options{
        Name:             "Multi-User Assistant",
        ThreadRepository: threadRepo,
        Model:            anthropic.New(),
    })
    if err != nil {
        panic(err)
    }

    // Alice's conversation
    aliceResponse, err := assistant.CreateResponse(
        context.Background(),
        dive.WithThreadID("alice-thread"),
        dive.WithUserID("alice"),
        dive.WithInput("My favorite color is blue."),
    )

    // Bob's conversation (separate thread)
    bobResponse, err := assistant.CreateResponse(
        context.Background(),
        dive.WithThreadID("bob-thread"),
        dive.WithUserID("bob"), 
        dive.WithInput("My favorite color is red."),
    )

    // Alice continues her conversation
    aliceResponse2, err := assistant.CreateResponse(
        context.Background(),
        dive.WithThreadID("alice-thread"),
        dive.WithUserID("alice"),
        dive.WithInput("What's my favorite color?"),
    )
    // Agent will remember Alice's favorite color is blue

    // Bob continues his conversation
    bobResponse2, err := assistant.CreateResponse(
        context.Background(),
        dive.WithThreadID("bob-thread"),
        dive.WithUserID("bob"),
        dive.WithInput("What's my favorite color?"),
    )
    // Agent will remember Bob's favorite color is red
}
```

### Shared Team Conversations

```go
func createTeamConversation() {
    threadRepo := objects.NewInMemoryThreadRepository()
    
    teamAgent, err := agent.New(agent.Options{
        Name: "Team Assistant",
        Instructions: `You help coordinate team work. Keep track of who is working on what
                      and maintain context about ongoing projects and decisions.`,
        ThreadRepository: threadRepo,
        Model:           anthropic.New(),
    })
    if err != nil {
        panic(err)
    }

    teamThreadID := "team-project-alpha"

    // Team member 1 starts discussion
    _, err = teamAgent.CreateResponse(
        context.Background(),
        dive.WithThreadID(teamThreadID),
        dive.WithUserID("alice"),
        dive.WithInput("I'm starting work on the authentication module. ETA is Friday."),
    )

    // Team member 2 adds information
    _, err = teamAgent.CreateResponse(
        context.Background(),
        dive.WithThreadID(teamThreadID),
        dive.WithUserID("bob"),
        dive.WithInput("I'll handle the database schema. Can we sync on the user model structure?"),
    )

    // Team member 3 asks for status
    response, err := teamAgent.CreateResponse(
        context.Background(),
        dive.WithThreadID(teamThreadID),
        dive.WithUserID("carol"),
        dive.WithInput("What's the current status of the authentication work?"),
    )
    
    // Agent will reference Alice's and Bob's previous messages
    fmt.Printf("Team Status: %s\n", response.Text())
}
```

## Thread Lifecycle Management

### Thread Cleanup

```go
type ThreadManager struct {
    repository dive.ThreadRepository
    maxAge     time.Duration
    maxMessages int
}

func NewThreadManager(repo dive.ThreadRepository) *ThreadManager {
    return &ThreadManager{
        repository:  repo,
        maxAge:     time.Hour * 24 * 30, // 30 days
        maxMessages: 1000,               // Max messages per thread
    }
}

func (tm *ThreadManager) CleanupOldThreads(ctx context.Context) error {
    // Implementation would depend on repository capabilities
    // For now, showing the concept
    
    cutoffDate := time.Now().Add(-tm.maxAge)
    log.Printf("Cleaning up threads older than %v", cutoffDate)
    
    // If repository supports it, delete old threads
    if cleaner, ok := tm.repository.(ThreadCleaner); ok {
        return cleaner.DeleteThreadsOlderThan(ctx, cutoffDate)
    }
    
    return nil
}

func (tm *ThreadManager) TrimLongThreads(ctx context.Context, threadID string) error {
    thread, err := tm.repository.GetThread(ctx, threadID)
    if err != nil {
        return err
    }
    
    if len(thread.Messages) > tm.maxMessages {
        // Keep system message and recent messages
        systemMessages := make([]*llm.Message, 0)
        for _, msg := range thread.Messages {
            if msg.Role == llm.RoleSystem {
                systemMessages = append(systemMessages, msg)
            }
        }
        
        // Keep last N user/assistant messages
        recentMessages := thread.Messages[len(thread.Messages)-tm.maxMessages+len(systemMessages):]
        
        // Combine system and recent messages
        thread.Messages = append(systemMessages, recentMessages...)
        
        return tm.repository.PutThread(ctx, thread)
    }
    
    return nil
}

// Interface for repositories that support cleanup
type ThreadCleaner interface {
    DeleteThreadsOlderThan(ctx context.Context, cutoff time.Time) error
}
```

### Thread Archival

```go
type ThreadArchiver struct {
    activeRepo   dive.ThreadRepository
    archiveRepo  dive.ThreadRepository
    archiveAge   time.Duration
}

func NewThreadArchiver(active, archive dive.ThreadRepository) *ThreadArchiver {
    return &ThreadArchiver{
        activeRepo:  active,
        archiveRepo: archive,
        archiveAge:  time.Hour * 24 * 7, // 7 days
    }
}

func (ta *ThreadArchiver) ArchiveInactiveThreads(ctx context.Context) error {
    cutoffDate := time.Now().Add(-ta.archiveAge)
    
    // Get threads to archive (implementation depends on repository)
    threadsToArchive, err := ta.getInactiveThreads(ctx, cutoffDate)
    if err != nil {
        return err
    }
    
    for _, thread := range threadsToArchive {
        // Move to archive
        if err := ta.archiveRepo.PutThread(ctx, thread); err != nil {
            log.Printf("Failed to archive thread %s: %v", thread.ID, err)
            continue
        }
        
        // Remove from active storage
        if err := ta.activeRepo.DeleteThread(ctx, thread.ID); err != nil {
            log.Printf("Failed to delete archived thread %s: %v", thread.ID, err)
        }
    }
    
    return nil
}

func (ta *ThreadArchiver) getInactiveThreads(ctx context.Context, cutoff time.Time) ([]*dive.Thread, error) {
    // This would need to be implemented based on repository capabilities
    // For PostgreSQL example:
    if pgRepo, ok := ta.activeRepo.(*PostgresThreadRepository); ok {
        return pgRepo.GetThreadsByDateRange(ctx, time.Time{}, cutoff)
    }
    
    return nil, fmt.Errorf("repository doesn't support date range queries")
}
```

## Advanced Thread Features

### Thread Metadata

```go
func useThreadMetadata() {
    threadRepo := objects.NewInMemoryThreadRepository()
    
    assistant, err := agent.New(agent.Options{
        Name:             "Metadata Assistant",
        ThreadRepository: threadRepo,
        Model:           anthropic.New(),
    })
    if err != nil {
        panic(err)
    }

    // Create thread with initial metadata
    threadID := "metadata-example"
    
    // First, create thread manually with metadata
    thread := &dive.Thread{
        ID:     threadID,
        UserID: "alice",
        Metadata: map[string]string{
            "project":     "web-api",
            "priority":    "high",
            "department":  "engineering",
            "created_by":  "alice",
        },
        Messages:  []*llm.Message{},
        CreatedAt: time.Now(),
    }
    
    err = threadRepo.PutThread(context.Background(), thread)
    if err != nil {
        panic(err)
    }

    // Use thread with metadata awareness
    response, err := assistant.CreateResponse(
        context.Background(),
        dive.WithThreadID(threadID),
        dive.WithUserID("alice"),
        dive.WithInput("I need help with my high-priority web API project."),
    )
    
    // Agent can access metadata through the thread
    fmt.Printf("Response: %s\n", response.Text())
}
```

### Thread Context Injection

```go
// Custom agent that injects thread context into prompts
func createContextAwareAgent(threadRepo dive.ThreadRepository) (*agent.Agent, error) {
    return agent.New(agent.Options{
        Name: "Context Aware Assistant",
        Instructions: `You are a helpful assistant. You have access to conversation metadata
                      that provides context about the user and their projects.`,
        ThreadRepository: threadRepo,
        Model:           anthropic.New(),
        
        // Custom system prompt that includes thread metadata
        SystemPromptTemplate: `You are {{.Name}}. {{.Instructions}}
        
        {{if .ThreadMetadata}}
        Current conversation context:
        {{range $key, $value := .ThreadMetadata}}
        - {{$key}}: {{$value}}
        {{end}}
        {{end}}`,
    })
}
```

### Thread Summarization

```go
type ThreadSummarizer struct {
    agent dive.Agent
    repo  dive.ThreadRepository
}

func NewThreadSummarizer(agent dive.Agent, repo dive.ThreadRepository) *ThreadSummarizer {
    return &ThreadSummarizer{
        agent: agent,
        repo:  repo,
    }
}

func (ts *ThreadSummarizer) SummarizeThread(ctx context.Context, threadID string) (string, error) {
    thread, err := ts.repo.GetThread(ctx, threadID)
    if err != nil {
        return "", err
    }
    
    // Convert thread messages to text
    var conversation strings.Builder
    for _, msg := range thread.Messages {
        conversation.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Text()))
    }
    
    // Create summarization prompt
    summaryPrompt := fmt.Sprintf(`Please provide a concise summary of this conversation:

%s

Summary should include:
- Key topics discussed
- Important decisions made
- Action items or next steps
- Overall context and outcomes`, conversation.String())

    // Use agent to generate summary
    response, err := ts.agent.CreateResponse(
        ctx,
        dive.WithInput(summaryPrompt),
    )
    if err != nil {
        return "", err
    }
    
    return response.Text(), nil
}

func (ts *ThreadSummarizer) SummarizeAndUpdateMetadata(ctx context.Context, threadID string) error {
    summary, err := ts.SummarizeThread(ctx, threadID)
    if err != nil {
        return err
    }
    
    // Update thread metadata with summary
    thread, err := ts.repo.GetThread(ctx, threadID)
    if err != nil {
        return err
    }
    
    if thread.Metadata == nil {
        thread.Metadata = make(map[string]string)
    }
    
    thread.Metadata["summary"] = summary
    thread.Metadata["summarized_at"] = time.Now().Format(time.RFC3339)
    
    return ts.repo.PutThread(ctx, thread)
}
```

## Performance Optimization

### Message Compression

```go
func optimizeThreadStorage() {
    // Implement message compression for large threads
    type CompressedThread struct {
        *dive.Thread
        CompressedMessages []byte `json:"compressed_messages"`
    }
    
    func compressMessages(messages []*llm.Message) ([]byte, error) {
        data, err := json.Marshal(messages)
        if err != nil {
            return nil, err
        }
        
        var buf bytes.Buffer
        writer := gzip.NewWriter(&buf)
        _, err = writer.Write(data)
        if err != nil {
            return nil, err
        }
        writer.Close()
        
        return buf.Bytes(), nil
    }
    
    func decompressMessages(compressed []byte) ([]*llm.Message, error) {
        reader, err := gzip.NewReader(bytes.NewReader(compressed))
        if err != nil {
            return nil, err
        }
        defer reader.Close()
        
        data, err := ioutil.ReadAll(reader)
        if err != nil {
            return nil, err
        }
        
        var messages []*llm.Message
        err = json.Unmarshal(data, &messages)
        return messages, err
    }
}
```

### Caching Strategy

```go
type CachedThreadRepository struct {
    underlying dive.ThreadRepository
    cache      map[string]*dive.Thread
    mutex      sync.RWMutex
    ttl        time.Duration
    lastAccess map[string]time.Time
}

func NewCachedThreadRepository(underlying dive.ThreadRepository) *CachedThreadRepository {
    ctr := &CachedThreadRepository{
        underlying: underlying,
        cache:      make(map[string]*dive.Thread),
        ttl:        time.Minute * 30,
        lastAccess: make(map[string]time.Time),
    }
    
    // Start cleanup goroutine
    go ctr.cleanupLoop()
    
    return ctr
}

func (ctr *CachedThreadRepository) GetThread(ctx context.Context, id string) (*dive.Thread, error) {
    ctr.mutex.Lock()
    defer ctr.mutex.Unlock()
    
    // Check cache first
    if thread, exists := ctr.cache[id]; exists {
        if time.Since(ctr.lastAccess[id]) < ctr.ttl {
            ctr.lastAccess[id] = time.Now()
            return thread, nil
        }
        // Expired, remove from cache
        delete(ctr.cache, id)
        delete(ctr.lastAccess, id)
    }
    
    // Load from underlying repository
    thread, err := ctr.underlying.GetThread(ctx, id)
    if err != nil {
        return nil, err
    }
    
    // Cache the result
    ctr.cache[id] = thread
    ctr.lastAccess[id] = time.Now()
    
    return thread, nil
}

func (ctr *CachedThreadRepository) PutThread(ctx context.Context, thread *dive.Thread) error {
    // Update underlying storage
    err := ctr.underlying.PutThread(ctx, thread)
    if err != nil {
        return err
    }
    
    // Update cache
    ctr.mutex.Lock()
    ctr.cache[thread.ID] = thread
    ctr.lastAccess[thread.ID] = time.Now()
    ctr.mutex.Unlock()
    
    return nil
}

func (ctr *CachedThreadRepository) cleanupLoop() {
    ticker := time.NewTicker(time.Minute * 5)
    defer ticker.Stop()
    
    for range ticker.C {
        ctr.mutex.Lock()
        now := time.Now()
        for id, lastAccess := range ctr.lastAccess {
            if now.Sub(lastAccess) > ctr.ttl {
                delete(ctr.cache, id)
                delete(ctr.lastAccess, id)
            }
        }
        ctr.mutex.Unlock()
    }
}
```

## Best Practices

### 1. Thread Naming Conventions

```go
// Good: Descriptive, structured thread IDs
func createWellNamedThreads() {
    // User-specific threads
    userThread := fmt.Sprintf("user-%s-main", userID)
    
    // Project-specific threads
    projectThread := fmt.Sprintf("project-%s-discussion", projectID)
    
    // Feature-specific threads
    featureThread := fmt.Sprintf("feature-%s-requirements", featureID)
    
    // Timestamped threads for sessions
    sessionThread := fmt.Sprintf("session-%s-%d", userID, time.Now().Unix())
}

// Avoid: Generic or non-descriptive IDs
func avoidBadThreadNaming() {
    // Too generic
    thread1 := "thread1"
    conversation := "conversation"
    
    // Non-descriptive
    randomID := generateRandomString()
}
```

### 2. Memory Management

```go
func manageThreadMemory() {
    // Implement message limits per thread
    const maxMessagesPerThread = 100
    
    agent, err := agent.New(agent.Options{
        Name: "Memory Managed Agent",
        ThreadRepository: &LimitedThreadRepository{
            underlying: threadRepo,
            maxMessages: maxMessagesPerThread,
        },
    })
}

type LimitedThreadRepository struct {
    underlying  dive.ThreadRepository
    maxMessages int
}

func (ltr *LimitedThreadRepository) PutThread(ctx context.Context, thread *dive.Thread) error {
    // Trim messages if exceeding limit
    if len(thread.Messages) > ltr.maxMessages {
        // Keep system messages and recent messages
        var systemMessages []*llm.Message
        var otherMessages []*llm.Message
        
        for _, msg := range thread.Messages {
            if msg.Role == llm.RoleSystem {
                systemMessages = append(systemMessages, msg)
            } else {
                otherMessages = append(otherMessages, msg)
            }
        }
        
        // Keep recent non-system messages
        keepCount := ltr.maxMessages - len(systemMessages)
        if keepCount > 0 && len(otherMessages) > keepCount {
            otherMessages = otherMessages[len(otherMessages)-keepCount:]
        }
        
        thread.Messages = append(systemMessages, otherMessages...)
    }
    
    return ltr.underlying.PutThread(ctx, thread)
}
```

### 3. Error Handling

```go
func robustThreadHandling(agent dive.Agent, threadID string) error {
    maxRetries := 3
    
    for attempt := 0; attempt < maxRetries; attempt++ {
        response, err := agent.CreateResponse(
            context.Background(),
            dive.WithThreadID(threadID),
            dive.WithInput("Hello"),
        )
        
        if err == nil {
            return nil
        }
        
        // Handle specific thread-related errors
        if errors.Is(err, dive.ErrThreadNotFound) {
            log.Printf("Thread %s not found, creating new thread", threadID)
            // Thread will be created automatically on next call
            continue
        }
        
        // Handle repository errors
        if isRepositoryError(err) {
            log.Printf("Repository error (attempt %d): %v", attempt+1, err)
            time.Sleep(time.Duration(attempt+1) * time.Second)
            continue
        }
        
        // Non-recoverable error
        return err
    }
    
    return fmt.Errorf("failed after %d attempts", maxRetries)
}

func isRepositoryError(err error) bool {
    // Check for database connection errors, timeouts, etc.
    return strings.Contains(err.Error(), "connection") ||
           strings.Contains(err.Error(), "timeout")
}
```

### 4. Testing Thread Management

```go
func TestThreadPersistence(t *testing.T) {
    // Use in-memory repository for testing
    threadRepo := objects.NewInMemoryThreadRepository()
    
    agent, err := agent.New(agent.Options{
        Name:             "Test Agent",
        Model:            &MockLLM{},
        ThreadRepository: threadRepo,
    })
    require.NoError(t, err)
    
    threadID := "test-thread"
    
    // First message
    response1, err := agent.CreateResponse(
        context.Background(),
        dive.WithThreadID(threadID),
        dive.WithInput("Remember my name is Alice"),
    )
    require.NoError(t, err)
    
    // Verify thread was created
    thread, err := threadRepo.GetThread(context.Background(), threadID)
    require.NoError(t, err)
    require.NotNil(t, thread)
    
    // Should have user message and assistant response
    assert.GreaterOrEqual(t, len(thread.Messages), 2)
    
    // Second message should maintain context
    response2, err := agent.CreateResponse(
        context.Background(),
        dive.WithThreadID(threadID),
        dive.WithInput("What's my name?"),
    )
    require.NoError(t, err)
    
    // Verify thread was updated
    thread, err = threadRepo.GetThread(context.Background(), threadID)
    require.NoError(t, err)
    assert.GreaterOrEqual(t, len(thread.Messages), 4) // 2 previous + 2 new
}
```

### 5. Security Considerations

```go
// User isolation and access control
type SecureThreadRepository struct {
    underlying dive.ThreadRepository
    accessControl AccessController
}

type AccessController interface {
    CanAccessThread(userID, threadID string) bool
    CanModifyThread(userID, threadID string) bool
}

func (str *SecureThreadRepository) GetThread(ctx context.Context, id string) (*dive.Thread, error) {
    userID := getUserIDFromContext(ctx)
    
    if !str.accessControl.CanAccessThread(userID, id) {
        return nil, fmt.Errorf("access denied to thread %s", id)
    }
    
    return str.underlying.GetThread(ctx, id)
}

func (str *SecureThreadRepository) PutThread(ctx context.Context, thread *dive.Thread) error {
    userID := getUserIDFromContext(ctx)
    
    if !str.accessControl.CanModifyThread(userID, thread.ID) {
        return fmt.Errorf("modification denied for thread %s", thread.ID)
    }
    
    return str.underlying.PutThread(ctx, thread)
}

func getUserIDFromContext(ctx context.Context) string {
    if userID, ok := ctx.Value("userID").(string); ok {
        return userID
    }
    return ""
}
```

## Next Steps

- [Agent Guide](agents.md) - Learn how agents use threads
- [Event Streaming](event-streaming.md) - Monitor thread activities
- [Workflow Guide](workflows.md) - Use threads in workflows
- [API Reference](../api/core.md) - Thread management APIs