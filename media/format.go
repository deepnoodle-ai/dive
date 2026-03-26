package media

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"

	_ "golang.org/x/image/webp" // register WebP decoder
)

// Format represents an image output format.
type Format string

const (
	FormatPNG  Format = "png"
	FormatJPEG Format = "jpeg"
	FormatWebP Format = "webp"
)

// MIMEType returns the MIME type string for the format.
func (f Format) MIMEType() string {
	switch f {
	case FormatPNG:
		return "image/png"
	case FormatJPEG:
		return "image/jpeg"
	case FormatWebP:
		return "image/webp"
	default:
		return "image/png"
	}
}

// FileExtension returns the file extension (with dot) for the format.
func (f Format) FileExtension() string {
	switch f {
	case FormatPNG:
		return ".png"
	case FormatJPEG:
		return ".jpg"
	case FormatWebP:
		return ".webp"
	default:
		return ".png"
	}
}

// String returns the string representation.
func (f Format) String() string {
	return string(f)
}

// ValidateFormat returns an error if the format is not a recognized value.
func ValidateFormat(f Format) error {
	switch f {
	case FormatPNG, FormatJPEG, FormatWebP, "":
		return nil
	default:
		return fmt.Errorf("invalid format %q; must be png, jpeg, or webp", f)
	}
}

// FormatFromMIME returns the Format corresponding to a MIME type string.
func FormatFromMIME(mime string) Format {
	switch mime {
	case "image/png":
		return FormatPNG
	case "image/jpeg", "image/jpg":
		return FormatJPEG
	case "image/webp":
		return FormatWebP
	default:
		return FormatPNG
	}
}

// DetectFormat inspects magic bytes and returns the image format.
// Returns FormatPNG if the format cannot be determined.
func DetectFormat(data []byte) Format {
	if len(data) >= 8 && data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return FormatPNG
	}
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return FormatJPEG
	}
	if len(data) >= 12 && data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
		return FormatWebP
	}
	return FormatPNG
}

// DetectMIMEFromBytes returns the MIME type based on magic bytes.
func DetectMIMEFromBytes(data []byte) string {
	return DetectFormat(data).MIMEType()
}

// ConvertImage re-encodes image data to the target format.
// Supports PNG and JPEG targets. Returns an error for WebP target
// (no Go stdlib encoder). If source format matches target, returns
// data unchanged.
func ConvertImage(data []byte, target Format) ([]byte, error) {
	sourceFormat := DetectFormat(data)
	if sourceFormat == target {
		return data, nil
	}
	if target == FormatWebP {
		return nil, fmt.Errorf("webp encoding is not supported (no Go stdlib encoder)")
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decoding image: %w", err)
	}

	var buf bytes.Buffer
	switch target {
	case FormatPNG:
		if err := png.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("encoding png: %w", err)
		}
	case FormatJPEG:
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
			return nil, fmt.Errorf("encoding jpeg: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported target format: %s", target)
	}
	return buf.Bytes(), nil
}
