# Deterministic File Inputs

This example demonstrates how Dive tracks filesystem inputs to ensure deterministic workflow execution.

## The Problem

Previously, when workflows included file content (like `Path: '*.md'`), the actual file contents weren't tracked as part of the operation parameters. This meant:

- If files changed between runs, the workflow could produce different results
- Operation IDs were generated without considering file content
- Replay functionality couldn't guarantee identical results

## The Solution

Dive now implements **Content Fingerprinting** for deterministic tracking:

### How It Works

1. **Content Resolution**: Before executing operations, Dive resolves all file paths and loads content
2. **Hash Calculation**: Each piece of content gets a SHA-256 hash
3. **Operation Parameters**: Content hashes are included in operation parameters for deterministic operation IDs
4. **Replay Support**: Content snapshots enable perfect replay even if source files change

### Example

When this step executes:

```yaml
- Name: "Analyze Files"  
  Type: prompt
  Content:
    - Path: "*.md"           # Expands to sample1.md, sample2.md
    - Path: "workflow.yaml"  # Single file
```

Dive will:

1. **Expand wildcards**: Find all matching `.md` files
2. **Read content**: Load the actual file contents
3. **Calculate hashes**: Generate SHA-256 hashes for each file
4. **Create fingerprint**: Combine all hashes into a single content fingerprint
5. **Store in operation**: Include the fingerprint in operation parameters

### Operation Parameters

The operation parameters now include:

```json
{
  "agent": "gpt-4o-mini",
  "prompt": "Analyze these files...",
  "content_hash": "a1b2c3d4e5f6...",
  "content_sources": "combined", 
  "content_size": 2048
}
```

### Benefits

- **Deterministic**: Same file contents always produce the same operation ID
- **Replayable**: Operations can be replayed even if source files are modified
- **Trackable**: Changes to input files are automatically detected
- **Cacheable**: Results can be cached based on content fingerprints

## Testing the Feature

1. **Run the workflow**:
   ```bash
   dive run workflow.yaml
   ```

2. **Check the operation ID** in the logs - note the deterministic ID

3. **Run again without changes** - should use cached results

4. **Modify a file** (e.g., edit `sample1.md`) and run again - gets new operation ID

5. **Revert the file** to original content - returns to original operation ID

## File Types Supported

The content fingerprinting works with all supported file types:

- **Text files** (`.txt`, `.md`, `.csv`, `.json`): Content is hashed directly
- **Images** (`.png`, `.jpg`, etc.): Binary content is hashed
- **PDFs**: Binary content is hashed  
- **URLs**: URL string is hashed (since remote content can change)
- **Dynamic content**: Resolved content is hashed

## Technical Details

### Content Fingerprint Structure

```go
type ContentFingerprint struct {
    Hash    string // SHA-256 hash of content
    Source  string // Source type or path
    Size    int64  // Content size in bytes  
    ModTime string // File modification time (future)
}
```

### Snapshot Storage

Content snapshots are stored with operation results, enabling:

- Perfect replay of operations
- Audit trails of content changes
- Debugging of content-dependent issues

This ensures that your workflows are truly deterministic and reproducible, regardless of changes to the underlying filesystem. 