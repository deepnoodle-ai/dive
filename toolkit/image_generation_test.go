package toolkit

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/wonton/assert"
)

// fakeImages maps a model name to the image the fake provider returns for it.
var fakeImages sync.Map

type fakeImageProvider struct{ model string }

func (p fakeImageProvider) GenerateImage(ctx context.Context, prompt string, config *media.Config) ([]*media.ImageResult, error) {
	v, _ := fakeImages.Load(p.model)
	return []*media.ImageResult{v.(*media.ImageResult)}, nil
}

func init() {
	media.RegisterImage(media.ImageProviderEntry{
		Name:    "toolkit-fake",
		Match:   media.PrefixMatcher("toolkit-fake-"),
		Factory: func(model string) media.ImageProvider { return fakeImageProvider{model: model} },
	})
}

// callFakeImageTool runs the tool against a fake model that returns result.
func callFakeImageTool(t *testing.T, result *media.ImageResult, input *ImageGenerationInput) (*dive.ToolResult, string) {
	t.Helper()
	model := "toolkit-fake-" + t.Name()
	fakeImages.Store(model, result)
	t.Cleanup(func() { fakeImages.Delete(model) })
	dir := t.TempDir()
	out, err := NewImageGenerationTool(model, WithImageToolWorkDir(dir)).Unwrap().(*imageGenerationTool).Call(context.Background(), input)
	assert.NoError(t, err)
	return out, dir
}

func encodePNG(t *testing.T, img image.Image, level png.CompressionLevel) []byte {
	t.Helper()
	var buf bytes.Buffer
	assert.NoError(t, (&png.Encoder{CompressionLevel: level}).Encode(&buf, img))
	return buf.Bytes()
}

func TestImageGenerationTool_ReturnsImageAndSavesFile(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	data := encodePNG(t, img, png.DefaultCompression)
	result, dir := callFakeImageTool(t, &media.ImageResult{
		Data: data, Format: media.FormatPNG, MimeType: "image/png", Width: 4, Height: 2,
	}, &ImageGenerationInput{Prompt: "a tiny square", OutputPath: "out.png"})

	assert.False(t, result.IsError)
	assert.Equal(t, 2, len(result.Content))

	text := result.Content[0]
	assert.Equal(t, dive.ToolResultContentTypeText, text.Type)
	assert.Contains(t, text.Text, "out.png")
	assert.Contains(t, text.Text, "(4x2 png)")
	assert.Equal(t, text.Text, result.Display)

	block := result.Content[1]
	assert.Equal(t, dive.ToolResultContentTypeImage, block.Type)
	assert.Equal(t, "image/png", block.MimeType)
	assert.Equal(t, base64.StdEncoding.EncodeToString(data), block.Data)

	saved, err := os.ReadFile(filepath.Join(dir, "out.png"))
	assert.NoError(t, err)
	assert.Equal(t, data, saved)
}

func TestImageGenerationTool_ImageMimeTypeFollowsFormat(t *testing.T) {
	var buf bytes.Buffer
	assert.NoError(t, jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil))
	// No MimeType from the provider: it is derived from the format.
	result, _ := callFakeImageTool(t, &media.ImageResult{
		Data: buf.Bytes(), Format: media.FormatJPEG, Width: 2, Height: 2,
	}, &ImageGenerationInput{Prompt: "a photo", Format: "jpeg"})

	assert.Equal(t, 2, len(result.Content))
	assert.Equal(t, "image/jpeg", result.Content[1].MimeType)
	assert.Contains(t, result.Content[0].Text, ".jpg")
}

