# Dive Watch Examples

The `dive watch` command monitors files and directories for changes and triggers LLM actions automatically. This is particularly useful for:

- **Code Review Automation**: Automatically review code changes for best practices, security issues, or style compliance
- **CI/CD Integration**: Run automated checks in development or CI pipelines
- **Documentation Updates**: Automatically update documentation when code changes
- **Quality Assurance**: Continuous monitoring of code quality metrics

## Basic Usage

### Simple File Watching

```bash
# Watch Go files and lint them
dive watch src/*.go --on-change "Lint and suggest fixes"

# Watch Python files for PEP8 compliance
dive watch "**/*.py" --on-change "Check for PEP8 compliance and suggest improvements"
```

### Directory Watching

```bash
# Watch entire directory recursively
dive watch . --recursive --on-change "Review changes for security vulnerabilities"

# Watch specific directory with filtering
dive watch src/ --recursive --only-extensions "go,js,ts" --on-change "Code review"
```

## Advanced Features

### Extension Filtering

```bash
# Only watch specific file types
dive watch . --recursive --only-extensions "go,py,js" --on-change "Code review"
```

### Ignore Patterns

```bash
# Ignore test files and dependencies
dive watch . --recursive --ignore "*.test.go,node_modules/**,vendor/**" --on-change "Review production code"
```

### Tool Integration

```bash
# Use web search for enhanced reviews
dive watch src/ --recursive --on-change "Review code and search for best practices" --tools "Web.Search"

# Use document tools for documentation updates
dive watch docs/ --on-change "Update documentation based on changes" --tools "Document.Write"
```

## CI/CD Integration

### Exit on Error

For CI/CD pipelines, use `--exit-on-error` to make the process fail if the LLM detects issues:

```bash
# CI/CD pipeline integration
dive watch src/ --recursive --on-change "Perform security audit and fail on critical issues" --exit-on-error
```

### Logging

Use `--log-file` to capture all watch events and responses for CI/CD reporting:

```bash
# Log all activities for CI/CD reporting
dive watch . --recursive --on-change "Code quality check" --log-file watch-results.log --exit-on-error
```

### Git Hooks Integration

Create a `.git/hooks/pre-commit` script:

```bash
#!/bin/bash
# Run dive watch on staged files
staged_files=$(git diff --cached --name-only --diff-filter=ACM)
if [ -n "$staged_files" ]; then
    echo "Running AI code review on staged files..."
    dive watch $staged_files --on-change "Review for code quality, security, and best practices" --exit-on-error
fi
```

### GitHub Actions Integration

Example `.github/workflows/ai-review.yml`:

```yaml
name: AI Code Review
on: [pull_request]

jobs:
  ai-review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v4
        with:
          go-version: "1.21"

      - name: Install Dive
        run: go install github.com/deepnoodle-ai/dive/cmd/dive@latest

      - name: Run AI Code Review
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: |
          dive watch . --recursive --only-extensions "go,js,py" \
            --ignore "*.test.go,node_modules/**,vendor/**" \
            --on-change "Review changes for security, performance, and best practices" \
            --exit-on-error --log-file ai-review.log

      - name: Upload Review Results
        uses: actions/upload-artifact@v3
        if: always()
        with:
          name: ai-review-results
          path: ai-review.log
```

## Configuration Examples

### Custom System Prompts

```bash
# Custom system prompt for specific review focus
dive watch src/ --recursive \
  --system-prompt "You are a senior Go developer focused on performance optimization" \
  --on-change "Review for performance improvements and memory efficiency"
```

### Debouncing

```bash
# Adjust debounce time for rapid file changes
dive watch . --recursive --debounce 1000 --on-change "Review changes"
```

### Multiple Tools

```bash
# Use multiple tools for comprehensive review
dive watch src/ --recursive \
  --tools "Web.Search,Document.Read,Document.Write" \
  --on-change "Review code and update documentation if needed"
```

## Best Practices

1. **Use Specific Patterns**: Be specific about what files to watch to avoid unnecessary triggers
2. **Set Appropriate Debounce**: Use longer debounce times for projects with rapid file changes
3. **Filter Extensions**: Use `--only-extensions` to focus on relevant file types
4. **Ignore Build Artifacts**: Always ignore generated files, dependencies, and build outputs
5. **CI/CD Integration**: Use `--exit-on-error` and `--log-file` for automated pipelines
6. **Resource Management**: Be mindful of LLM API costs when watching large codebases

## Troubleshooting

### High CPU Usage

- Use more specific patterns instead of watching entire directories
- Increase debounce time with `--debounce`
- Use `--only-extensions` to filter file types

### Too Many Triggers

- Add ignore patterns for build artifacts and temporary files
- Use `--debounce` to reduce rapid fire events
- Be more specific with watch patterns

### LLM Errors

- Check API keys and rate limits
- Use `--log-file` to capture detailed error information
- Consider using `--reasoning-budget` for complex tasks
