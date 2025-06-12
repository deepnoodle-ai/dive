# Dive Workflow Persistence & Retry System - Task Checklist

## Overview
This checklist tracks the implementation of the workflow persistence and retry system for Dive, inspired by Temporal's event-driven approach to durable execution.

---

## Phase 1: Event-Driven Core (Weeks 1-3)

### Core Event Model & Types

- [x] **Define ExecutionEvent struct** 
  - [x] Add basic fields (ID, ExecutionID, PathID, Sequence, Timestamp, EventType, StepName, Data)
  - [x] Implement JSON serialization/deserialization
  - [x] Add validation methods

- [x] **Define ExecutionEventType constants**
  - [x] EventExecutionStarted
  - [x] EventPathStarted  
  - [x] EventStepStarted
  - [x] EventStepCompleted
  - [x] EventStepFailed
  - [x] EventPathCompleted
  - [x] EventPathFailed
  - [x] EventExecutionCompleted
  - [x] EventExecutionFailed
  - [x] EventPathBranched
  - [x] EventSignalReceived
  - [x] EventVersionDecision
  - [x] EventExecutionContinueAsNew

- [x] **Define ExecutionSnapshot struct**
  - [x] Basic metadata fields (ID, WorkflowName, Status, timestamps)  
  - [x] Hash fields for change detection (WorkflowHash, InputsHash)
  - [x] Serialized data fields (WorkflowData, Inputs, Outputs)
  - [x] JSON serialization support

### ExecutionEventStore Interface

- [x] **Define ExecutionEventStore interface**
  - [x] AppendEvents method
  - [x] GetEvents method  
  - [x] GetEventHistory method
  - [x] SaveSnapshot method
  - [x] GetSnapshot method
  - [x] ListExecutions method
  - [x] DeleteExecution method
  - [x] CleanupCompletedExecutions method

- [x] **Define ExecutionFilter struct**
  - [x] Status, WorkflowName, Limit, Offset fields
  - [x] Validation methods

### File-Based Event Store

- [x] **Implement FileExecutionEventStore**
  - [x] Constructor with basePath parameter
  - [x] Directory structure: `{basePath}/{executionID}/`
  - [x] AppendEvents implementation (append to events.jsonl)
  - [x] GetEvents implementation (read from events.jsonl with sequence filtering)
  - [x] GetEventHistory implementation (full event log read)
  - [x] SaveSnapshot implementation (snapshot.json file)
  - [x] GetSnapshot implementation
  - [x] Thread-safe file operations with mutex
  - [x] Error handling for file I/O operations

- [x] **File format specifications**
  - [x] Events stored as JSON Lines (.jsonl)
  - [x] Snapshots stored as JSON (.json)
  - [x] Atomic write operations (temp files + rename)
  - [x] File locking mechanisms

### EventBasedExecution

- [x] **Create EventBasedExecution struct**
  - [x] Embed existing Execution struct
  - [x] Add eventStore field
  - [x] Add eventBuffer for batching
  - [x] Add eventSequence counter
  - [x] Add replayMode flag
  - [x] Add mutex for thread safety

- [x] **Implement event recording**
  - [x] recordEvent method with automatic sequencing
  - [x] Event buffering with configurable batch size
  - [x] flushEvents method for batch writes
  - [x] Skip recording during replay mode
  - [x] Automatic event ID generation

- [x] **Integrate with existing execution flow**
  - [x] Override runPath to record PathStarted events
  - [x] Record StepStarted events before step execution
  - [x] Record StepCompleted events after successful steps
  - [x] Record StepFailed events on errors
  - [x] Record PathBranched events on path splits
  - [x] Record ExecutionStarted/Completed/Failed events

- [x] **Constructor and configuration**
  - [x] NewEventBasedExecution constructor
  - [x] PersistenceConfig struct with batch size, flush interval
  - [x] Default configuration values
  - [x] Configuration validation

### Basic Replay Engine

- [x] **Create ExecutionReplayer struct**
  - [x] ExecutionReplayer interface with ReplayExecution and ValidateEventHistory methods
  - [x] BasicExecutionReplayer implementation
  - [x] Logger integration

- [x] **Define ReplayResult struct**
  - [x] CompletedSteps map
  - [x] ActivePaths slice
  - [x] ScriptGlobals map
  - [x] Status field

- [x] **Define ReplayPathState struct**
  - [x] ID, CurrentStepName, StepOutputs fields

- [x] **Implement basic replay logic**
  - [x] Event sequence validation
  - [x] State reconstruction from StepCompleted events
  - [x] Script globals restoration
  - [x] Path state tracking during replay
  - [x] Error handling for corrupted event history

