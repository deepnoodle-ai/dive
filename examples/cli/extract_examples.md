# Dive Extract Examples

The `dive extract` command allows you to extract structured data from various file types using JSON schemas. This feature is powered by AI and can handle text files, images, and PDFs.

## Basic Usage

```bash
dive extract --schema schema.json --input document.txt --output extracted.json
```

## Examples

### 1. Extracting Person Information

**Schema** (`examples/schemas/person.json`):
```json
{
  "type": "object",
  "properties": {
    "name": {"type": "string", "description": "Full name"},
    "age": {"type": "integer", "description": "Age in years"},
    "email": {"type": "string", "format": "email"},
    "phone": {"type": "string"},
    "occupation": {"type": "string"}
  },
  "required": ["name"]
}
```

**Command**:
```bash
dive extract --schema examples/schemas/person.json --input examples/inputs/contact_info.txt --output person_data.json
```

### 2. Financial Data Extraction

**Command**:
```bash
dive extract \
  --schema examples/schemas/financial_report.json \
  --input examples/inputs/sample_report.txt \
  --output financial_data.json \
  --instructions "focus on monetary values and convert currency symbols to numbers"
```

### 3. Entity Extraction with Bias Filtering

**Command**:
```bash
dive extract \
  --schema examples/schemas/entity.json \
  --input document.txt \
  --bias-filter "avoid gender-based assumptions about roles and professions" \
  --output entities.json
```

### 4. Image Analysis

```bash
dive extract \
  --schema examples/schemas/person.json \
  --input photo.jpg \
  --bias-filter "extract information without making assumptions based on appearance"
```

### 5. PDF Processing

```bash
dive extract \
  --schema examples/schemas/financial_report.json \
  --input annual_report.pdf \
  --output report_data.json
```

## Schema Design Tips

1. **Be Specific**: Provide clear descriptions for each field to guide the extraction
2. **Use Appropriate Types**: Use `string`, `number`, `integer`, `boolean`, `array`, `object` as needed
3. **Mark Required Fields**: Use the `required` array to specify mandatory fields
4. **Add Constraints**: Use `minimum`, `maximum`, `minLength`, `maxLength`, `pattern`, `format` for validation
5. **Nested Objects**: Use nested objects for complex data structures

## Bias Filtering

The `--bias-filter` flag helps ensure fair and unbiased extraction:

- **Gender Bias**: "avoid gender-based assumptions about professions or roles"
- **Age Bias**: "do not make assumptions about capabilities based on age"
- **Cultural Bias**: "avoid cultural stereotypes or assumptions"
- **Appearance Bias**: "extract information without making judgments based on appearance"

## Advanced Features

### Custom Instructions

Use the `--instructions` flag for specific extraction guidance:

```bash
dive extract \
  --schema schema.json \
  --input document.txt \
  --instructions "prioritize financial data and convert all monetary values to USD"
```

### File Type Support

- **Text Files**: `.txt`, `.md`, `.csv`, `.json`, `.xml`, `.html`
- **Images**: `.jpg`, `.jpeg`, `.png`, `.gif`, `.bmp`, `.webp`
- **PDFs**: `.pdf`
- **Other**: The tool will attempt to process any file as text if type detection fails

### Output Options

- **Console Output**: If `--output` is not specified, results are displayed in the terminal
- **File Output**: Use `--output` to save results to a JSON file
- **Pretty Formatting**: Output is automatically formatted for readability

## Error Handling

The extract command provides detailed error messages for common issues:

- Missing or invalid schema files
- File not found or access denied
- Files that are too large (default limit: 10MB)
- Invalid JSON schemas
- Extraction failures

## Integration with Workflows

The extract tool can also be used within Dive workflows:

```yaml
name: document_processing
steps:
  - name: extract_data
    agent: data_extractor
    tools:
      - name: extract
        parameters:
          max_file_size: 5242880  # 5MB
```

## Tips for Best Results

1. **Clear Schemas**: Design schemas that clearly describe the expected data structure
2. **Appropriate Models**: Use models with good reasoning capabilities for complex extractions
3. **Bias Awareness**: Always consider potential biases and use filtering when appropriate
4. **Iterative Refinement**: Test with sample data and refine schemas as needed
5. **File Preparation**: Ensure input files are clear and well-formatted when possible