# Claude Code Agents: Explore and Plan

This document describes the Explore and Plan agent capabilities available in Claude Code, useful for understanding how to leverage these specialized agents when building AI applications.

## Overview

Claude Code provides specialized agents that can be invoked via the Task tool to handle specific types of work. Two key agents are:

- **Explore Agent**: Fast, read-only code exploration specialist
- **Plan Agent**: Software architect for designing implementation strategies

Both agents operate in **read-only mode** and cannot modify files.

---

## Explore Agent

### Purpose

The Explore agent is a read-only code exploration specialist designed to rapidly search through and analyze codebases. Primary purposes include:

- **Fast file discovery**: Locate files matching specific patterns or naming conventions
- **Code search and analysis**: Find implementations, usages, and patterns within code
- **Codebase navigation**: Understand structure and relationships between components
- **Content analysis**: Read and summarize file contents for research or documentation
- **Git history inspection**: Examine commit history and diffs to understand code evolution

### Tools Available

| Tool     | Purpose                                                                          |
| -------- | -------------------------------------------------------------------------------- |
| **Glob** | Fast file pattern matching (e.g., `**/*.ts`, `src/**/*.go`)                      |
| **Grep** | Regex-based content search with context lines and multiple output modes          |
| **Read** | Direct file access for code, images, PDFs, and Jupyter notebooks                 |
| **Bash** | Read-only terminal operations (`ls`, `find`, `git log`, `git diff`, etc.)        |
| **LSP**  | Language Server Protocol for definitions, references, hover info, call hierarchy |

### Thoroughness Levels

When invoking the Explore agent, specify a thoroughness level:

| Level             | Time     | When to Use                                                           |
| ----------------- | -------- | --------------------------------------------------------------------- |
| **quick**         | ~1-2 min | Initial surveys, finding a specific file/symbol, fast answers         |
| **medium**        | ~3-5 min | Understanding component interactions, tracing features, default level |
| **very thorough** | ~10+ min | Complete feature mapping, architecture analysis, security audits      |

### Best Suited For

- **Structure questions**: "What's the directory structure for module X?"
- **Implementation questions**: "How does authentication work?"
- **Usage questions**: "Where is function X called from?"
- **Pattern questions**: "What's the error handling pattern used?"
- **Navigation**: "What's in the tools directory?"
- **History**: "What changed in recent commits?"

### Example Prompts

```
# Quick
Find all Go files in the llm/providers directory and list their names.

# Medium
Explain how the Agent interface is implemented. Find the main Agent
implementation, show its methods, and find where it's instantiated.

# Very Thorough
Document the complete authentication flow. Find all files related to auth,
trace the execution path from entry point through validation, and identify
all error cases and special handling.
```

### Tips for Best Results

1. **Be specific about scope**: Specify directories, file types, or modules when possible
2. **Use meaningful search terms**: Search for function names, class names, or domain-specific terms
3. **Specify thoroughness upfront**: Tell the agent "quick," "medium," or "very thorough"
4. **Ask follow-up questions**: Dig deeper into specific areas after initial exploration
5. **Provide context**: Mention what the feature does or what you're trying to understand
6. **Request specific formats**: "Give me a list" vs. "Explain how" vs. "Show code snippets"

---

## Plan Agent

### Purpose

The Plan agent is a software architect and planning specialist designed to explore codebases and create detailed implementation plans. It analyzes existing code, understands architectural patterns, and designs solutions—all without making modifications.

### Primary Use Cases

- **Feature Planning**: Designing how to implement new features
- **Codebase Exploration**: Understanding unfamiliar code structures and patterns
- **Architecture Analysis**: Evaluating current architecture and proposing improvements
- **Refactoring Strategy**: Planning large-scale code reorganizations
- **Integration Design**: Planning how to integrate new libraries, APIs, or services
- **Migration Planning**: Designing migration paths for upgrades or architectural changes
- **Technical Debt Assessment**: Identifying and planning remediation of technical debt

### Tools Available