// An image over the inline limit is sent as a downscaled JPEG preview, while
// the original is what lands on disk.
func TestImageGenerationTool_OversizedImageIsDownscaled(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 3000, 2000))
	for i := range img.Pix {
		img.Pix[i] = 128
	}
	data := encodePNG(t, img, png.NoCompression)
	assert.True(t, base64.StdEncoding.EncodedLen(len(data)) > maxInlineImageBase64)

	result, dir := callFakeImageTool(t, &media.ImageResult{
		Data: data, Format: media.FormatPNG, MimeType: "image/png", Width: 3000, Height: 2000,
	}, &ImageGenerationInput{Prompt: "a big grey field", OutputPath: "big.png"})

	assert.Equal(t, 2, len(result.Content))
	assert.Contains(t, result.Content[0].Text, "1568x1045 JPEG preview")
	assert.Equal(t, "Generated image: "+filepath.Join(mustEvalSymlinks(t, dir), "big.png")+" (3000x2000 png)", result.Display)
	assert.Equal(t, "image/jpeg", result.Content[1].MimeType)

	preview, err := base64.StdEncoding.DecodeString(result.Content[1].Data)
	assert.NoError(t, err)
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(preview))
	assert.NoError(t, err)
	assert.Equal(t, 1568, cfg.Width)
	assert.Equal(t, 1045, cfg.Height)

	saved, err := os.ReadFile(filepath.Join(dir, "big.png"))
	assert.NoError(t, err)
	assert.Equal(t, len(data), len(saved))
}

// An oversized image that cannot be decoded is left out, and the text says so.
func TestImageGenerationTool_OversizedUndecodableImageIsOmitted(t *testing.T) {
	data := bytes.Repeat([]byte{0}, maxInlineImageBase64)
	result, _ := callFakeImageTool(t, &media.ImageResult{
		Data: data, Format: media.FormatPNG, Width: 10, Height: 10,
	}, &ImageGenerationInput{Prompt: "junk"})

	assert.Equal(t, 1, len(result.Content))
	assert.Equal(t, dive.ToolResultContentTypeText, result.Content[0].Type)
	assert.Contains(t, result.Content[0].Text, "too large to show inline")
	assert.False(t, strings.Contains(result.Display, "too large"))
}

// pngWithDimensions encodes a tiny PNG and rewrites its IHDR to declare
// width x height, fixing the chunk CRC so DecodeConfig accepts it.
func pngWithDimensions(t *testing.T, width, height uint32) []byte {
	t.Helper()
	data := encodePNG(t, image.NewGray(image.Rect(0, 0, 1, 1)), png.DefaultCompression)
	// Signature (8), IHDR length (4), "IHDR" (4), then width and height.
	binary.BigEndian.PutUint32(data[16:20], width)
	binary.BigEndian.PutUint32(data[20:24], height)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	return data
}

// An image declaring huge dimensions is rejected from its header, before a
// full decode would allocate width*height pixels.
func TestDownscaleJPEG_RejectsHugeDimensions(t *testing.T) {
	data := pngWithDimensions(t, 30000, 30000)
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	assert.NoError(t, err)
	assert.Equal(t, 30000, cfg.Width)

	_, _, _, err = downscaleJPEG(data, inlineImageMaxEdge)
	assert.True(t, errors.Is(err, errImageTooLarge))
}

// An oversized result declaring huge dimensions is left out, and the text says so.
func TestImageGenerationTool_OversizedHugeDimensionsImageIsOmitted(t *testing.T) {
	// Pad past the inline limit so the downscale path runs.
	data := append(pngWithDimensions(t, 30000, 30000), make([]byte, maxInlineImageBase64)...)
	result, _ := callFakeImageTool(t, &media.ImageResult{
		Data: data, Format: media.FormatPNG, Width: 30000, Height: 30000,
	}, &ImageGenerationInput{Prompt: "a vast plain"})

	assert.Equal(t, 1, len(result.Content))
	assert.Contains(t, result.Content[0].Text, "too large to show inline")
}

func mustEvalSymlinks(t *testing.T, path string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(path)
	assert.NoError(t, err)
	return real
}

func TestImageGenerationTool_Name(t *testing.T) {
	tool := NewImageGenerationTool("test-model")
	assert.Equal(t, "ImageGeneration", tool.Name())
}

