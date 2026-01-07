---
description: Run Go tests with optional pattern
argument-hint: "[package-pattern]"
allowed-tools:
  - Bash
  - Read
  - Grep
  - Glob
---

Run Go tests in this repository.

Test pattern: $ARGUMENTS

1. If a pattern is provided, run `go test -v ./$1/...`
2. If no pattern, run `go test ./...`
3. If tests fail:
   - Analyze the failure output
   - Identify the root cause
   - Suggest specific fixes