| Tool          | Purpose                                                                         |
| ------------- | ------------------------------------------------------------------------------- |
| **Read**      | Read file contents including code, images, PDFs, notebooks                      |
| **Glob**      | Fast file pattern matching                                                      |
| **Grep**      | Search file contents using regex patterns                                       |
| **Bash**      | Read-only commands: `ls`, `git status`, `git log`, `git diff`, etc.             |
| **LSP**       | Go-to-definition, find-references, hover info, document symbols, call hierarchy |
| **WebFetch**  | Fetch and analyze web content                                                   |
| **WebSearch** | Search the web for current information                                          |

### Typical Output Structure

The Plan agent provides structured output including:

1. **Exploration Summary**: Overview of relevant files, key interfaces, existing conventions
2. **Implementation Approach**: High-level strategy, trade-offs, phased approach if needed
3. **Step-by-Step Plan**: Detailed steps in dependency order with specific files and patterns
4. **Critical Files**: 3-5 most important files for the implementation
5. **Potential Challenges**: Edge cases, dependencies, risks and mitigation

### Example Prompts

```
# Feature Implementation
Plan how to add WebSocket support to this API server. Consider the existing
HTTP handler patterns and authentication middleware.

# Architectural Analysis
Analyze the current data access layer and design a plan for adding caching.
What patterns exist and how should caching integrate?

# Refactoring
The user authentication is scattered across multiple packages. Design a plan
to consolidate it into a single auth package while maintaining backward compatibility.

# Integration
Plan how to integrate Stripe payments into this e-commerce application.
Examine the existing payment-related code and design the integration.

# Migration
We need to migrate from REST to GraphQL. Analyze the current API structure
and create a phased migration plan.
```

### Tips for Best Results

1. **Be specific about goals**: "Add rate limiting with per-user and per-IP limits" vs. "Add a new feature"
2. **Provide context**: Mention relevant files, constraints, performance requirements
3. **Scope appropriately**: Break very large efforts into phases
4. **Ask follow-ups**: Request deeper exploration or alternative approaches
5. **Leverage exploration first**: Ask the agent to find similar patterns before designing new ones

---

## Plan Mode

Plan Mode is a **permission mode** in Claude Code—distinct from the Plan Agent. It restricts Claude to read-only operations for safe code analysis and planning.

### Plan Mode vs Plan Agent

| Concept | Type | Description |
|---------|------|-------------|
| **Plan Mode** | Permission mode | Restricts Claude to read-only operations; no file modifications allowed |
| **Plan Agent** | Subagent | Specialized agent invoked via Task tool for implementation planning |

Plan Mode is *how Claude operates*, while the Plan Agent is a *specialized subprocess* for research tasks.

### When to Use Plan Mode

**Use Plan Mode for:**
- Multi-step implementations requiring careful planning before execution
- Safe code exploration without risk of accidental modifications
- Complex refactoring where you want to iterate on the approach first
- Understanding unfamiliar codebases before making changes
- Interactive development where you want to review the plan before committing

**Skip Plan Mode when:**
- You're ready to make changes and want automatic execution
- Quick fixes that don't need planning
- Simple, single-file changes

### Entering and Exiting Plan Mode

**Option 1: Keyboard shortcut (during session)**
```
Shift+Tab  →  cycles through permission modes
            →  "accept edits on" (Auto-Accept)
            →  "plan mode on" (Plan Mode)
            →  back to default
```

**Option 2: Command line flag**
```bash
claude --permission-mode plan
```

**Option 3: Headless query**
```bash
claude --permission-mode plan -p "Analyze the auth system and suggest improvements"
```

### Tools Available in Plan Mode

| Tool | Available | Notes |
|------|-----------|-------|
| **Read** | Yes | Read file contents |
| **Glob** | Yes | File pattern matching |
| **Grep** | Yes | Content searching |
| **Bash** | Partial | Read-only commands only (`ls`, `git log`, `git diff`, etc.) |
| **Edit/Write** | No | Modifications not permitted |

### Permission Modes Overview

| Mode | Description |
|------|-------------|
| `default` | Standard behavior, prompts for permission on modifications |
| `acceptEdits` | Automatically accepts file edit permissions |
| `plan` | Read-only operations only, no modifications allowed |
| `bypassPermissions` | Skips all permission prompts (use with caution) |

