# Configuration Reference

Complete reference for configuring Dive environments, agents, and runtime settings.

## 📋 Table of Contents

- [Configuration Files](#configuration-files)
- [Environment Configuration](#environment-configuration)
- [Agent Configuration](#agent-configuration)
- [LLM Provider Configuration](#llm-provider-configuration)
- [Tool Configuration](#tool-configuration)
- [MCP Server Configuration](#mcp-server-configuration)
- [Runtime Configuration](#runtime-configuration)
- [Environment Variables](#environment-variables)
- [Examples](#examples)

## Configuration Files

Dive supports YAML and JSON configuration files:

```yaml
# dive.yaml - Main configuration file
Name: My Dive Environment
Description: Production AI agent environment
Version: "1.0.0"

Config:
  DefaultProvider: anthropic
  DefaultModel: claude-sonnet-4-20250514
  LogLevel: info
  MaxConcurrency: 10

Agents:
  - Name: Assistant
    # Agent configuration...

MCPServers:
  - Name: github
    # MCP server configuration...
```

### File Discovery

Dive looks for configuration files in this order:

1. File specified with `--config` flag
2. `dive.yaml` in current directory
3. `dive.yml` in current directory
4. `.dive/config.yaml` in current directory
5. `.dive/config.yml` in current directory

## Environment Configuration

### Basic Environment Settings

```yaml
Name: Development Environment
Description: Local development setup
Version: "1.0.0"

Config:
  # Default LLM provider for all agents
  DefaultProvider: anthropic  # anthropic, openai, groq, ollama
  
  # Default model name
  DefaultModel: claude-sonnet-4-20250514
  
  # Logging configuration
  LogLevel: info              # debug, info, warn, error
  LogFormat: text            # text, json
  LogOutput: stdout          # stdout, stderr, file:path
  
  # Concurrency limits
  MaxConcurrency: 5          # Max concurrent operations
  MaxAgentConcurrency: 3     # Max concurrent agents
  
  # Timeout settings
  DefaultTimeout: 30s        # Default operation timeout
  LLMTimeout: 60s           # LLM request timeout
  ToolTimeout: 30s          # Tool execution timeout
  
  # Memory and storage
  ThreadStorageType: memory  # memory, postgres, sqlite
  DocumentStorageType: memory
  
  # Security settings
  AllowUnsafeTools: false    # Allow potentially dangerous tools
  MaxTokens: 100000         # Maximum tokens per request
  
  # Development settings
  DevMode: false            # Enable development features
  HotReload: false          # Auto-reload on config changes
```

### Storage Configuration

```yaml
Config:
  # In-memory storage (default, not persistent)
  ThreadStorageType: memory
  
  # PostgreSQL storage
  ThreadStorageType: postgres
  PostgresConfig:
    Host: localhost
    Port: 5432
    Database: dive
    Username: dive_user
    Password: ${POSTGRES_PASSWORD}
    SSLMode: require
    
  # SQLite storage
  ThreadStorageType: sqlite
  SQLiteConfig:
    Path: ./data/dive.db
    
  # File-based storage
  DocumentStorageType: file
  FileStorageConfig:
    BasePath: ./data/documents
    MaxFileSize: 10MB
```

## Agent Configuration

### Basic Agent Settings

```yaml
Agents:
  - Name: Research Assistant
    Description: AI agent for research and analysis
    
    # Model configuration
    Provider: anthropic
    Model: claude-sonnet-4-20250514
    
    # Agent behavior
    Instructions: |
      You are a thorough research assistant who provides accurate, 
      well-sourced information. Always cite your sources.
      
    Temperature: 0.7           # Creativity/randomness (0.0-1.0)
    MaxTokens: 4000           # Maximum response tokens
    TopP: 0.9                 # Nucleus sampling parameter
    
    # Memory and context
    MaxContextLength: 100000   # Maximum context window
    ThreadRepository: default  # Thread storage repository
    
    # Tool configuration
    Tools:
      - web_search
      - read_file
      - write_file
      
    # Advanced settings
    StreamResponses: true      # Enable response streaming
    EnableFunctionCalling: true
    RetryAttempts: 3          # Number of retry attempts
    RetryDelay: 1s            # Delay between retries
```

### Agent Personalities

```yaml
Agents:
  # Formal, professional agent
  - Name: Business Analyst
    Instructions: |
      You are a professional business analyst with expertise in:
      - Financial analysis and forecasting
      - Market research and competitive analysis
      - Strategic planning and recommendations
      
      Communication style:
      - Professional and formal tone
      - Data-driven insights with supporting evidence
      - Clear executive summaries with actionable recommendations
      
    Temperature: 0.3  # Lower temperature for consistency
    
  # Creative, enthusiastic agent  
  - Name: Creative Writer
    Instructions: |
      You are an imaginative creative writer who excels at:
      - Storytelling and narrative development
      - Creative content generation
      - Engaging, vivid descriptions
      
      Communication style:
      - Enthusiastic and engaging tone
      - Rich, descriptive language
      - Creative solutions to problems
      
    Temperature: 0.8  # Higher temperature for creativity
```

### Specialized Agent Configurations

```yaml
Agents:
  # Code review agent
  - Name: Code Reviewer
    Instructions: |
      You are an expert code reviewer focusing on:
      - Code quality, readability, and maintainability
      - Security vulnerabilities and best practices
      - Performance optimization opportunities
      - Documentation and testing completeness
      
    Tools:
      - read_file
      - write_file
      - run_tests
      - static_analysis
    
    ModelConfig:
      Temperature: 0.2        # Low randomness for consistency
      MaxTokens: 6000
      
  # Data analysis agent
  - Name: Data Scientist
    Instructions: |
      You are a data scientist specializing in:
      - Statistical analysis and hypothesis testing
      - Data visualization and reporting  
      - Machine learning model development
      - Business intelligence insights
      
    Tools:
      - read_file
      - python_executor
      - generate_chart
      - query_database
      
    ModelConfig:
      Temperature: 0.4
      MaxTokens: 8000
```

## LLM Provider Configuration

### Anthropic (Claude)

```yaml
Config:
  Providers:
    anthropic:
      ApiKey: ${ANTHROPIC_API_KEY}
      BaseUrl: https://api.anthropic.com
      
      # Model-specific settings
      Models:
        claude-sonnet-4-20250514:
          MaxTokens: 4096
          Temperature: 0.7
          TopP: 0.9
          
        claude-haiku:
          MaxTokens: 2048
          Temperature: 0.5
          
      # Request settings
      Timeout: 60s
      MaxRetries: 3
      RetryDelay: 2s
      
      # Rate limiting
      RateLimitRPM: 1000        # Requests per minute
      RateLimitTPM: 100000      # Tokens per minute
```

### OpenAI (GPT)

```yaml
Config:
  Providers:
    openai:
      ApiKey: ${OPENAI_API_KEY}
      Organization: ${OPENAI_ORG_ID}  # Optional
      BaseUrl: https://api.openai.com/v1
      
      Models:
        gpt-4o:
          MaxTokens: 4000
          Temperature: 0.7
          TopP: 1.0
          FrequencyPenalty: 0
          PresencePenalty: 0
          
        gpt-4o-mini:
          MaxTokens: 2000
          Temperature: 0.5
          
      # Function calling
      EnableFunctionCalling: true
      FunctionCallTimeout: 30s
```

### Groq

```yaml
Config:
  Providers:
    groq:
      ApiKey: ${GROQ_API_KEY}
      BaseUrl: https://api.groq.com/openai/v1
      
      Models:
        mixtral-8x7b-32768:
          MaxTokens: 4000
          Temperature: 0.7
          
        llama2-70b-4096:
          MaxTokens: 2000
          Temperature: 0.6
          
      # Groq-specific settings
      StreamingEnabled: true
      MaxConcurrentRequests: 5
```

### Ollama (Local)

```yaml
Config:
  Providers:
    ollama:
      BaseUrl: http://localhost:11434
      
      Models:
        llama3:
          ContextLength: 4096
          Temperature: 0.7
          NumPredict: 2048
          
        codellama:
          ContextLength: 8192
          Temperature: 0.3
          
      # Local-specific settings
      PullMissingModels: true
      ModelTimeout: 300s        # Time to wait for model loading
```

## Tool Configuration

### Built-in Tool Configuration

```yaml
Config:
  Tools:
    # Web search configuration
    web_search:
      Provider: google
      GoogleApiKey: ${GOOGLE_SEARCH_API_KEY}
      GoogleSearchCx: ${GOOGLE_SEARCH_CX}
      MaxResults: 10
      SafeSearch: moderate
      
    # File operations
    file_operations:
      AllowedPaths:
        - ./documents/
        - ./data/
      MaxFileSize: 10MB
      AllowedExtensions: [txt, md, json, yaml, csv]
      
    # Command execution
    command_execution:
      AllowedCommands:
        - git
        - npm
        - python
      Timeout: 30s
      WorkingDirectory: ./
      Environment:
        NODE_ENV: development
```

### Custom Tool Registration

```yaml
Config:
  CustomTools:
    - Name: database_query
      Type: plugin
      Path: ./plugins/database.so
      Config:
        ConnectionString: ${DATABASE_URL}
        Timeout: 30s
        
    - Name: slack_integration
      Type: http
      BaseUrl: https://api.slack.com
      Config:
        Token: ${SLACK_TOKEN}
        DefaultChannel: "#general"
```

## MCP Server Configuration

### Built-in MCP Servers

```yaml
MCPServers:
  # GitHub integration
  - Name: github
    Type: url
    URL: https://mcp.github.com/sse
    Config:
      Token: ${GITHUB_TOKEN}
      DefaultOrg: myorg
      
  # File system access
  - Name: filesystem
    Type: stdio
    Command: npx
    Args: ["@modelcontextprotocol/server-filesystem", "./workspace"]
    
  # Database access
  - Name: postgres
    Type: stdio
    Command: npx
    Args: ["@modelcontextprotocol/server-postgres"]
    Config:
      ConnectionString: ${DATABASE_URL}
      
  # Web browsing
  - Name: brave-search
    Type: url
    URL: https://mcp.brave.com/sse
    Config:
      ApiKey: ${BRAVE_SEARCH_API_KEY}
```

### Custom MCP Server

```yaml
MCPServers:
  - Name: custom-service
    Type: stdio
    Command: ./bin/custom-mcp-server
    Args: ["--config", "./config/mcp.json"]
    Environment:
      API_KEY: ${CUSTOM_API_KEY}
      LOG_LEVEL: info
    
    # Server capabilities
    Capabilities:
      Resources: true
      Tools: true
      Prompts: true
      
    # Connection settings
    Timeout: 30s
    MaxRetries: 3
    RestartOnFailure: true
```

## Runtime Configuration

### Development Settings

```yaml
Config:
  # Development mode
  DevMode: true
  
  # Hot reload configuration changes
  HotReload: true
  WatchPaths:
    - ./configs/
    - ./agents/
    
  # Debug settings
  Debug:
    LogLLMRequests: true
    LogToolCalls: true
    SaveConversations: true
    ConversationPath: ./logs/conversations/
    
  # Performance profiling
  Profiling:
    Enabled: true
    Port: 6060
    Path: /debug/pprof
```

### Production Settings

```yaml
Config:
  # Production optimizations
  DevMode: false
  LogLevel: warn
  
  # Resource limits
  MaxConcurrency: 20
  MaxMemoryUsage: 1GB
  MaxDiskUsage: 10GB
  
  # Monitoring
  Metrics:
    Enabled: true
    Port: 9090
    Path: /metrics
    
  Health:
    Enabled: true
    Port: 8080
    Path: /health
    
  # Security
  Security:
    EnableTLS: true
    CertFile: ./certs/server.crt
    KeyFile: ./certs/server.key
    AllowedOrigins:
      - https://app.example.com
```

## Environment Variables

### Required Variables

```bash
# LLM Provider API Keys (at least one required)
ANTHROPIC_API_KEY=your-anthropic-key
OPENAI_API_KEY=your-openai-key
GROQ_API_KEY=your-groq-key

# Optional: Organization IDs
OPENAI_ORG_ID=your-org-id

# Database connection (if using persistent storage)
DATABASE_URL=postgresql://user:pass@localhost/dive
POSTGRES_PASSWORD=your-db-password

# External service API keys
GOOGLE_SEARCH_API_KEY=your-google-key
GOOGLE_SEARCH_CX=your-search-engine-id
GITHUB_TOKEN=your-github-token
SLACK_TOKEN=your-slack-token
```

### Optional Variables

```bash
# Configuration
DIVE_CONFIG_PATH=./config/production.yaml
DIVE_LOG_LEVEL=info
DIVE_LOG_FORMAT=json

# Runtime settings
DIVE_MAX_CONCURRENCY=10
DIVE_DEFAULT_TIMEOUT=30s
DIVE_DEV_MODE=false

# Storage settings
DIVE_DATA_DIR=./data
DIVE_CACHE_DIR=./cache

# Security
DIVE_ALLOW_UNSAFE_TOOLS=false
DIVE_MAX_TOKENS=100000
```

## Examples

### Complete Development Configuration

```yaml
Name: Development Environment
Description: Local development setup with all features enabled
Version: "1.0.0"

Config:
  DefaultProvider: anthropic
  DefaultModel: claude-sonnet-4-20250514
  LogLevel: debug
  DevMode: true
  HotReload: true
  MaxConcurrency: 3
  
  ThreadStorageType: sqlite
  SQLiteConfig:
    Path: ./dev-data/threads.db
    
  Tools:
    web_search:
      Provider: google
      GoogleApiKey: ${GOOGLE_SEARCH_API_KEY}
      GoogleSearchCx: ${GOOGLE_SEARCH_CX}

Agents:
  - Name: Dev Assistant
    Instructions: |
      You are a development assistant who helps with coding tasks.
      Be thorough but concise in your responses.
    Temperature: 0.5
    Tools:
      - web_search
      - read_file
      - write_file

Workflows:
  - Name: Code Review
    Inputs:
      - Name: file_path
        Type: string
        Required: true
        
    Steps:
      - Name: Read Code
        Agent: Dev Assistant
        Prompt: "Review the code in ${inputs.file_path} for quality and issues"
        Store: review_results
        
      - Name: Generate Report
        Action: Document.Write
        Parameters:
          Path: "reviews/review-${workflow.id}.md"
          Content: ${review_results}

MCPServers:
  - Name: filesystem
    Type: stdio
    Command: npx
    Args: ["@modelcontextprotocol/server-filesystem", "./"]
```

### Production Configuration

```yaml
Name: Production Environment
Description: Scalable production setup
Version: "2.0.0"

Config:
  DefaultProvider: anthropic
  DefaultModel: claude-sonnet-4-20250514
  LogLevel: info
  LogFormat: json
  DevMode: false
  MaxConcurrency: 20
  
  ThreadStorageType: postgres
  PostgresConfig:
    Host: db.example.com
    Port: 5432
    Database: dive_production
    Username: dive_user
    Password: ${POSTGRES_PASSWORD}
    SSLMode: require
    
  Security:
    EnableTLS: true
    CertFile: /etc/ssl/certs/dive.crt
    KeyFile: /etc/ssl/private/dive.key
    
  Metrics:
    Enabled: true
    Port: 9090

Agents:
  - Name: Customer Support
    Provider: anthropic
    Model: claude-sonnet-4-20250514
    Instructions: |
      You are a professional customer support agent.
      Be helpful, empathetic, and solution-focused.
    Temperature: 0.3
    MaxTokens: 2000
    Tools:
      - knowledge_base_search
      - ticket_management
      
  - Name: Content Moderator
    Provider: openai
    Model: gpt-4o-mini
    Instructions: |
      You moderate user-generated content for safety and compliance.
      Flag inappropriate content and provide detailed explanations.
    Temperature: 0.1
    Tools:
      - content_analysis
      - safety_check

Workflows:
  - Name: Support Ticket Processing
    Inputs:
      - Name: ticket_id
        Type: string
        Required: true
        
    Steps:
      - Name: Process Ticket
        Agent: Customer Support
        Prompt: "Handle support ticket ${inputs.ticket_id}"
        Timeout: 120s
        
      - Name: Update Status
        Action: Ticket.UpdateStatus
        Parameters:
          TicketId: ${inputs.ticket_id}
          Status: processed

MCPServers:
  - Name: knowledge-base
    Type: url
    URL: https://kb.example.com/mcp
    Config:
      ApiKey: ${KB_API_KEY}
```

This configuration reference provides comprehensive options for customizing Dive environments, agents, workflows, and runtime behavior. Start with basic configurations and gradually add complexity as your needs grow.