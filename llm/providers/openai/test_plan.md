# OpenAI Provider Test Coverage Plan

## Current Test Coverage Status

### Existing Tests
- **provider_test.go** (18KB, 719 lines)
  - ✅ Provider initialization with options
  - ✅ Request/response building and conversion
  - ✅ Basic Generate and Stream methods
  - ✅ Error handling (rate limits, auth errors)
  - ✅ Document content handling
  - ✅ Mock HTTP client approach
  - ✅ Basic integration tests

- **mcp_test.go** (6.3KB, 227 lines)
  - ✅ MCP server configuration
  - ✅ MCP approval handling
  - ✅ MCP streaming events

- **tool_image_generation_test.go** (2.3KB, 90 lines)
  - ✅ Interface implementation
  - ✅ Tool configuration

### Missing Test Files
- ❌ **tool_web_search_test.go** - No tests for web search tool
- ❌ **options_test.go** - Could add specific option tests
- ❌ **types_test.go** - JSON marshaling/unmarshaling tests

## Test Organization Strategy

### 1. Build Tags for Test Separation

```go
// Unit tests (default)
// +build !integration

// Integration tests
// +build integration
```

### 2. Directory Structure
```
openai/
├── provider_test.go          # Unit tests
├── provider_integration_test.go  # Integration tests
├── mcp_test.go              # MCP unit tests
├── tool_image_generation_test.go
├── tool_web_search_test.go  # NEW
├── types_test.go            # NEW
├── testdata/                # NEW
│   ├── responses/           # Mock API responses
│   └── fixtures/            # Test fixtures
```

## Detailed Test Coverage Plan

### 1. Core Provider Tests (provider_test.go)

#### Unit Tests
- [ ] **Provider Creation**
  - [ ] Test all option combinations
  - [ ] Test default values
  - [ ] Test environment variable loading
  - [ ] Test nil/empty options

- [ ] **Request Building**
  - [ ] Basic message conversion
  - [ ] System prompts
  - [ ] Multi-modal content (text, images, documents)
  - [ ] Tool configuration (standard tools)
  - [ ] Tool choice variations ("auto", "none", "required", specific tool)
  - [ ] MCP server configuration
  - [ ] Reasoning effort for o-series models
  - [ ] JSON schema output format
  - [ ] Temperature, max_tokens, other parameters
  - [ ] Metadata handling
  - [ ] Service tier
  - [ ] Previous response ID
  - [ ] Parallel tool calls
  - [ ] Request headers

- [ ] **Response Conversion**
  - [ ] Text responses
  - [ ] Tool calls (function_call)
  - [ ] Image generation results
  - [ ] Web search results
  - [ ] MCP calls and results
  - [ ] Error responses
  - [ ] Usage statistics
  - [ ] Reasoning results
  - [ ] Incomplete response handling

- [ ] **Streaming**
  - [ ] Message start/stop events
  - [ ] Content block start/delta/stop
  - [ ] Tool use events
  - [ ] Partial content updates
  - [ ] Error during streaming
  - [ ] Context cancellation mid-stream
  - [ ] Connection drops
  - [ ] Malformed SSE data

- [ ] **Error Handling**
  - [ ] HTTP errors (4xx, 5xx)
  - [ ] Network errors
  - [ ] JSON parsing errors
  - [ ] Rate limiting with retry-after
  - [ ] Context timeout/cancellation
  - [ ] Invalid API key
  - [ ] Model not found

- [ ] **Retry Logic**
  - [ ] Successful retry after transient error
  - [ ] Max retries exhausted
  - [ ] Exponential backoff timing
  - [ ] Non-retryable errors
  - [ ] Context cancellation during retry

### 2. MCP Tests (mcp_test.go - enhance existing)

- [ ] **MCP Configuration**
  - [ ] Multiple MCP servers
  - [ ] Different approval requirements
  - [ ] Custom headers
  - [ ] Tool filtering (allowed_tools)
  - [ ] Missing required fields

- [ ] **MCP Interactions**
  - [ ] Tool listing
  - [ ] Tool execution
  - [ ] Approval flow (request -> response)
  - [ ] Denial of approval
  - [ ] Server errors

### 3. Tool Tests

#### Image Generation (enhance existing)
- [ ] All configuration options
- [ ] Model selection
- [ ] Different sizes and qualities
- [ ] Output format options
- [ ] Compression settings
- [ ] Partial image streaming

#### Web Search (NEW: tool_web_search_test.go)
- [ ] Basic properties
- [ ] Configuration with domains
- [ ] Search context sizes
- [ ] User location variations
- [ ] Empty/nil configurations

### 4. Types Tests (NEW: types_test.go)

- [ ] **JSON Marshaling/Unmarshaling**
  - [ ] Request types
  - [ ] Response types
  - [ ] Streaming event types
  - [ ] Error types
  - [ ] Omitempty fields
  - [ ] Null vs missing fields

### 5. Integration Tests (provider_integration_test.go)

```go
// +build integration
```

- [ ] **Real API Tests**
  - [ ] Simple text generation
  - [ ] Tool use (if available)
  - [ ] Streaming responses
  - [ ] Error scenarios
  - [ ] Rate limiting behavior

- [ ] **Performance Tests**
  - [ ] Response time benchmarks
  - [ ] Token counting accuracy
  - [ ] Streaming latency

## Mock HTTP Client Enhancement

### Current Mock
- Basic request/response mocking
- Simple status code and body

### Enhanced Mock Features
- [ ] Request validation (headers, body structure)
- [ ] Response delays for timeout testing
- [ ] SSE streaming simulation
- [ ] Connection drop simulation
- [ ] Chunked response simulation
- [ ] Request history inspection

## Test Utilities

### Helper Functions
```go
// testutil.go
func NewTestProvider(opts ...Option) (*Provider, *MockHTTPClient)
func AssertJSONEqual(t *testing.T, expected, actual interface{})
func LoadTestData(t *testing.T, filename string) []byte
func GenerateMockStreamData(events []StreamEvent) string
```

### Test Fixtures
- Mock API responses for different scenarios
- Sample documents and images
- Error response templates
- Stream event sequences

## CI/CD Configuration

### Makefile Targets
```makefile
# Run unit tests only
test-unit:
	go test -v ./llm/providers/openai/...

# Run integration tests (requires OPENAI_API_KEY)
test-integration:
	go test -v -tags=integration ./llm/providers/openai/...

# Run all tests
test-all: test-unit test-integration

# Coverage report
test-coverage:
	go test -v -coverprofile=coverage.out ./llm/providers/openai/...
	go tool cover -html=coverage.out
```

### GitHub Actions
```yaml
- name: Run Unit Tests
  run: make test-unit

- name: Run Integration Tests
  if: github.event_name == 'push' && github.ref == 'refs/heads/main'
  env:
    OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
  run: make test-integration
```

## Test Coverage Goals

- **Unit Test Coverage**: >90%
- **Critical Path Coverage**: 100%
- **Integration Test Coverage**: Key workflows only

## Implementation Priority

1. **High Priority**
   - Web search tool tests
   - Enhanced streaming tests
   - Retry logic tests
   - Types marshaling tests

2. **Medium Priority**
   - Enhanced mock client
   - Test utilities
   - Additional error scenarios
   - O-series model features

3. **Low Priority**
   - Performance benchmarks
   - Edge case scenarios
   - Test fixture organization 