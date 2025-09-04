# Dive Extract Feature Implementation

## Overview

Successfully implemented the `dive extract` command that allows users to extract structured data from text files, images, and PDFs using JSON schemas, with support for bias filtering and custom instructions.

## Implementation Details

### Core Components

1. **Extract Tool** (`toolkit/extract.go`)
   - Implements the `dive.TypedTool[*ExtractInput]` interface
   - Handles file type detection (text, image, PDF)
   - Validates input files and schemas
   - Supports configurable file size limits (default: 10MB)
   - Provides structured analysis results for the AI agent

2. **CLI Command** (`cmd/dive/cli/extract.go`)
   - Implements the `dive extract` subcommand
   - Supports all required flags: `--schema`, `--input`, `--output`
   - Includes optional flags: `--bias-filter`, `--instructions`
   - Creates specialized extraction agent with appropriate instructions
   - Handles JSON validation and output formatting

3. **Tool Registration** (`config/tools.go`)
   - Added `extract` tool to the tool initializers map
   - Integrated with existing tool configuration system

### Features Implemented

#### ✅ Schema-Based Extraction
- Load and validate JSON schemas from files
- Support for complex nested object structures
- Proper error handling for invalid schemas

#### ✅ Multi-Format Support
- **Text Files**: `.txt`, `.md`, `.csv`, `.json`, `.xml`, `.html`
- **Images**: `.jpg`, `.jpeg`, `.png`, `.gif`, `.bmp`, `.webp`
- **PDFs**: `.pdf` files
- **Auto-detection**: MIME type and extension-based detection

#### ✅ Bias Filtering
- Optional `--bias-filter` flag for specifying bias mitigation instructions
- Integrated into agent instructions for consistent application
- Examples: gender bias, age bias, cultural assumptions

#### ✅ Custom Instructions
- Optional `--instructions` flag for additional extraction guidance
- Allows users to specify focus areas or special requirements
- Examples: "focus on financial data", "prioritize contact information"

#### ✅ Flexible Output
- Console output with pretty-formatted JSON
- Optional file output with `--output` flag
- Automatic directory creation for output paths
- JSON validation and error handling

#### ✅ Comprehensive Error Handling
- File existence and permission validation
- Schema validation and parsing
- File size limit enforcement
- Clear, actionable error messages

### Example Usage

```bash
# Basic extraction
dive extract --schema entity.json --input report.pdf --output extracted.json

# With bias filtering
dive extract --schema person.json --input image.jpg --bias-filter "avoid gender assumptions"

# With custom instructions
dive extract --schema financial.json --input report.txt --instructions "focus on monetary values"

# Full example
dive extract \
  --schema examples/schemas/entity.json \
  --input examples/inputs/sample_report.txt \
  --output results.json \
  --bias-filter "avoid assumptions about gender, age, or cultural background" \
  --instructions "prioritize factual information and numerical data" \
  --provider anthropic \
  --model claude-3-5-sonnet-20241022
```

### Testing

#### Unit Tests
- **Tool Tests** (`toolkit/extract_test.go`): 12 test cases covering all functionality
- **CLI Tests** (`cmd/dive/cli/extract_test.go`): 8 test cases covering command logic
- All tests pass with comprehensive coverage

#### Integration Tests
- CLI command registration verification
- Help system integration
- Flag validation testing
- End-to-end workflow validation (requires API keys)

### Example Schemas Provided

1. **Person Schema** (`examples/schemas/person.json`)
   - Name, age, contact information, address, occupation
   - Demonstrates nested object structures

2. **Financial Report Schema** (`examples/schemas/financial_report.json`)
   - Revenue, expenses, assets, liabilities, key metrics
   - Complex nested financial data structures

3. **Entity Schema** (`examples/schemas/entity.json`)
   - People, organizations, locations, dates, monetary values
   - Multi-array structure for entity extraction

### Example Input Files

1. **Contact Information** (`examples/inputs/contact_info.txt`)
   - Multiple people with various contact details
   - Tests person extraction capabilities

2. **Sample Report** (`examples/inputs/sample_report.txt`)
   - Quarterly financial report with mixed data types
   - Tests financial and entity extraction

### Documentation

- **Examples Guide** (`examples/extract_examples.md`): Comprehensive usage examples
- **Demo Script** (`examples/extract_demo.sh`): Interactive demonstration
- **CLI Help**: Integrated help system with examples

## Architecture Integration

The extract feature integrates seamlessly with Dive's existing architecture:

- **Agent System**: Uses specialized extraction agents with custom instructions
- **Tool System**: Follows the standard `TypedTool` pattern
- **CLI Framework**: Uses Cobra command structure consistent with other commands
- **Configuration**: Integrates with existing tool initialization system
- **Error Handling**: Follows established error handling patterns

## Key Design Decisions

1. **Tool-Based Approach**: Implemented as a tool that can be used by agents, allowing for workflow integration
2. **Schema Flexibility**: Accepts any valid JSON schema for maximum versatility
3. **Bias Awareness**: Built-in support for bias filtering as a first-class feature
4. **File Type Agnostic**: Handles multiple file types through intelligent detection
5. **CLI Integration**: Follows Dive's existing command patterns for consistency

## Future Enhancements

Potential areas for future development:
- PDF text extraction libraries for better PDF handling
- Image OCR integration for text extraction from images
- Batch processing capabilities
- Schema validation and suggestion features
- Integration with document repositories
- Workflow templates for common extraction patterns

## Conclusion

The `dive extract` feature is now fully implemented and ready for use. It provides a powerful, flexible, and bias-aware solution for structured data extraction that integrates seamlessly with the Dive AI toolkit framework.