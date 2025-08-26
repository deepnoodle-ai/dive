# Integration Examples

Real-world integration examples showing how to connect Dive with external services, databases, APIs, and enterprise systems.

## 📋 Table of Contents

- [Database Integrations](#database-integrations)
- [API Service Integrations](#api-service-integrations)  
- [Cloud Platform Integrations](#cloud-platform-integrations)
- [Enterprise System Integrations](#enterprise-system-integrations)
- [DevOps Tool Integrations](#devops-tool-integrations)
- [Communication Platform Integrations](#communication-platform-integrations)
- [E-commerce Platform Integrations](#e-commerce-platform-integrations)
- [Analytics and Monitoring Integrations](#analytics-and-monitoring-integrations)

## Database Integrations

### PostgreSQL Integration

Complete integration with PostgreSQL for persistent storage and analytics.

```go
package main

import (
    "context"
    "database/sql"
    "fmt"
    "log"
    
    "github.com/diveagents/dive"
    "github.com/diveagents/dive/agent"
    "github.com/diveagents/dive/llm/providers/anthropic"
    "github.com/diveagents/dive/objects"
    _ "github.com/lib/pq"
)

// PostgreSQL Thread Repository Implementation
type PostgresThreadRepository struct {
    db *sql.DB
}

func NewPostgresThreadRepository(connectionString string) (*PostgresThreadRepository, error) {
    db, err := sql.Open("postgres", connectionString)
    if err != nil {
        return nil, err
    }
    
    // Initialize schema
    if err := createThreadSchema(db); err != nil {
        return nil, err
    }
    
    return &PostgresThreadRepository{db: db}, nil
}

func (ptr *PostgresThreadRepository) CreateThread(ctx context.Context, thread *objects.Thread) error {
    query := `
        INSERT INTO threads (id, created_at, updated_at, metadata)
        VALUES ($1, $2, $3, $4)`
    
    _, err := ptr.db.ExecContext(ctx, query,
        thread.ID,
        thread.CreatedAt,
        thread.UpdatedAt,
        thread.Metadata,
    )
    return err
}

func (ptr *PostgresThreadRepository) GetThread(ctx context.Context, threadID string) (*objects.Thread, error) {
    query := `
        SELECT id, created_at, updated_at, metadata
        FROM threads WHERE id = $1`
    
    var thread objects.Thread
    err := ptr.db.QueryRowContext(ctx, query, threadID).Scan(
        &thread.ID,
        &thread.CreatedAt,
        &thread.UpdatedAt,
        &thread.Metadata,
    )
    if err != nil {
        return nil, err
    }
    
    // Load messages
    messages, err := ptr.getThreadMessages(ctx, threadID)
    if err != nil {
        return nil, err
    }
    thread.Messages = messages
    
    return &thread, nil
}

func (ptr *PostgresThreadRepository) AddMessage(ctx context.Context, threadID string, message *objects.Message) error {
    query := `
        INSERT INTO messages (id, thread_id, role, content, created_at, tool_calls, metadata)
        VALUES ($1, $2, $3, $4, $5, $6, $7)`
    
    _, err := ptr.db.ExecContext(ctx, query,
        message.ID,
        threadID,
        message.Role,
        message.Content,
        message.CreatedAt,
        message.ToolCalls,
        message.Metadata,
    )
    return err
}

// Database Analytics Tool
type DatabaseAnalyticsTool struct {
    db *sql.DB
}

func NewDatabaseAnalyticsTool(db *sql.DB) *DatabaseAnalyticsTool {
    return &DatabaseAnalyticsTool{db: db}
}

func (dat *DatabaseAnalyticsTool) Name() string {
    return "database_analytics"
}

func (dat *DatabaseAnalyticsTool) Description() string {
    return "Run analytical queries against the database for insights and reporting"
}

func (dat *DatabaseAnalyticsTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    queryType, _ := params["type"].(string)
    timeRange, _ := params["time_range"].(string)
    
    switch queryType {
    case "conversation_stats":
        return dat.getConversationStats(ctx, timeRange)
    case "user_activity":
        return dat.getUserActivity(ctx, timeRange)
    case "popular_topics":
        return dat.getPopularTopics(ctx, timeRange)
    default:
        return nil, fmt.Errorf("unsupported query type: %s", queryType)
    }
}

func (dat *DatabaseAnalyticsTool) getConversationStats(ctx context.Context, timeRange string) (interface{}, error) {
    query := `
        SELECT 
            DATE(created_at) as date,
            COUNT(*) as conversation_count,
            AVG(message_count) as avg_messages_per_conversation,
            COUNT(DISTINCT user_id) as unique_users
        FROM (
            SELECT 
                t.created_at,
                t.metadata->>'user_id' as user_id,
                COUNT(m.id) as message_count
            FROM threads t
            LEFT JOIN messages m ON t.id = m.thread_id
            WHERE t.created_at >= NOW() - INTERVAL '%s'
            GROUP BY t.id, t.created_at, t.metadata->>'user_id'
        ) subquery
        GROUP BY DATE(created_at)
        ORDER BY date DESC`
    
    rows, err := dat.db.QueryContext(ctx, fmt.Sprintf(query, timeRange))
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    var stats []map[string]interface{}
    for rows.Next() {
        var date string
        var convCount int
        var avgMessages float64
        var uniqueUsers int
        
        err := rows.Scan(&date, &convCount, &avgMessages, &uniqueUsers)
        if err != nil {
            return nil, err
        }
        
        stats = append(stats, map[string]interface{}{
            "date":                     date,
            "conversation_count":       convCount,
            "avg_messages_per_conversation": avgMessages,
            "unique_users":            uniqueUsers,
        })
    }
    
    return stats, nil
}

// Usage example
func setupDatabaseIntegration() error {
    // Create database connection
    db, err := sql.Open("postgres", "postgresql://user:pass@localhost/dive_prod")
    if err != nil {
        return err
    }
    
    // Create custom thread repository
    threadRepo, err := NewPostgresThreadRepository("postgresql://user:pass@localhost/dive_prod")
    if err != nil {
        return err
    }
    
    // Create analytics tool
    analyticsTool := NewDatabaseAnalyticsTool(db)
    
    // Create agent with database integration
    analyst, err := agent.New(agent.Options{
        Name:             "Database Analyst",
        Instructions:     "You analyze database metrics and provide insights about system usage.",
        Model:            anthropic.New(),
        ThreadRepository: threadRepo,
        Tools: []dive.Tool{
            dive.ToolAdapter(analyticsTool),
        },
    })
    if err != nil {
        return err
    }
    
    // Use the analyst
    response, err := analyst.CreateResponse(
        context.Background(),
        dive.WithInput("Analyze conversation trends from the last 30 days"),
    )
    if err != nil {
        return err
    }
    
    fmt.Printf("Analysis: %s\n", response.Text())
    return nil
}
```

### MongoDB Integration

```yaml
# Configuration for MongoDB integration
Name: MongoDB Content Management System
Description: AI-powered content management with MongoDB storage

Config:
  DefaultProvider: anthropic
  CustomTools:
    - Name: mongodb_operations
      Type: plugin
      Config:
        ConnectionString: ${MONGODB_URI}
        Database: content_management
        Collections:
          - articles
          - media
          - users
          - analytics

Agents:
  - Name: Content Manager
    Instructions: |
      You manage content stored in MongoDB. You can create, update,
      search, and organize content across different collections.
    Tools:
      - mongodb_operations
      - content_analyzer
      - media_processor

Workflows:
  - Name: Content Publishing Pipeline
    Steps:
      - Name: Store Content
        Agent: Content Manager
        Prompt: |
          Store this content in MongoDB:
          Title: ${inputs.title}
          Content: ${inputs.content}
          Author: ${inputs.author}
          Tags: ${inputs.tags}
          
          Ensure proper indexing and metadata.
        
      - Name: Process Media
        Condition: ${inputs.has_media} == true
        Agent: Content Manager
        Prompt: "Process and store associated media files"
        
      - Name: Update Analytics
        Action: MongoDB.UpdateStats
        Parameters:
          Collection: analytics
          Document:
            content_id: ${content.id}
            published_at: ${timestamp}
```

## API Service Integrations

### GitHub Integration

Complete GitHub integration for code analysis and repository management.

```go
package main

import (
    "context"
    "fmt"
    "net/http"
    
    "github.com/diveagents/dive"
    "github.com/diveagents/dive/toolkit"
    "github.com/google/go-github/v45/github"
    "golang.org/x/oauth2"
)

type GitHubIntegrationTool struct {
    client *github.Client
    owner  string
    repo   string
}

func NewGitHubIntegrationTool(token, owner, repo string) *GitHubIntegrationTool {
    ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
    tc := oauth2.NewClient(context.Background(), ts)
    client := github.NewClient(tc)
    
    return &GitHubIntegrationTool{
        client: client,
        owner:  owner,
        repo:   repo,
    }
}

func (git *GitHubIntegrationTool) Name() string {
    return "github_integration"
}

func (git *GitHubIntegrationTool) Description() string {
    return "Interact with GitHub repositories: create issues, PRs, analyze code, manage releases"
}

func (git *GitHubIntegrationTool) Parameters() toolkit.ToolParameters {
    return toolkit.ToolParameters{
        Type: "object",
        Properties: map[string]toolkit.ToolParameter{
            "action": {
                Type:        "string",
                Description: "GitHub action to perform",
                Enum:        []interface{}{"create_issue", "create_pr", "get_commits", "analyze_code", "get_issues"},
            },
            "title": {
                Type:        "string",
                Description: "Title for issues or PRs",
            },
            "body": {
                Type:        "string",
                Description: "Body content for issues or PRs",
            },
            "branch": {
                Type:        "string",
                Description: "Branch name for operations",
            },
            "base_branch": {
                Type:        "string", 
                Description: "Base branch for PRs",
                Default:     "main",
            },
        },
        Required: []string{"action"},
    }
}

func (git *GitHubIntegrationTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    action, _ := params["action"].(string)
    
    switch action {
    case "create_issue":
        return git.createIssue(ctx, params)
    case "create_pr":
        return git.createPR(ctx, params)
    case "get_commits":
        return git.getRecentCommits(ctx, params)
    case "analyze_code":
        return git.analyzeCodeQuality(ctx, params)
    case "get_issues":
        return git.getOpenIssues(ctx, params)
    default:
        return nil, fmt.Errorf("unsupported action: %s", action)
    }
}

func (git *GitHubIntegrationTool) createIssue(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    title, _ := params["title"].(string)
    body, _ := params["body"].(string)
    
    issueRequest := &github.IssueRequest{
        Title: &title,
        Body:  &body,
    }
    
    issue, _, err := git.client.Issues.Create(ctx, git.owner, git.repo, issueRequest)
    if err != nil {
        return nil, fmt.Errorf("failed to create issue: %w", err)
    }
    
    return map[string]interface{}{
        "number": issue.GetNumber(),
        "url":    issue.GetHTMLURL(),
        "state":  issue.GetState(),
    }, nil
}

func (git *GitHubIntegrationTool) createPR(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    title, _ := params["title"].(string)
    body, _ := params["body"].(string)
    branch, _ := params["branch"].(string)
    baseBranch, _ := params["base_branch"].(string)
    if baseBranch == "" {
        baseBranch = "main"
    }
    
    prRequest := &github.NewPullRequest{
        Title: &title,
        Body:  &body,
        Head:  &branch,
        Base:  &baseBranch,
    }
    
    pr, _, err := git.client.PullRequests.Create(ctx, git.owner, git.repo, prRequest)
    if err != nil {
        return nil, fmt.Errorf("failed to create PR: %w", err)
    }
    
    return map[string]interface{}{
        "number": pr.GetNumber(),
        "url":    pr.GetHTMLURL(),
        "state":  pr.GetState(),
    }, nil
}

func (git *GitHubIntegrationTool) getRecentCommits(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    opts := &github.CommitsListOptions{
        ListOptions: github.ListOptions{PerPage: 10},
    }
    
    commits, _, err := git.client.Repositories.ListCommits(ctx, git.owner, git.repo, opts)
    if err != nil {
        return nil, fmt.Errorf("failed to get commits: %w", err)
    }
    
    var result []map[string]interface{}
    for _, commit := range commits {
        result = append(result, map[string]interface{}{
            "sha":     commit.GetSHA(),
            "message": commit.GetCommit().GetMessage(),
            "author":  commit.GetCommit().GetAuthor().GetName(),
            "date":    commit.GetCommit().GetAuthor().GetDate(),
            "url":     commit.GetHTMLURL(),
        })
    }
    
    return result, nil
}

// Complete GitHub workflow example
func setupGitHubWorkflow() error {
    githubTool := NewGitHubIntegrationTool(
        os.Getenv("GITHUB_TOKEN"),
        "myorg",
        "myrepo",
    )
    
    agent, err := agent.New(agent.Options{
        Name: "GitHub Assistant",
        Instructions: `You help manage GitHub repositories. You can create issues,
                      pull requests, analyze code, and provide repository insights.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(githubTool),
        },
    })
    if err != nil {
        return err
    }
    
    // Example: Automated code review
    response, err := agent.CreateResponse(
        context.Background(),
        dive.WithInput(`
            Analyze the recent commits in the repository and create an issue
            if any code quality concerns are found. Focus on:
            - Large commit sizes
            - Missing documentation
            - Potential security issues
            - Code complexity
        `),
    )
    if err != nil {
        return err
    }
    
    fmt.Printf("GitHub Assistant Response: %s\n", response.Text())
    return nil
}
```

### Slack Integration

```yaml
Name: Slack Bot Integration
Description: AI-powered Slack bot with workflow automation

MCPServers:
  - Name: slack
    Type: url
    URL: https://mcp.slack.com/sse
    Config:
      Token: ${SLACK_BOT_TOKEN}
      SigningSecret: ${SLACK_SIGNING_SECRET}

Agents:
  - Name: Slack Assistant
    Instructions: |
      You are a helpful Slack bot that assists with:
      - Answering questions
      - Scheduling meetings
      - Creating reminders
      - Managing team workflows
      
      Be conversational and helpful. Use emojis appropriately.
    Tools:
      - slack_messaging
      - calendar_integration
      - team_directory
    Temperature: 0.7

Workflows:
  - Name: Handle Slack Message
    Inputs:
      - Name: channel
        Type: string
        Required: true
      - Name: user
        Type: string
        Required: true
      - Name: message
        Type: string
        Required: true
      - Name: thread_ts
        Type: string
        Required: false
        
    Steps:
      - Name: Process Message
        Agent: Slack Assistant
        Prompt: |
          Respond to this Slack message:
          
          Channel: ${inputs.channel}
          User: ${inputs.user}
          Message: "${inputs.message}"
          Thread: ${inputs.thread_ts}
          
          Provide a helpful, contextual response.
        Store: response
        
      - Name: Send Response
        Action: Slack.SendMessage
        Parameters:
          Channel: ${inputs.channel}
          Text: ${response}
          ThreadTS: ${inputs.thread_ts}
          
  - Name: Daily Standup Reminder
    Schedule: "0 9 * * MON-FRI"
    Steps:
      - Name: Send Reminder
        Action: Slack.SendMessage
        Parameters:
          Channel: "#development"
          Text: |
            🌅 Good morning team! Time for standup.
            
            Please share:
            • What you accomplished yesterday
            • What you're working on today  
            • Any blockers or help needed
```

## Cloud Platform Integrations

### AWS Integration

```go
package main

import (
    "context"
    "fmt"
    
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/s3"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
    "github.com/aws/aws-sdk-go-v2/service/lambda"
    "github.com/diveagents/dive"
    "github.com/diveagents/dive/toolkit"
)

type AWSIntegrationTool struct {
    s3Client       *s3.Client
    dynamoClient   *dynamodb.Client
    lambdaClient   *lambda.Client
}

func NewAWSIntegrationTool(ctx context.Context) (*AWSIntegrationTool, error) {
    cfg, err := config.LoadDefaultConfig(ctx)
    if err != nil {
        return nil, err
    }
    
    return &AWSIntegrationTool{
        s3Client:     s3.NewFromConfig(cfg),
        dynamoClient: dynamodb.NewFromConfig(cfg),
        lambdaClient: lambda.NewFromConfig(cfg),
    }, nil
}

func (aws *AWSIntegrationTool) Name() string {
    return "aws_integration"
}

func (aws *AWSIntegrationTool) Description() string {
    return "Interact with AWS services: S3, DynamoDB, Lambda, and more"
}

func (aws *AWSIntegrationTool) Parameters() toolkit.ToolParameters {
    return toolkit.ToolParameters{
        Type: "object",
        Properties: map[string]toolkit.ToolParameter{
            "service": {
                Type:        "string",
                Description: "AWS service to interact with",
                Enum:        []interface{}{"s3", "dynamodb", "lambda"},
            },
            "action": {
                Type:        "string",
                Description: "Action to perform",
            },
            "bucket": {
                Type:        "string",
                Description: "S3 bucket name",
            },
            "key": {
                Type:        "string", 
                Description: "S3 object key or DynamoDB key",
            },
            "table": {
                Type:        "string",
                Description: "DynamoDB table name",
            },
            "function": {
                Type:        "string",
                Description: "Lambda function name",
            },
        },
        Required: []string{"service", "action"},
    }
}

func (aws *AWSIntegrationTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    service, _ := params["service"].(string)
    action, _ := params["action"].(string)
    
    switch service {
    case "s3":
        return aws.handleS3Action(ctx, action, params)
    case "dynamodb":
        return aws.handleDynamoDBAction(ctx, action, params)
    case "lambda":
        return aws.handleLambdaAction(ctx, action, params)
    default:
        return nil, fmt.Errorf("unsupported service: %s", service)
    }
}

func (aws *AWSIntegrationTool) handleS3Action(ctx context.Context, action string, params map[string]interface{}) (interface{}, error) {
    bucket, _ := params["bucket"].(string)
    key, _ := params["key"].(string)
    
    switch action {
    case "list_objects":
        input := &s3.ListObjectsV2Input{Bucket: &bucket}
        result, err := aws.s3Client.ListObjectsV2(ctx, input)
        if err != nil {
            return nil, err
        }
        
        var objects []map[string]interface{}
        for _, obj := range result.Contents {
            objects = append(objects, map[string]interface{}{
                "key":           *obj.Key,
                "size":          obj.Size,
                "last_modified": obj.LastModified,
                "etag":          *obj.ETag,
            })
        }
        return objects, nil
        
    case "get_object":
        input := &s3.GetObjectInput{
            Bucket: &bucket,
            Key:    &key,
        }
        result, err := aws.s3Client.GetObject(ctx, input)
        if err != nil {
            return nil, err
        }
        defer result.Body.Close()
        
        return map[string]interface{}{
            "content_type":   *result.ContentType,
            "content_length": result.ContentLength,
            "last_modified":  result.LastModified,
        }, nil
        
    default:
        return nil, fmt.Errorf("unsupported S3 action: %s", action)
    }
}

// AWS-powered data processing workflow
func setupAWSWorkflow() error {
    awsTool, err := NewAWSIntegrationTool(context.Background())
    if err != nil {
        return err
    }
    
    agent, err := agent.New(agent.Options{
        Name: "AWS Data Processor",
        Instructions: `You process and analyze data stored in AWS services.
                      You can work with S3 buckets, DynamoDB tables, and Lambda functions
                      to create comprehensive data processing pipelines.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(awsTool),
        },
    })
    if err != nil {
        return err
    }
    
    response, err := agent.CreateResponse(
        context.Background(),
        dive.WithInput(`
            Process the data files in the 'analytics-data' S3 bucket.
            For each CSV file:
            1. Get the file metadata
            2. Analyze the data structure
            3. Store summary statistics in DynamoDB
            4. Trigger a Lambda function for further processing
        `),
    )
    if err != nil {
        return err
    }
    
    fmt.Printf("AWS Processing Result: %s\n", response.Text())
    return nil
}
```

### Docker and Kubernetes Integration

```yaml
Name: Container Management System
Description: AI-powered container and Kubernetes management

Config:
  CustomTools:
    - Name: docker_operations
      Type: plugin
      Config:
        DockerSocket: /var/run/docker.sock
        
    - Name: kubernetes_operations
      Type: plugin
      Config:
        KubeConfig: ~/.kube/config
        DefaultNamespace: default

Agents:
  - Name: DevOps Engineer
    Instructions: |
      You manage containerized applications and Kubernetes clusters.
      You can deploy, scale, monitor, and troubleshoot applications.
    Tools:
      - docker_operations
      - kubernetes_operations
      - monitoring_tools
    Temperature: 0.3

Workflows:
  - Name: Deploy Application
    Inputs:
      - Name: app_name
        Type: string
        Required: true
      - Name: image_tag
        Type: string
        Required: true
      - Name: replicas
        Type: integer
        Default: 3
      - Name: namespace
        Type: string
        Default: default
        
    Steps:
      - Name: Build Docker Image
        Agent: DevOps Engineer
        Prompt: |
          Build Docker image for application: ${inputs.app_name}
          Tag: ${inputs.image_tag}
          
          Ensure image is optimized and secure.
        Store: build_result
        
      - Name: Deploy to Kubernetes
        Agent: DevOps Engineer
        Prompt: |
          Deploy ${inputs.app_name}:${inputs.image_tag} to Kubernetes:
          
          Namespace: ${inputs.namespace}
          Replicas: ${inputs.replicas}
          
          Create deployment, service, and ingress resources.
          Apply security policies and resource limits.
        Store: deployment_result
        
      - Name: Verify Deployment
        Agent: DevOps Engineer
        Prompt: |
          Verify deployment is successful:
          ${deployment_result}
          
          Check:
          - Pod status
          - Service endpoints
          - Health checks
          - Resource usage
        Store: verification_result
        
  - Name: Scale Application
    Inputs:
      - Name: app_name
        Type: string
        Required: true
      - Name: target_replicas
        Type: integer
        Required: true
        
    Steps:
      - Name: Analyze Current State
        Agent: DevOps Engineer
        Prompt: |
          Analyze current state of application: ${inputs.app_name}
          Target replicas: ${inputs.target_replicas}
          
          Check resource usage, performance metrics, and scaling history.
        
      - Name: Execute Scaling
        Agent: DevOps Engineer
        Prompt: |
          Scale ${inputs.app_name} to ${inputs.target_replicas} replicas.
          Monitor the scaling process and ensure stability.
```

## Enterprise System Integrations

### CRM Integration (Salesforce)

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "strings"
    
    "github.com/diveagents/dive"
    "github.com/diveagents/dive/toolkit"
)

type SalesforceIntegrationTool struct {
    instanceURL string
    accessToken string
    client      *http.Client
}

func NewSalesforceIntegrationTool(instanceURL, accessToken string) *SalesforceIntegrationTool {
    return &SalesforceIntegrationTool{
        instanceURL: instanceURL,
        accessToken: accessToken,
        client:      &http.Client{},
    }
}

func (sf *SalesforceIntegrationTool) Name() string {
    return "salesforce_integration"
}

func (sf *SalesforceIntegrationTool) Description() string {
    return "Interact with Salesforce CRM: manage leads, opportunities, accounts, and contacts"
}

func (sf *SalesforceIntegrationTool) Parameters() toolkit.ToolParameters {
    return toolkit.ToolParameters{
        Type: "object",
        Properties: map[string]toolkit.ToolParameter{
            "action": {
                Type:        "string",
                Description: "Salesforce action to perform",
                Enum:        []interface{}{"create_lead", "update_opportunity", "get_account", "search_contacts", "run_report"},
            },
            "object_type": {
                Type:        "string",
                Description: "Salesforce object type",
                Enum:        []interface{}{"Lead", "Opportunity", "Account", "Contact", "Case"},
            },
            "data": {
                Type:        "object",
                Description: "Data for create/update operations",
            },
            "query": {
                Type:        "string",
                Description: "SOQL query or search term",
            },
        },
        Required: []string{"action"},
    }
}

func (sf *SalesforceIntegrationTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    action, _ := params["action"].(string)
    
    switch action {
    case "create_lead":
        return sf.createLead(ctx, params)
    case "update_opportunity":
        return sf.updateOpportunity(ctx, params)
    case "get_account":
        return sf.getAccount(ctx, params)
    case "search_contacts":
        return sf.searchContacts(ctx, params)
    case "run_report":
        return sf.runReport(ctx, params)
    default:
        return nil, fmt.Errorf("unsupported action: %s", action)
    }
}

func (sf *SalesforceIntegrationTool) createLead(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    data, ok := params["data"].(map[string]interface{})
    if !ok {
        return nil, fmt.Errorf("data parameter required for create_lead")
    }
    
    // Prepare API request
    apiURL := fmt.Sprintf("%s/services/data/v58.0/sobjects/Lead", sf.instanceURL)
    
    jsonData, err := json.Marshal(data)
    if err != nil {
        return nil, err
    }
    
    req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(string(jsonData)))
    if err != nil {
        return nil, err
    }
    
    req.Header.Set("Authorization", "Bearer "+sf.accessToken)
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := sf.client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var result map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }
    
    return result, nil
}

func (sf *SalesforceIntegrationTool) runReport(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    query, _ := params["query"].(string)
    
    // Execute SOQL query
    apiURL := fmt.Sprintf("%s/services/data/v58.0/query?q=%s", sf.instanceURL, url.QueryEscape(query))
    
    req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
    if err != nil {
        return nil, err
    }
    
    req.Header.Set("Authorization", "Bearer "+sf.accessToken)
    
    resp, err := sf.client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var result map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }
    
    return result, nil
}

// CRM workflow example
func setupCRMWorkflow() error {
    sfTool := NewSalesforceIntegrationTool(
        os.Getenv("SALESFORCE_INSTANCE_URL"),
        os.Getenv("SALESFORCE_ACCESS_TOKEN"),
    )
    
    crmAgent, err := agent.New(agent.Options{
        Name: "CRM Assistant",
        Instructions: `You help manage customer relationships in Salesforce.
                      You can create leads, update opportunities, analyze sales data,
                      and provide insights about customer interactions.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(sfTool),
        },
    })
    if err != nil {
        return err
    }
    
    // Example: Lead qualification workflow
    response, err := crmAgent.CreateResponse(
        context.Background(),
        dive.WithInput(`
            Analyze recent leads created in the last week.
            For each lead:
            1. Check if they match our ideal customer profile
            2. Research their company size and industry
            3. Score them based on qualification criteria
            4. Update the lead record with scoring and notes
            5. Assign high-scoring leads to appropriate sales reps
        `),
    )
    if err != nil {
        return err
    }
    
    fmt.Printf("CRM Analysis: %s\n", response.Text())
    return nil
}
```

### ERP Integration (SAP)

```yaml
Name: ERP Integration System
Description: AI-powered ERP data analysis and workflow automation

Config:
  CustomTools:
    - Name: sap_integration
      Type: http
      BaseUrl: ${SAP_BASE_URL}
      Config:
        Username: ${SAP_USERNAME}
        Password: ${SAP_PASSWORD}
        Client: ${SAP_CLIENT}
        
    - Name: financial_analyzer
      Type: plugin
      Config:
        CurrencyAPI: ${CURRENCY_API_KEY}

Agents:
  - Name: Financial Analyst
    Instructions: |
      You analyze financial data from ERP systems, create reports,
      and provide insights for business decision making.
    Tools:
      - sap_integration
      - financial_analyzer
      - report_generator
    Temperature: 0.2
    
  - Name: Operations Manager
    Instructions: |
      You manage operational data including inventory, procurement,
      and supply chain information from ERP systems.
    Tools:
      - sap_integration
      - inventory_optimizer
      - supplier_analyzer
    Temperature: 0.3

Workflows:
  - Name: Monthly Financial Report
    Schedule: "0 0 1 * *"  # First day of each month
    Steps:
      - Name: Extract Financial Data
        Agent: Financial Analyst
        Prompt: |
          Extract monthly financial data from SAP:
          
          Period: Previous month
          Include:
          - Revenue by division
          - Cost centers analysis
          - Budget vs actual
          - Cash flow statements
          - Key financial ratios
        Store: financial_data
        
      - Name: Analyze Trends
        Agent: Financial Analyst
        Prompt: |
          Analyze financial trends from the extracted data:
          ${financial_data}
          
          Identify:
          - Month-over-month changes
          - Year-over-year comparisons
          - Variance explanations
          - Risk indicators
          - Opportunities for improvement
        Store: trend_analysis
        
      - Name: Generate Executive Report
        Agent: Financial Analyst
        Prompt: |
          Create an executive financial report:
          
          Data: ${financial_data}
          Analysis: ${trend_analysis}
          
          Format for C-level presentation with:
          - Executive summary
          - Key metrics dashboard
          - Variance explanations
          - Actionable recommendations
          - Risk assessments
        Store: executive_report
        
      - Name: Distribute Report
        Action: Email.Send
        Parameters:
          To: ["cfo@company.com", "ceo@company.com"]
          Subject: "Monthly Financial Report - ${month_year}"
          Body: ${executive_report}
          Attachments: ["financial_dashboard.pdf"]
```

## DevOps Tool Integrations

### CI/CD Pipeline Integration

```yaml
Name: CI/CD Pipeline Assistant
Description: AI-powered continuous integration and deployment

MCPServers:
  - Name: jenkins
    Type: http
    BaseUrl: ${JENKINS_URL}
    Config:
      Username: ${JENKINS_USER}
      Token: ${JENKINS_TOKEN}
      
  - Name: gitlab
    Type: url
    URL: https://mcp.gitlab.com/sse
    Config:
      Token: ${GITLAB_TOKEN}
      ProjectId: ${GITLAB_PROJECT_ID}

Agents:
  - Name: CI/CD Engineer
    Instructions: |
      You manage CI/CD pipelines, analyze build failures,
      optimize deployment processes, and ensure code quality.
    Tools:
      - jenkins_operations
      - gitlab_integration
      - docker_build
      - test_analyzer
    Temperature: 0.2

Workflows:
  - Name: Pipeline Failure Analysis
    Triggers:
      - Event: pipeline_failed
        Source: jenkins
        
    Steps:
      - Name: Analyze Failure
        Agent: CI/CD Engineer
        Prompt: |
          Analyze this pipeline failure:
          
          Pipeline: ${trigger.pipeline_name}
          Build: ${trigger.build_number}
          Branch: ${trigger.branch}
          Error: ${trigger.error_message}
          
          Investigate:
          1. Root cause analysis
          2. Similar historical failures
          3. Code changes in this build
          4. Environment factors
          5. Recommended fixes
        Store: failure_analysis
        
      - Name: Auto-Fix Attempt
        Condition: ${failure_analysis.auto_fixable} == true
        Agent: CI/CD Engineer
        Prompt: |
          Attempt automatic fix based on analysis:
          ${failure_analysis}
          
          Apply fix and trigger rebuild if appropriate.
        Store: auto_fix_result
        
      - Name: Create Issue
        Condition: ${failure_analysis.auto_fixable} == false
        Action: GitLab.CreateIssue
        Parameters:
          Title: "Pipeline Failure: ${trigger.pipeline_name} #${trigger.build_number}"
          Description: |
            ## Pipeline Failure Analysis
            
            ${failure_analysis}
            
            ## Recommended Actions
            
            ${failure_analysis.recommendations}
          Labels: ["pipeline", "bug", "urgent"]
          Assignee: ${failure_analysis.suggested_assignee}
```

### Monitoring and Alerting Integration

```go
package main

import (
    "context"
    "fmt"
    "time"
    
    "github.com/diveagents/dive"
    "github.com/diveagents/dive/agent"
    "github.com/diveagents/dive/llm/providers/anthropic"
    "github.com/prometheus/client_golang/api"
    prometheus "github.com/prometheus/client_golang/api/v1"
)

type MonitoringIntegrationTool struct {
    prometheusClient prometheus.API
    grafanaURL       string
    alertManager     string
}

func NewMonitoringIntegrationTool(promURL, grafanaURL, alertManagerURL string) (*MonitoringIntegrationTool, error) {
    client, err := api.NewClient(api.Config{
        Address: promURL,
    })
    if err != nil {
        return nil, err
    }
    
    return &MonitoringIntegrationTool{
        prometheusClient: prometheus.NewAPI(client),
        grafanaURL:       grafanaURL,
        alertManager:     alertManagerURL,
    }, nil
}

func (mit *MonitoringIntegrationTool) Name() string {
    return "monitoring_integration"
}

func (mit *MonitoringIntegrationTool) Description() string {
    return "Query metrics from Prometheus, analyze alerts, and manage monitoring dashboards"
}

func (mit *MonitoringIntegrationTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
    action, _ := params["action"].(string)
    query, _ := params["query"].(string)
    
    switch action {
    case "query_metrics":
        return mit.queryMetrics(ctx, query)
    case "get_alerts":
        return mit.getActiveAlerts(ctx)
    case "analyze_anomalies":
        return mit.analyzeAnomalies(ctx, params)
    default:
        return nil, fmt.Errorf("unsupported action: %s", action)
    }
}

func (mit *MonitoringIntegrationTool) queryMetrics(ctx context.Context, query string) (interface{}, error) {
    result, warnings, err := mit.prometheusClient.Query(ctx, query, time.Now())
    if err != nil {
        return nil, err
    }
    
    return map[string]interface{}{
        "result":   result,
        "warnings": warnings,
        "query":    query,
    }, nil
}

// Monitoring workflow setup
func setupMonitoringWorkflow() error {
    monitoringTool, err := NewMonitoringIntegrationTool(
        "http://prometheus:9090",
        "http://grafana:3000",
        "http://alertmanager:9093",
    )
    if err != nil {
        return err
    }
    
    opsAgent, err := agent.New(agent.Options{
        Name: "Operations Engineer",
        Instructions: `You monitor system health, analyze performance metrics,
                      and respond to alerts. Provide actionable insights and
                      recommendations for system optimization.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(monitoringTool),
        },
    })
    if err != nil {
        return err
    }
    
    // Example: Automated performance analysis
    response, err := opsAgent.CreateResponse(
        context.Background(),
        dive.WithInput(`
            Analyze system performance for the last hour:
            
            1. Check CPU and memory usage across all nodes
            2. Analyze response times for critical services
            3. Review error rates and failed requests
            4. Identify any performance anomalies
            5. Provide optimization recommendations
        `),
    )
    if err != nil {
        return err
    }
    
    fmt.Printf("Performance Analysis: %s\n", response.Text())
    return nil
}
```

These integration examples demonstrate how Dive can connect with virtually any external system through:

- **Custom tools** for direct API integration
- **MCP servers** for standardized protocols
- **Database connections** for data persistence
- **Webhook handlers** for event-driven workflows
- **Configuration-based setup** for easy deployment

Each integration pattern can be adapted for different services and use cases, providing a foundation for building sophisticated AI-powered automation systems that work seamlessly with existing enterprise infrastructure.