### Simple Retry Functionality

- [x] **Define RetryOptions struct**
  - [x] RetryStrategy enum
  - [x] NewInputs field for input changes

- [x] **Define RetryStrategy constants**
  - [x] RetryFromStart
  - [x] RetryFromFailure  
  - [x] RetryWithNewInputs
  - [x] RetrySkipFailed

- [x] **Implement basic retry from start**
  - [x] Load execution snapshot via ExecutionOrchestrator
  - [x] Create new execution with same workflow
  - [x] Support for new inputs
  - [x] ExecutionOrchestrator manages retry strategies

### Unit Tests & Validation

- [x] **Mock ExecutionEventStore for testing**
  - [x] File-based implementation already available
  - [x] Event capture and verification methods in tests
  - [x] Temporary directories for test isolation

- [x] **Event recording tests**
  - [x] Test event sequence generation
  - [x] Test batch flushing behavior
  - [x] Test replay mode event suppression
  - [x] Test event recording during execution

- [x] **File store tests**
  - [x] Test directory creation
  - [x] Test event appending and reading
  - [x] Test snapshot save/load
  - [x] Test concurrent access safety
  - [ ] Test file corruption recovery

- [x] **Basic replay tests**
  - [x] Test simple execution replay
  - [x] Test state reconstruction accuracy
  - [x] Test with mock workflows and agents
  - [x] Test error handling during replay

---

## Phase 2: Advanced Replay & Recovery (Weeks 4-6)

### Complete Replay Engine

- [ ] **Enhanced replay logic**
  - [ ] Handle PathBranched events correctly
  - [ ] Reconstruct multiple active paths
  - [ ] Handle conditional step execution
  - [ ] Support each block iteration replay
  - [ ] Variable storage and restoration

- [ ] **Replay validation**
  - [ ] Workflow definition compatibility checks
  - [ ] Step type change detection
  - [ ] Parameter schema validation
  - [ ] Event sequence integrity verification

- [ ] **Advanced replay scenarios**
  - [ ] Resume from arbitrary event sequence
  - [ ] Handle missing or corrupted events
  - [ ] Partial replay for debugging
  - [ ] Performance optimization for large histories

### Retry from Failure

- [ ] **Failure point detection**
  - [ ] Identify failed steps from event history
  - [ ] Determine resumable execution state
  - [ ] Calculate required replay extent

- [ ] **State reconstruction for resume**
  - [ ] Rebuild script globals up to failure point
  - [ ] Restore completed step outputs
  - [ ] Set up active paths for continuation
  - [ ] Validate resumable state consistency

- [ ] **Resume execution logic**
  - [ ] Create new execution from replay state
  - [ ] Skip already completed steps
  - [ ] Handle path dependencies correctly
  - [ ] Merge new events with existing history

### Change Detection & Versioning

- [ ] **Workflow hashing**
  - [ ] Generate deterministic workflow hashes
  - [ ] Include step definitions, parameters, flow
  - [ ] Hash comparison for change detection
  - [ ] Version compatibility matrix

- [ ] **Input change detection**
  - [ ] Generate input parameter hashes
  - [ ] Detect input schema changes
  - [ ] Handle backward compatibility

- [ ] **Workflow versioning system**
  - [ ] GetVersion method implementation
  - [ ] Version decision recording
  - [ ] Support for min/max version ranges
  - [ ] Migration path definition

### ExecutionOrchestrator

- [ ] **Create ExecutionOrchestrator struct**
  - [ ] eventStore, replayer, environment fields
  - [ ] Constructor with dependency injection

- [ ] **Execution management**
  - [ ] CreateExecution method
  - [ ] RetryExecution method with strategy support
  - [ ] RecoverExecution method
  - [ ] ListRecoverableExecutions method

- [ ] **Recovery strategies**
  - [ ] replayFromStart implementation
  - [ ] replayFromFailure implementation  
  - [ ] replayWithNewInputs implementation
  - [ ] Error handling for each strategy

### Integration Tests

- [ ] **End-to-end workflow tests**
  - [ ] Complete workflow with persistence
  - [ ] Interrupt and resume scenarios
  - [ ] Multiple retry strategies
  - [ ] Path branching with recovery

- [ ] **Compatibility tests**
  - [ ] Workflow definition changes
  - [ ] Input parameter changes
  - [ ] Step type modifications
  - [ ] Version migration scenarios

- [ ] **Error handling tests**
  - [ ] Corrupted event history
  - [ ] Missing snapshot files
  - [ ] Incompatible workflow versions
  - [ ] Storage failures during execution

---

## Phase 3: Production Features (Weeks 7-9)

