package media

import (
	"image"
	"image/color"
	"image/png"
	"bytes"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

// testPNGBytes creates a minimal valid PNG image.
func testPNGBytes() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name   string
		data   []byte
		expect Format
	}{
		{"PNG magic bytes", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, FormatPNG},
		{"JPEG magic bytes", []byte{0xFF, 0xD8, 0xFF, 0xE0}, FormatJPEG},
		{"WebP magic bytes", []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50}, FormatWebP},
		{"unknown bytes", []byte{0x00, 0x01, 0x02}, FormatPNG},
		{"empty", []byte{}, FormatPNG},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, DetectFormat(tt.data))
		})
	}
}

func TestFormat_MIMEType(t *testing.T) {
	assert.Equal(t, "image/png", FormatPNG.MIMEType())
	assert.Equal(t, "image/jpeg", FormatJPEG.MIMEType())
	assert.Equal(t, "image/webp", FormatWebP.MIMEType())
	assert.Equal(t, "image/png", Format("unknown").MIMEType())
}

func TestFormat_FileExtension(t *testing.T) {
	assert.Equal(t, ".png", FormatPNG.FileExtension())
	assert.Equal(t, ".jpg", FormatJPEG.FileExtension())
	assert.Equal(t, ".webp", FormatWebP.FileExtension())
}

func TestValidateFormat(t *testing.T) {
	assert.NoError(t, ValidateFormat(FormatPNG))
	assert.NoError(t, ValidateFormat(FormatJPEG))
	assert.NoError(t, ValidateFormat(FormatWebP))
	assert.NoError(t, ValidateFormat(""))
	assert.Error(t, ValidateFormat("bmp"))
}

func TestFormatFromMIME(t *testing.T) {
	assert.Equal(t, FormatPNG, FormatFromMIME("image/png"))
	assert.Equal(t, FormatJPEG, FormatFromMIME("image/jpeg"))
	assert.Equal(t, FormatJPEG, FormatFromMIME("image/jpg"))
	assert.Equal(t, FormatWebP, FormatFromMIME("image/webp"))
	assert.Equal(t, FormatPNG, FormatFromMIME("unknown"))
}

func TestConvertImage_PNGToJPEG(t *testing.T) {
	pngData := testPNGBytes()
	jpegData, err := ConvertImage(pngData, FormatJPEG)
	assert.NoError(t, err)
	assert.Equal(t, FormatJPEG, DetectFormat(jpegData))
}

func TestConvertImage_SameFormat(t *testing.T) {
	pngData := testPNGBytes()
	result, err := ConvertImage(pngData, FormatPNG)
	assert.NoError(t, err)
	assert.Equal(t, pngData, result)
}

func TestConvertImage_WebPTargetError(t *testing.T) {
	pngData := testPNGBytes()
	_, err := ConvertImage(pngData, FormatWebP)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "webp encoding is not supported")
}

func TestDetectMIMEFromBytes(t *testing.T) {
	assert.Equal(t, "image/png", DetectMIMEFromBytes([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}))
	assert.Equal(t, "image/jpeg", DetectMIMEFromBytes([]byte{0xFF, 0xD8, 0xFF}))
}