### Example Workflows

**Planning a complex refactor:**
```bash
claude --permission-mode plan

> I need to refactor authentication to use OAuth2. Create a migration plan.

[Claude explores current auth implementation]
[Claude analyzes database schemas, API endpoints, configuration]
[Claude presents comprehensive plan with files, steps, compatibility notes]

> What about backward compatibility?

[Claude refines plan based on follow-up]

# When ready to implement, press Shift+Tab to exit Plan Mode
```

**Exploring a new codebase:**
```bash
> I just joined this project. Help me understand the architecture.

[Claude explores file structure, patterns, dependencies]
[Claude presents system design overview safely without modifications]
```

### Updating the Plan

Plans are iterative. You can refine and update the plan before implementation:

**Requesting changes:**
```bash
> Add error handling considerations to step 3

> What if we used Redis instead of in-memory caching?

> Break down the database migration into smaller steps

> Add rollback procedures for each step
```

**The plan iteration workflow:**

1. **Claude presents initial plan** after exploring the codebase
2. **You review and request changes** - ask questions, suggest alternatives, request more detail
3. **Claude updates the plan** - incorporates your feedback, explores additional code if needed
4. **Repeat** until you're satisfied with the approach
5. **Exit Plan Mode** (`Shift+Tab`) when ready to implement

**Comparing approaches:**
```bash
> Show me two alternatives: one using the existing auth middleware,
> another creating a new dedicated service

[Claude presents Option A and Option B with trade-offs]

> Let's go with Option B. Update the plan to use that approach.

[Claude revises the plan]
```

**Adding constraints:**
```bash
> We need to maintain backward compatibility with API v1

> The migration must be zero-downtime

> We can't add new dependencies to the project
```

Claude will re-evaluate and update the plan based on these constraints.

### Configure Plan Mode as Default

To make Plan Mode your default:

```json
// .claude/settings.json
{
  "permissions": {
    "defaultMode": "plan"
  }
}
```

---

## Invoking Agents

Agents are invoked using the Task tool with the `subagent_type` parameter:

```
Task(
  subagent_type: "Explore",
  prompt: "Find all error handling patterns in the API layer",
  description: "Explore error handling"
)

Task(
  subagent_type: "Plan",
  prompt: "Design an implementation plan for adding rate limiting",
  description: "Plan rate limiting feature"
)
```

### When to Use Each Agent

| Scenario                          | Agent   |
| --------------------------------- | ------- |
| "How does X work?"                | Explore |
| "Where is X implemented?"         | Explore |
| "What files contain X?"           | Explore |
| "How should we implement X?"      | Plan    |
| "Design a plan for X"             | Plan    |
| "What's the best approach for X?" | Plan    |

### Workflow Pattern

A common workflow is:

1. Use **Explore** to understand existing code and patterns
2. Use **Plan** to design the implementation strategy
3. Implement the plan (manually or with write-capable agents)

---

## Summary

### Agents

| Agent       | Purpose               | Mode      | Best For                                             |
| ----------- | --------------------- | --------- | ---------------------------------------------------- |
| **Explore** | Code exploration      | Read-only | Finding files, understanding code, tracing flows     |
| **Plan**    | Implementation design | Read-only | Feature planning, architecture, refactoring strategy |

Both agents help reduce context usage by offloading complex exploration and planning tasks to specialized subprocesses.

### Plan Mode vs Plan Agent

| Feature | Plan Mode | Plan Agent |
|---------|-----------|------------|
| **What it is** | Permission mode | Subagent |
| **How to invoke** | `Shift+Tab` or `--permission-mode plan` | `Task(subagent_type: "Plan", ...)` |
| **Scope** | Entire session | Single task |
| **Use case** | Safe exploration before implementing | Get implementation plan for a specific feature |

Use **Plan Mode** when you want Claude restricted to read-only for the entire session. Use the **Plan Agent** when you want a focused implementation plan for a specific task while maintaining normal permissions.
