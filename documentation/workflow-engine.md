# YAML Workflow Engine Design Decisions

## Overview

This document captures the key architectural decisions for designing a YAML-defined workflow engine that uses LLM interactions as activities, inspired by event-centric systems like Temporal.

## Core Architecture

### Event-Centric Design with Activities Pattern

**Decision**: Treat LLM prompt steps as activities (non-deterministic) while keeping workflow orchestration deterministic.

**Rationale**: 
- Provides fault tolerance and replay capabilities
- Clear separation between orchestration logic and external interactions
- Follows proven patterns from Temporal

**Implementation**:
- Each YAML prompt step = one activity execution
- Workflow coordination logic must be deterministic
- Activities (LLM calls) can be non-deterministic and are treated as black boxes

## Script Usage Categories

### Three Distinct Categories

**1. Conditional Scripts** - Must be deterministic
```yaml
- Name: Check Progress
  Condition: |
    len(state.completed_items) >= state.target_count && 
    state.quality_score > 0.8
```

**2. Template Scripts** - Must be deterministic  
```yaml
- Name: Generate Prompt
  Prompt: |
    Analyze ${format_item(state.current_item)}
    Previous results: ${build_context(state.results)}
```

**3. Activity Scripts** - Can be non-deterministic
```yaml
- Name: Fetch External Data
  Type: ScriptActivity
  Script: |
    response = http.get("https://api.example.com/data")
    return process_response(response)
  Store: external_data
```

**Rationale**: 
- Conditional and template scripts affect workflow decisions and must be replayable
- Activity scripts are isolated and their results are recorded, not their execution process
- Clear boundaries prevent accidental non-determinism in critical paths

### Script Enforcement Strategy

**Implementation**: Restrict available functions based on script context
- Conditional/Template: Only deterministic functions (no time, random, HTTP, etc.)
- Activity: All functions allowed including non-deterministic ones

## State Management

### Event Sourcing via Store Pattern

**Decision**: Use `Store:` syntax that internally generates "VariableSet" events

**User Interface**:
```yaml
- Name: Analyze Language
  Agent: Analyst  
  Prompt: "Analyze ${state.language}"
  Store: language_analysis  # Simple variable assignment
```

**Internal Implementation**:
```json
{
  "type": "VariableSet",
  "step_id": "analyze_language_step_1", 
  "variable": "language_analysis",
  "value": "Python is a high-level...",
  "timestamp": "2025-06-13T10:30:00Z"
}
```

**Benefits**:
- Users get simple mental model (set variable)
- Engine gets event sourcing benefits (replay, audit trail, fault tolerance)
- Clean abstraction hides complexity while preserving power

### Script Activity State Updates

**Decision**: Script activities use the same `Store:` pattern as other steps

**Rationale**:
- Consistent user experience across all step types
- Maintains activity pattern (input → output → store)
- Preserves event sourcing benefits
- Activities remain pure functions

**Example**:
```yaml
- Name: Process With External Data
  Type: ScriptActivity
  Script: |
    external_data = fetch_data(state.user_id)
    return process(external_data)
  Store: processed_result
```

## Variable Access Patterns

### Workflow Inputs

**Decision**: Use `inputs.<name>` pattern for accessing workflow inputs

**Implementation**:
```yaml
# Workflow definition
Inputs:
  user_id: string
  config:
    threshold: float

Steps:
  - Name: Process
    Script: |
      user_id = inputs.user_id
      threshold = inputs.config.threshold
```

**Rationale**:
- Clear distinction from state variables
- Explicit and unambiguous
- Prevents name conflicts

### State Variables

**Decision**: Use `state.<name>` pattern for accessing blackboard state

**Alternatives Considered**:
- `store.<name>` - Rejected due to verb/noun confusion with `Store:` YAML key
- `vars.<name>` - Good but less semantically accurate
- `data.<name>` - Too generic
- `blackboard.<name>` - More verbose, less familiar

**Final Choice**: `state.<name>`

**Rationale**:
- Most semantically accurate (represents workflow state)
- Familiar concept across many domains (state machines, UI frameworks)
- Clear distinction from inputs and local variables
- Prevents accidental assignment (read-only in scripts)

### Complete Access Pattern

**Final Pattern**:
```yaml
- Name: Complex Processing
  Type: ScriptActivity
  Script: |
    # Workflow inputs (immutable)
    user_id = inputs.user_id
    config = inputs.processing_config
    
    # Workflow state (read-only)
    previous_analysis = state.last_analysis
    current_step = state.current_step
    
    # Local variables
    external_data = fetch_external(user_id)
    
    return process_all(previous_analysis, external_data, config)
  Store: final_result
```

## Key Design Principles

1. **Deterministic Core**: Workflow orchestration and decision logic must be deterministic for replay
2. **Activity Isolation**: External interactions happen in activities that can be non-deterministic
3. **Event Sourcing**: All state changes via events for replay capability and audit trails
4. **Clear Abstractions**: Hide complexity (event sourcing) while preserving power
5. **Consistent Patterns**: Same mental models across all step types (Store, inputs, state)
6. **Safe Defaults**: Prevent accidental non-determinism through restricted script contexts

## Benefits of This Design

- **Reliability**: Event sourcing provides fault tolerance and replay capabilities
- **Usability**: Simple YAML interface hides implementation complexity
- **Flexibility**: Script activities allow non-deterministic work when needed
- **Debuggability**: Complete audit trail of all state changes
- **Consistency**: Uniform patterns across all workflow operations
- **Safety**: Clear boundaries prevent common mistakes

## Implementation Notes

- Script execution contexts must be set up differently based on usage (conditional vs activity)
- State objects should be read-only proxies in script contexts
- Event replay mechanism needed for workflow recovery
- Consider state validation schemas to prevent corruption