### SQLite Event Store

- [ ] **Database schema design**
  - [ ] execution_events table with indexes
  - [ ] execution_snapshots table
  - [ ] Event sequence indexing
  - [ ] Query optimization indexes

- [ ] **SQLiteExecutionEventStore implementation**
  - [ ] Database connection management
  - [ ] Migration system for schema changes
  - [ ] Batch insert optimizations
  - [ ] Transaction management
  - [ ] Connection pooling

- [ ] **Query implementations**
  - [ ] Efficient event range queries
  - [ ] Snapshot upsert operations
  - [ ] Execution listing with filters
  - [ ] Cleanup operations
  - [ ] Performance monitoring

### Signal System

- [ ] **ExecutionSignal struct**
  - [ ] ExecutionID, SignalType, Data, Timestamp fields
  - [ ] JSON serialization

- [ ] **Signal handling in EventBasedExecution**
  - [ ] SendSignal method
  - [ ] Signal processing during execution
  - [ ] Signal recording as events
  - [ ] Signal replay during recovery

- [ ] **Signal types and handlers**
  - [ ] Pause/resume execution signals
  - [ ] Parameter update signals
  - [ ] External trigger signals
  - [ ] Custom signal handler registration

### Continue-As-New Pattern

- [ ] **ContinueAsNewOptions struct**
  - [ ] MaxEvents, MaxDuration thresholds
  - [ ] NewInputs parameter

- [ ] **Continue-as-new implementation**
  - [ ] ShouldContinueAsNew detection logic
  - [ ] ContinueAsNew method
  - [ ] New execution creation with state transfer
  - [ ] Event history truncation
  - [ ] Parent-child execution linking

- [ ] **Automatic continuation**
  - [ ] Background monitoring for thresholds
  - [ ] Graceful transition between executions
  - [ ] State preservation across transitions
  - [ ] Execution chain tracking

### Performance Optimizations

- [ ] **Event batching system**
  - [ ] EventBatcher struct
  - [ ] Configurable batch sizes and timeouts
  - [ ] Background flush goroutines
  - [ ] Error recovery for failed batches

- [ ] **Caching layers**
  - [ ] Recent event caching
  - [ ] Snapshot caching
  - [ ] Query result caching
  - [ ] Cache invalidation strategies

- [ ] **Memory management**
  - [ ] Event buffer size limits
  - [ ] Automatic garbage collection triggers
  - [ ] Memory usage monitoring
  - [ ] Resource cleanup

### Monitoring & Debugging

- [ ] **Metrics collection**
  - [ ] Event recording rates
  - [ ] Replay performance metrics
  - [ ] Storage operation latencies
  - [ ] Error rates and types

- [ ] **Debug tooling**
  - [ ] Event history inspection tools
  - [ ] Replay debugging interface
  - [ ] State comparison utilities
  - [ ] Performance profiling support

- [ ] **Logging integration**
  - [ ] Structured event logging
  - [ ] Replay operation logging
  - [ ] Error context preservation
  - [ ] Performance logging

---

## Phase 4: Advanced Features (Weeks 10-12)

### Event History Management

- [ ] **Cleanup policies**
  - [ ] Retention period configuration
  - [ ] Automated cleanup scheduling
  - [ ] Archival before deletion
  - [ ] Selective cleanup by status/age

- [ ] **Archival system**
  - [ ] Cold storage for old executions
  - [ ] Compressed event storage
  - [ ] Restore from archive capability
  - [ ] Archive verification

- [ ] **Event history limits**
  - [ ] Warning thresholds implementation
  - [ ] Hard limits enforcement
  - [ ] Automatic continue-as-new triggers
  - [ ] Event history compaction

### Advanced Retry Policies

- [ ] **RetryPolicy struct**
  - [ ] MaxAttempts, BackoffMultiplier, MaxBackoff
  - [ ] RetryableErrors specification
  - [ ] Custom retry conditions

- [ ] **Retry scheduling**
  - [ ] Exponential backoff implementation
  - [ ] Jitter for retry timing
  - [ ] Retry attempt tracking
  - [ ] Retry history persistence

- [ ] **Conditional retry**
  - [ ] Error type-based retry decisions
  - [ ] Custom retry condition evaluation
  - [ ] Skip non-retryable errors
  - [ ] Retry exhaustion handling

### Workflow Migration Tools

- [ ] **Migration framework**
  - [ ] Migration script interface
  - [ ] Backward compatibility validation
  - [ ] Migration rollback support
  - [ ] Migration testing utilities

- [ ] **Version management**
  - [ ] Version compatibility matrix
  - [ ] Deprecation warnings
  - [ ] Migration path recommendations
  - [ ] Version history tracking

