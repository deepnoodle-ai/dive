# Chat with Session History Example

This example demonstrates how to use Dive's chat command with persistent session history.

## Overview

The `dive chat` command now supports persistent conversation history through session files. This allows you to:

- Resume conversations across CLI sessions
- Save important conversations for later reference
- Share conversation histories with others
- Manage and organize multiple chat sessions

## Usage

### Basic Chat with Session

Start a chat session with history persistence:

```bash
dive chat --session ~/.dive/sessions/my-conversation.json --model claude-3-5-sonnet-20241022 --provider anthropic
```

### Resume an Existing Session

Simply use the same session file path to resume a previous conversation:

```bash
dive chat --session ~/.dive/sessions/my-conversation.json --model claude-3-5-sonnet-20241022 --provider anthropic
```

The agent will have access to the full conversation history and can reference previous messages.

### Single Message with Session

Send a single message and add it to the session history:

```bash
dive chat "What did we discuss about Go interfaces?" --session ~/.dive/sessions/my-conversation.json --model claude-3-5-sonnet-20241022 --provider anthropic
```

## Session Management

### List Sessions

View all session files in a directory:

```bash
dive session list ~/.dive/sessions/
```

This shows:
- File names and sizes
- Last modification times
- Number of threads and messages
- User information

### Inspect Session Details

View detailed information about a specific session:

```bash
dive session show ~/.dive/sessions/my-conversation.json
```

Add `--full` to see the complete conversation:

```bash
dive session show ~/.dive/sessions/my-conversation.json --full
```

### Clean Up Sessions

Remove empty session files:

```bash
dive session clean ~/.dive/sessions/ --empty
```

Remove sessions older than 7 days:

```bash
dive session clean ~/.dive/sessions/ --older-than 168h
```

Use `--dry-run` to preview what would be deleted:

```bash
dive session clean ~/.dive/sessions/ --empty --dry-run
```

## Session File Format

Session files are JSON documents containing an array of thread objects. Each thread contains:

- `id`: Thread identifier (typically "cli-chat" for CLI sessions)
- `user_id`: User identifier (optional)
- `created_at`: Thread creation timestamp
- `updated_at`: Last update timestamp
- `messages`: Array of message objects with roles and content

The format is designed to be human-readable and compatible with other tools.

## Best Practices

1. **Organize by Topic**: Use descriptive session file names
   ```bash
   ~/.dive/sessions/project-planning.json
   ~/.dive/sessions/code-review-2024-01-15.json
   ~/.dive/sessions/research-llm-agents.json
   ```

2. **Regular Cleanup**: Use the clean commands to manage storage
   ```bash
   # Weekly cleanup of old sessions
   dive session clean ~/.dive/sessions/ --older-than 168h
   ```

3. **Backup Important Sessions**: Session files are just JSON, so you can copy/backup them easily
   ```bash
   cp ~/.dive/sessions/important-conversation.json ~/backups/
   ```

4. **Share Sessions**: You can share session files with team members
   ```bash
   # Send session file to colleague
   dive session show my-session.json --full > conversation-summary.txt
   ```

## Integration with Other Commands

Session files work seamlessly with all chat features:

```bash
# Chat with tools and session history
dive chat --session ~/.dive/sessions/research.json \
          --tools web_search,document_read \
          --model claude-3-5-sonnet-20241022 \
          --provider anthropic

# Chat with custom system prompt and session
dive chat --session ~/.dive/sessions/coding.json \
          --system-prompt "You are a Go programming expert" \
          --model claude-3-5-sonnet-20241022 \
          --provider anthropic
```