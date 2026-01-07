---
description: Create a conventional commit
allowed-tools:
  - Bash
  - Read
  - Grep
---

Create a git commit for the current staged changes.

1. Run `git status` to see what's staged
2. Run `git diff --staged` to review the actual changes
3. Generate a commit message following conventional commits:
   - feat: new features
   - fix: bug fixes
   - docs: documentation changes
   - refactor: code refactoring
   - test: adding or updating tests
   - chore: maintenance tasks

4. The first line should be concise (50 chars max)
5. Add a blank line and detailed body if needed
6. Create the commit with `git commit -m "..."`
