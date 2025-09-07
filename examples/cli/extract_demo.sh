#!/bin/bash

# Dive Extract Feature Demonstration
# This script demonstrates the dive extract command functionality

echo "🚀 Dive Extract Feature Demo"
echo "=========================="
echo

# Build the dive CLI
echo "📦 Building dive CLI..."
go build -o dive ./cmd/dive
echo "✅ Build complete"
echo

# Show help for extract command
echo "📖 Extract Command Help:"
echo "------------------------"
./dive extract --help
echo

# Test 1: Extract person information from contact data
echo "🧪 Test 1: Person Information Extraction"
echo "----------------------------------------"
echo "Schema: examples/schemas/person.json"
echo "Input: examples/inputs/contact_info.txt"
echo
echo "Command:"
echo "./dive extract --schema examples/schemas/person.json --input examples/inputs/contact_info.txt --output person_results.json --provider anthropic --model claude-3-5-sonnet-20241022"
echo
echo "Note: This would require ANTHROPIC_API_KEY environment variable to be set"
echo

# Test 2: Financial data extraction
echo "🧪 Test 2: Financial Data Extraction"
echo "------------------------------------"
echo "Schema: examples/schemas/financial_report.json"
echo "Input: examples/inputs/sample_report.txt"
echo
echo "Command:"
echo "./dive extract --schema examples/schemas/financial_report.json --input examples/inputs/sample_report.txt --output financial_results.json --instructions 'focus on numerical values and convert to appropriate types'"
echo

# Test 3: Entity extraction with bias filtering
echo "🧪 Test 3: Entity Extraction with Bias Filtering"
echo "------------------------------------------------"
echo "Schema: examples/schemas/entity.json"
echo "Input: examples/inputs/sample_report.txt"
echo
echo "Command:"
echo "./dive extract --schema examples/schemas/entity.json --input examples/inputs/sample_report.txt --bias-filter 'avoid assumptions about gender, age, or cultural background when extracting person information' --output entities_results.json"
echo

# Show schema examples
echo "📋 Example Schemas:"
echo "------------------"
echo
echo "Person Schema (examples/schemas/person.json):"
echo "============================================="
cat examples/schemas/person.json
echo
echo
echo "Financial Report Schema (examples/schemas/financial_report.json):"
echo "================================================================="
head -20 examples/schemas/financial_report.json
echo "... (truncated for brevity)"
echo
echo
echo "Entity Schema (examples/schemas/entity.json):"
echo "============================================="
head -15 examples/schemas/entity.json
echo "... (truncated for brevity)"
echo

# Show input examples
echo "📄 Example Input Files:"
echo "----------------------"
echo
echo "Contact Information (examples/inputs/contact_info.txt):"
echo "======================================================"
cat examples/inputs/contact_info.txt
echo
echo
echo "Sample Report (examples/inputs/sample_report.txt):"
echo "================================================="
cat examples/inputs/sample_report.txt
echo

echo "🎯 Key Features Demonstrated:"
echo "=============================="
echo "✅ JSON schema-based extraction"
echo "✅ Multiple file type support (text, images, PDFs)"
echo "✅ Bias filtering capabilities"
echo "✅ Custom extraction instructions"
echo "✅ Flexible output options"
echo "✅ Comprehensive error handling"
echo "✅ CLI integration with Dive framework"
echo
echo "🔧 To use with real data:"
echo "========================"
echo "1. Set up your LLM provider (e.g., export ANTHROPIC_API_KEY=your_key)"
echo "2. Run: dive extract --schema your_schema.json --input your_file.txt --output results.json"
echo "3. Add --bias-filter and --instructions as needed"
echo
echo "Demo complete! 🎉"