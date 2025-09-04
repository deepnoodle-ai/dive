#!/bin/bash

# Example CI/CD integration script for dive watch
# This script can be used in CI pipelines to automatically review code changes

set -e

echo "🚀 Starting AI-powered code review with Dive Watch..."

# Configuration
WATCH_PATTERNS="src/**/*.go"
ON_CHANGE_ACTION="Review this code for security vulnerabilities, performance issues, and adherence to Go best practices. Provide specific recommendations with line numbers."
LOG_FILE="ai-review-$(date +%Y%m%d-%H%M%S).log"
TOOLS="Web.Search"

# Check if required environment variables are set
if [ -z "$ANTHROPIC_API_KEY" ] && [ -z "$OPENAI_API_KEY" ]; then
    echo "❌ Error: No API key found. Set ANTHROPIC_API_KEY or OPENAI_API_KEY"
    exit 1
fi

# Determine provider based on available API key
if [ -n "$ANTHROPIC_API_KEY" ]; then
    PROVIDER="anthropic"
    MODEL="claude-3-5-sonnet-20241022"
elif [ -n "$OPENAI_API_KEY" ]; then
    PROVIDER="openai"
    MODEL="gpt-4"
fi

echo "📋 Configuration:"
echo "   Provider: $PROVIDER"
echo "   Model: $MODEL"
echo "   Patterns: $WATCH_PATTERNS"
echo "   Log File: $LOG_FILE"
echo ""

# Run dive watch with CI/CD optimized settings
dive watch $WATCH_PATTERNS \
    --on-change "$ON_CHANGE_ACTION" \
    --provider "$PROVIDER" \
    --model "$MODEL" \
    --tools "$TOOLS" \
    --exit-on-error \
    --log-file "$LOG_FILE" \
    --ignore "*.test.go,vendor/**,*.pb.go" \
    --debounce 1000 \
    --agent-name "CI-Reviewer" \
    --system-prompt "You are a senior software engineer performing code review for a CI/CD pipeline. Focus on security, performance, and maintainability. Be concise but thorough."

echo "✅ AI code review completed successfully!"
echo "📄 Review results saved to: $LOG_FILE"