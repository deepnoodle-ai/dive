#!/bin/bash

# Example usage of the dive classify command
# This demonstrates how to use the classification feature for data filtering and processing

echo "=== Dive Classify Command Examples ==="
echo

# Example 1: Sentiment Analysis
echo "1. Sentiment Analysis:"
echo "Command: dive classify --text 'This movie was absolutely fantastic!' --labels 'positive,negative,neutral'"
echo

# Example 2: Priority Classification with JSON output for scripting
echo "2. Priority Classification (JSON output for scripts):"
echo "Command: dive classify --text 'Critical system failure needs immediate attention' --labels 'urgent,normal,low' --json"
echo

# Example 3: Content Category Classification
echo "3. Content Category Classification:"
echo "Command: dive classify --text 'Learn Python programming basics' --labels 'education,entertainment,news,technology'"
echo

# Example 4: Using specific model
echo "4. Using specific model:"
echo "Command: dive classify --text 'Breaking news: Major scientific discovery' --labels 'urgent,normal,low' --model 'claude-3-5-sonnet-20241022'"
echo

# Example 5: Script integration example
echo "5. Script Integration Example:"
echo "This shows how to use the classify command in a bash script for data processing:"
echo
cat << 'EOF'
#!/bin/bash

# Process a list of texts and filter by classification
texts=(
    "URGENT: Server down!"
    "Meeting scheduled for tomorrow"
    "System maintenance complete"
    "CRITICAL: Security breach detected"
)

for text in "${texts[@]}"; do
    result=$(dive classify --text "$text" --labels "urgent,normal,low" --json)
    confidence=$(echo "$result" | jq -r '.top_classification.confidence')
    label=$(echo "$result" | jq -r '.top_classification.label')
    
    # Process only urgent items with high confidence
    if [[ "$label" == "urgent" ]] && (( $(echo "$confidence > 0.7" | bc -l) )); then
        echo "HIGH PRIORITY: $text"
        # Add to urgent queue, send notifications, etc.
    fi
done
EOF
echo

echo "=== Setup Instructions ==="
echo "1. Set your API key: export ANTHROPIC_API_KEY='your-key-here'"
echo "2. Or use another provider: --provider openai (with OPENAI_API_KEY)"
echo "3. For local testing: --provider ollama (requires ollama running locally)"
echo
echo "=== Output Formats ==="
echo "• Default: Human-readable with colors and confidence percentages"
echo "• --json: Structured JSON output for script integration"
echo "• Includes confidence scores (0.0 to 1.0) and reasoning for each classification"