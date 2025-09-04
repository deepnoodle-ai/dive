# Dive Diff Examples

This directory contains example files for demonstrating the `dive diff` functionality.

## API Response Comparison

The `old_api_response.json` and `new_api_response.json` files show how API responses might evolve over time. Use them to test semantic diff analysis:

```bash
# Basic diff showing size changes
dive diff examples/diff/old_api_response.json examples/diff/new_api_response.json

# AI-powered semantic analysis
dive diff examples/diff/old_api_response.json examples/diff/new_api_response.json --explain-changes

# Markdown output format
dive diff examples/diff/old_api_response.json examples/diff/new_api_response.json --explain-changes --format markdown
```

## Use Cases

### Output Drift Detection
Monitor changes in AI-generated content or API responses over time:

```bash
# Compare today's output with yesterday's
dive diff outputs/2024-01-14-results.txt outputs/2024-01-15-results.txt --explain-changes
```

### Code Review
Understand semantic changes in generated or modified code:

```bash
# Compare before and after refactoring
dive diff src/old_implementation.go src/new_implementation.go --explain-changes --format markdown
```

### Content Analysis
Analyze changes in documentation or text content:

```bash
# Compare document versions
dive diff docs/v1.0/README.md docs/v2.0/README.md --explain-changes
```

## Expected Output

When you run the semantic diff with `--explain-changes`, you should expect:

1. **Content Changes**: What information was added, removed, or modified
2. **Structural Changes**: How the organization or format changed  
3. **Semantic Meaning**: What the changes mean in context
4. **Impact Assessment**: How significant these changes are

The AI will provide natural language explanations rather than just technical diff output, making it easier to understand the implications of changes.