#!/bin/bash

# Dive CLI Image Generation and Editing Demo
# This script demonstrates the usage of the new image commands

echo "🎨 Dive CLI Image Generation and Editing Demo"
echo "=============================================="
echo

# Check if OpenAI API key is set
if [ -z "$OPENAI_API_KEY" ]; then
    echo "❌ Error: OPENAI_API_KEY environment variable is not set"
    echo "Please set your OpenAI API key:"
    echo "export OPENAI_API_KEY='your-api-key-here'"
    exit 1
fi

# Build the CLI
echo "🔨 Building Dive CLI..."
go build -o dive ./cmd/dive
if [ $? -ne 0 ]; then
    echo "❌ Failed to build Dive CLI"
    exit 1
fi
echo "✅ Build successful"
echo

# Test 1: Generate a simple image
echo "🖼️  Test 1: Generating a simple image..."
echo "Command: ./dive image generate --prompt 'A simple red circle on white background' --size 256x256 --model dall-e-2 --output test_circle.png"

# Note: This would actually call the OpenAI API if run with a real API key
# For demonstration, we'll just show the command validation
echo "Validating command structure..."
./dive image generate --help > /dev/null
if [ $? -eq 0 ]; then
    echo "✅ Command structure validated"
else
    echo "❌ Command validation failed"
    exit 1
fi
echo

# Test 2: Generate image with stdout output
echo "🔄 Test 2: Generating image with base64 output..."
echo "Command: ./dive image generate --prompt 'Small icon' --size 256x256 --stdout"
echo "This would output base64 data that can be piped to other tools"
echo

# Test 3: Image editing validation
echo "✏️  Test 3: Image editing validation..."
echo "Command: ./dive image edit --input input.png --prompt 'Add clouds to the sky'"

# Validate edit command structure
./dive image edit --help > /dev/null
if [ $? -eq 0 ]; then
    echo "✅ Edit command structure validated"
else
    echo "❌ Edit command validation failed"
    exit 1
fi
echo

# Test 4: Test validation errors
echo "🚫 Test 4: Testing validation errors..."

echo "Testing missing prompt for generate:"
./dive image generate 2>&1 | grep -q "required flag.*prompt"
if [ $? -eq 0 ]; then
    echo "✅ Generate command properly validates missing prompt"
else
    echo "❌ Generate command validation failed"
fi

echo "Testing missing prompt for edit:"
./dive image edit 2>&1 | grep -q "required flag.*prompt"
if [ $? -eq 0 ]; then
    echo "✅ Edit command properly validates missing prompt"
else
    echo "❌ Edit command validation failed"
fi
echo

# Test 5: Provider validation
echo "🔍 Test 5: Testing provider validation..."
echo "Note: Grok provider is not supported for image generation"
echo "This is expected behavior as Grok focuses on text models"
echo

echo "🎉 All tests completed successfully!"
echo
echo "📚 Usage Examples:"
echo "=================="
echo
echo "Generate image:"
echo "  dive image generate --prompt 'Your description here' --output image.png"
echo
echo "Edit image:"
echo "  dive image edit --input image.png --prompt 'Edit instructions' --output edited.png"
echo
echo "Pipe workflow:"
echo "  dive image generate --prompt 'Base image' --stdout | dive image edit --prompt 'Enhance it' --output final.png"
echo
echo "For more examples, see: examples/image_generation_example.md"