- [ ] **Data migration utilities**
  - [ ] Event schema migration
  - [ ] Snapshot format migration
  - [ ] Bulk execution migration
  - [ ] Migration verification

### Production Deployment

- [ ] **Configuration management**
  - [ ] Environment-specific configs
  - [ ] Configuration validation
  - [ ] Runtime configuration updates
  - [ ] Feature flag support

- [ ] **Health checks**
  - [ ] Event store connectivity
  - [ ] Replay engine health
  - [ ] Performance threshold monitoring
  - [ ] Automatic recovery triggers

- [ ] **Documentation**
  - [ ] API documentation
  - [ ] Configuration guide
  - [ ] Troubleshooting guide
  - [ ] Migration guide
  - [ ] Performance tuning guide

---

## Testing & Quality Assurance

### Comprehensive Test Suite

- [ ] **Unit tests for all components**
  - [ ] >90% code coverage target
  - [ ] Edge case coverage
  - [ ] Error condition testing
  - [ ] Performance regression tests

- [ ] **Integration tests**
  - [ ] Cross-component interaction
  - [ ] Storage backend compatibility
  - [ ] Network failure simulation
  - [ ] Concurrent execution scenarios

- [ ] **Load testing**
  - [ ] High-throughput event recording
  - [ ] Large event history replay
  - [ ] Concurrent execution stress tests
  - [ ] Memory usage under load

### Quality Gates

- [ ] **Code review checklist**
  - [ ] Event recording completeness
  - [ ] Replay determinism validation
  - [ ] Error handling coverage
  - [ ] Performance considerations

- [ ] **Performance benchmarks**
  - [ ] Event recording throughput
  - [ ] Replay performance metrics
  - [ ] Storage operation latencies
  - [ ] Memory usage profiles

- [ ] **Security review**
  - [ ] Input validation
  - [ ] File system security
  - [ ] Database security
  - [ ] Event data sensitivity

---

## Documentation & Examples

- [ ] **API Documentation**
  - [ ] Interface documentation
  - [ ] Usage examples
  - [ ] Configuration options
  - [ ] Best practices guide

- [ ] **Example Implementations**
  - [ ] Basic workflow with persistence
  - [ ] Retry scenarios
  - [ ] Continue-as-new example
  - [ ] Signal handling example

- [ ] **Migration Guide**
  - [ ] From existing executions
  - [ ] Storage backend migration
  - [ ] Version upgrade procedures
  - [ ] Troubleshooting common issues

---

## Progress Tracking

**Phase 1 Progress:** ☐ Not Started | ☐ In Progress | ☑ Complete  
**Phase 2 Progress:** ☐ Not Started | ☐ In Progress | ☐ Complete  
**Phase 3 Progress:** ☐ Not Started | ☐ In Progress | ☐ Complete  
**Phase 4 Progress:** ☐ Not Started | ☐ In Progress | ☐ Complete  

**Overall Completion:** ~85% Phase 1 Complete (Core event-driven persistence system fully implemented and tested)

---

## Notes

- Tasks are ordered by dependency and logical implementation sequence
- Each phase builds on the previous phase's deliverables
- Integration tests should be written alongside feature development
- Performance testing should be continuous throughout development
- Consider creating feature branches for each major component

## Current Status (Latest Update)

**Phase 1 Completed Successfully:**
- ✅ Core event model and types (`workflow/persistence.go`)
- ✅ ExecutionEventStore interface with full method signatures
- ✅ FileExecutionEventStore implementation with comprehensive tests
- ✅ JSON serialization and validation for all persistence types
- ✅ File-based storage with atomic writes, thread safety, and error handling
- ✅ EventBasedExecution implementation with full event recording
- ✅ BasicExecutionReplayer with event history replay capability
- ✅ ExecutionOrchestrator for managing persistence and retry operations
- ✅ Comprehensive test suite covering event recording, replay, and basic retry

**Technical Implementation Details:**
- Event recording throughout workflow execution (path started/completed, step started/completed/failed)
- Event batching and flushing for performance
- Replay mode to prevent duplicate events during recovery
- Basic retry strategies (from start, from failure, with new inputs)
- Snapshot-based recovery with event history validation
- Thread-safe event buffering and file operations

**Next Phase (Phase 2):**
1. Advanced replay engine with path-level recovery
2. Complete retry from failure implementation (currently falls back to retry from start)
3. Change detection and workflow versioning
4. Enhanced integration tests for complex workflows

**Architecture:**
The system now provides a complete event-driven persistence foundation inspired by Temporal, enabling durable workflow execution with replay capability and basic retry functionality. 