func TestImageGenerationTool_Description(t *testing.T) {
	tool := NewImageGenerationTool("imagen-4")
	desc := tool.Description()
	assert.Contains(t, desc, "imagen-4")
	assert.Contains(t, desc, "image")
	assert.Contains(t, desc, "returns both the file path and the image")
}

func TestImageGenerationTool_Annotations(t *testing.T) {
	tool := NewImageGenerationTool("test-model")
	ann := tool.Annotations()
	assert.Equal(t, "ImageGeneration", ann.Title)
	assert.True(t, !ann.ReadOnlyHint)
	assert.True(t, !ann.DestructiveHint)
	assert.True(t, ann.OpenWorldHint)
}

func TestImageGenerationTool_Schema(t *testing.T) {
	tool := NewImageGenerationTool("test-model")
	s := tool.Schema()
	assert.Equal(t, "object", string(s.Type))
	assert.Contains(t, s.Required, "prompt")
	assert.NotNil(t, s.Properties["prompt"])
	assert.NotNil(t, s.Properties["aspect_ratio"])
	assert.NotNil(t, s.Properties["output_path"])
	assert.NotNil(t, s.Properties["format"])

	// Verify only supported aspect ratios are advertised (no 4:3 or 3:4)
	arEnum := s.Properties["aspect_ratio"].Enum
	assert.Equal(t, 3, len(arEnum))
	assert.Contains(t, arEnum, any("1:1"))
	assert.Contains(t, arEnum, any("16:9"))
	assert.Contains(t, arEnum, any("9:16"))
}

func TestImageGenerationTool_Schema_NoDurationEnum(t *testing.T) {
	// Verify video tool duration has no enum (provider-specific)
	tool := NewVideoGenerationTool("test-model")
	s := tool.Schema()
	assert.Nil(t, s.Properties["duration"].Enum)
}

func TestImageGenerationTool_WorkDir(t *testing.T) {
	dir := t.TempDir()
	tool := NewImageGenerationTool("test-model", WithImageToolWorkDir(dir))
	inner := tool.Unwrap().(*imageGenerationTool)
	assert.Equal(t, dir, inner.workDir)
}

func TestValidateOutputPath(t *testing.T) {
	workDir := t.TempDir()
	// Resolve symlinks in workDir itself (e.g. /var -> /private/var on macOS)
	realWorkDir, err := filepath.EvalSymlinks(workDir)
	assert.Nil(t, err)

	// Valid relative path
	path, err := validateOutputPath("output.png", workDir)
	assert.Nil(t, err)
	assert.True(t, filepath.IsAbs(path))
	assert.Equal(t, filepath.Join(realWorkDir, "output.png"), path)

	// Valid nested relative path
	path, err = validateOutputPath(filepath.FromSlash("subdir/output.png"), workDir)
	assert.Nil(t, err)
	assert.True(t, filepath.IsAbs(path))
	assert.Equal(t, filepath.Join(realWorkDir, "subdir", "output.png"), path)

	// Absolute path rejected
	absPath := filepath.Join(workDir, "other", "file.png")
	_, err = validateOutputPath(absPath, workDir)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "relative path")

	// Traversal attack rejected
	_, err = validateOutputPath(filepath.FromSlash("../../etc/passwd"), workDir)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "within the working directory")

	// Traversal with nested path rejected
	_, err = validateOutputPath(filepath.FromSlash("subdir/../../etc/passwd"), workDir)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "within the working directory")
}

func TestValidateOutputPath_SymlinkTraversal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests require elevated privileges on Windows")
	}

	workDir := t.TempDir()
	outsideDir := t.TempDir()

	// Create a file outside the workDir
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	os.WriteFile(outsideFile, []byte("secret"), 0o644)

	// Create a symlink inside workDir pointing outside
	linkPath := filepath.Join(workDir, "escape")
	err := os.Symlink(outsideDir, linkPath)
	assert.Nil(t, err)

	// Attempting to write through the symlink should be rejected
	_, err = validateOutputPath(filepath.FromSlash("escape/secret.txt"), workDir)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "within the working directory")
}
