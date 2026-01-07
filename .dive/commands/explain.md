---
description: Explain a file or code section
argument-hint: "[file-path]"
allowed-tools:
  - Read
  - Grep
  - Glob
---

Explain the code in: $1

1. Read the specified file
2. Provide a clear explanation of:
   - The file's purpose and role in the codebase
   - Key data structures and types
   - Important functions and their responsibilities
   - How it interacts with other parts of the system

3. Use simple language and concrete examples where helpful
