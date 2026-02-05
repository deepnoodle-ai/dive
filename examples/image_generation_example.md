# Image Generation and Editing Examples

This document demonstrates how to use the Dive CLI's image generation and editing capabilities.

## Prerequisites

Set your OpenAI API key:

```bash
export OPENAI_API_KEY="your-api-key-here"
```

## Image Generation

### Basic Image Generation

Generate a simple image and save to file:

```bash
dive image generate --prompt "A beautiful sunset over mountains" --output sunset.png
```

### Generate with Specific Size and Quality

```bash
dive image generate \
  --prompt "A futuristic city with flying cars" \
  --size 1536x1024 \
  --quality high \
  --model gpt-image-1 \
  --output futuristic_city.png
```

### Generate Multiple Images

```bash
dive image generate \
  --prompt "Abstract geometric patterns" \
  --count 3 \
  --model gpt-image-1 \
  --output pattern.png
```

This will create `pattern_1.png`, `pattern_2.png`, and `pattern_3.png`.

### Output to Stdout (for piping)

```bash
dive image generate \
  --prompt "A small icon of a house" \
  --size 256x256 \
  --stdout > house_icon.base64
```

### Pipe to Other Tools

```bash
# Generate image and immediately convert to different format
dive image generate \
  --prompt "Logo design" \
  --stdout | base64 -d > logo.png
```

## Image Editing

### Basic Image Editing

Edit an existing image:

```bash
dive image edit \
  --input original.png \
  --prompt "Add a rainbow in the sky" \
  --output edited.png
```

### Edit with Mask

Use a mask to specify which parts of the image to edit:

```bash
dive image edit \
  --input photo.png \
  --prompt "Replace the background with a forest" \
  --mask background_mask.png \
  --output photo_with_forest.png
```

### Edit with Different Output Size

```bash
dive image edit \
  --input large_image.png \
  --prompt "Make the colors more vibrant" \
  --size 512x512 \
  --output vibrant_small.png
```

### Pipe Input from Previous Command

```bash
# Generate an image and immediately edit it
dive image generate \
  --prompt "A simple landscape" \
  --stdout | dive image edit \
  --prompt "Add snow to the landscape" \
  --output snowy_landscape.png
```

### Output to Stdout for Further Processing

```bash
dive image edit \
  --input input.png \
  --prompt "Convert to black and white style" \
  --stdout > bw_image.base64
```

## Supported Models and Providers

### Image Generation

- **DALL-E 2** (`dall-e-2`): Classic DALL-E model, supports sizes 256x256, 512x512, 1024x1024
- **DALL-E 3** (`dall-e-3`): Latest DALL-E model, supports 1024x1024, 1536x1024, 1024x1536
- **GPT Image 1** (`gpt-image-1`): OpenAI's newest image model with advanced capabilities

### Image Editing

- **DALL-E 2** (`dall-e-2`): Currently the only model that supports image editing

### Quality Settings

- **For gpt-image-1**: `high`, `medium`, `low`, `auto`
- **For dall-e-3**: `standard`, `hd`

## Tips

1. **File Formats**: The CLI automatically handles PNG, JPEG, and other common image formats
2. **Base64 Output**: Use `--stdout` to get base64 output for piping to other tools
3. **Batch Processing**: Use shell scripts to process multiple images
4. **Masks**: For image editing, masks should be PNG files with transparent areas indicating regions to edit
5. **API Limits**: Be aware of OpenAI API rate limits and costs when generating multiple images

## Troubleshooting

### Common Errors

- **"OPENAI_API_KEY environment variable is required"**: Set your OpenAI API key
- **"prompt is required"**: Always provide a prompt for both generation and editing
- **"error opening input image"**: Ensure the input file path is correct and the file exists
- **"invalid model"**: Check that you're using supported models for the operation

### File Size Limits

- Input images must be less than 4MB
- Mask images must be PNG format and have the same dimensions as the input image
