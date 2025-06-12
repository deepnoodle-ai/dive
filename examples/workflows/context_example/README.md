# Dynamic Script Content Example

This example demonstrates how to use dynamic script content in Dive workflows. Dynamic script content allows you to generate content at runtime using either inline Risor scripts or external script files.

## Features

### Inline Risor Scripts

Use the `Script` field to define inline Risor scripts that generate content dynamically:

```yaml
Content:
  - Script: |
      {
        "type": "text",
        "text": "Current timestamp: " + time.now().string()
      }
```

The script can:
- Return a simple string (becomes a text content block)
- Return a content object with `type` and relevant fields
- Return an array of content objects
- Access workflow variables through globals

### External Script Files

Use the `ScriptPath` field to reference external script files:

```yaml
Content:
  - ScriptPath: ./generate_content.py
```

External scripts:
- Can be written in any language (Python, Shell, etc.)
- Must output valid JSON to stdout
- Receive workflow globals via `DIVE_GLOBALS` environment variable
- Are automatically made executable before running

## Script Output Format

Scripts should return JSON in one of these formats:

### Simple Text
```json
"Hello World"
```

### Content Object
```json
{
  "type": "text",
  "text": "Generated content here"
}
```

### Multiple Content Blocks
```json
[
  {"type": "text", "text": "First block"},
  {"type": "text", "text": "Second block"}
]
```

### Image Content
```json
{
  "type": "image",
  "source_type": "url",
  "url": "https://example.com/image.png",
  "media_type": "image/png"
}
```

## Accessing Workflow Variables

Scripts can access workflow variables (stored via the `Store` field in workflow steps):

### In Risor Scripts
Variables are directly accessible:
```risor
{
  "type": "text", 
  "text": "Previous step result: " + previous_result
}
```

### In External Scripts
Variables are available via the `DIVE_GLOBALS` environment variable:

```python
#!/usr/bin/env python3
import json
import os

# Access workflow globals
globals_env = os.getenv('DIVE_GLOBALS', '{}')
globals_data = json.loads(globals_env)

# Use the data
content = {
    "type": "text",
    "text": f"Using stored value: {globals_data.get('my_variable', 'none')}"
}

print(json.dumps(content))
```

## Supported Content Types

The dynamic scripts can generate:

- **Text Content**: Simple text strings or formatted content
- **Image Content**: References to images via URL or base64 data
- **Document Content**: PDF or other document references

## File Extensions

For external scripts, Dive recognizes:
- `.risor`, `.rs` - Executed as Risor scripts (with access to globals)
- All other extensions - Executed as external programs with JSON output

## Security Considerations

- External scripts are made executable (`chmod 755`)
- Scripts run with the same permissions as the Dive process
- Be cautious when using user-provided script content
- Consider sandboxing for production environments

## Example Use Cases

1. **Dynamic timestamps**: Generate current time/date information
2. **API calls**: Fetch data from external services at runtime
3. **File system operations**: Read directory contents, file metadata
4. **Calculations**: Perform complex computations based on workflow state
5. **Conditional content**: Generate different content based on previous results 