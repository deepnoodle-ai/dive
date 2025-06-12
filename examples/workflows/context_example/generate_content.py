#!/usr/bin/env python3
import json
import os
from datetime import datetime

# Access any globals passed from the workflow execution
globals_env = os.getenv("DIVE_GLOBALS")
globals_data = {}
if globals_env:
    globals_data = json.loads(globals_env)

# Generate dynamic content
content = {
    "type": "text",
    "text": f"Generated at {datetime.now().isoformat()} by external Python script",
}

# Include any workflow variables in the output
if globals_data:
    content["text"] += f"\nWorkflow globals: {json.dumps(globals_data, indent=2)}"

# Output the content as JSON
print(json.dumps(content, indent=2))
