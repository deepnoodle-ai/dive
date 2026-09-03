package meta

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"mime"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/wonton/assert"
)

// wavHeader builds a minimal RIFF/WAVE header with no samples after it, which
// is all validateTranscriptionWAV reads.
func wavHeader(format, channels uint16, sampleRate uint32, bitsPerSample uint16) []byte {
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))
	binary.Write(&buf, binary.LittleEndian, format)
	binary.Write(&buf, binary.LittleEndian, channels)
	binary.Write(&buf, binary.LittleEndian, sampleRate)
	binary.Write(&buf, binary.LittleEndian, sampleRate*uint32(channels)*uint32(bitsPerSample)/8)
	binary.Write(&buf, binary.LittleEndian, channels*bitsPerSample/8)
	binary.Write(&buf, binary.LittleEndian, bitsPerSample)
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	return buf.Bytes()
}

// wavExtensibleHeader builds a WAVE_FORMAT_EXTENSIBLE header whose subformat
// GUID names the given format code. ffmpeg emits this shape for some inputs, so
// a valid extensible-PCM file has to pass and an extensible float file has to
// be caught here rather than at the API.
func wavExtensibleHeader(subFormat uint16, channels uint16, sampleRate uint32, bitsPerSample uint16) []byte {
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(60))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(40))
	binary.Write(&buf, binary.LittleEndian, uint16(0xFFFE))
	binary.Write(&buf, binary.LittleEndian, channels)
	binary.Write(&buf, binary.LittleEndian, sampleRate)
	binary.Write(&buf, binary.LittleEndian, sampleRate*uint32(channels)*uint32(bitsPerSample)/8)
	binary.Write(&buf, binary.LittleEndian, channels*bitsPerSample/8)
	binary.Write(&buf, binary.LittleEndian, bitsPerSample)
	binary.Write(&buf, binary.LittleEndian, uint16(22)) // cbSize
	binary.Write(&buf, binary.LittleEndian, bitsPerSample)
	binary.Write(&buf, binary.LittleEndian, uint32(0x4)) // channel mask
	binary.Write(&buf, binary.LittleEndian, subFormat)
	buf.Write([]byte{0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x80, 0x00,
		0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71}) // KSDATAFORMAT GUID suffix
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	return buf.Bytes()
}

// Meta answers every unsupported input with a bare 400, so these checks are
// the only place the caller learns which part of the format was wrong.
func TestValidateTranscriptionWAV(t *testing.T) {
	tests := []struct {
		name  string
		audio []byte
		want  string
	}{
		{"24kHz mono PCM", wavHeader(1, 1, 24000, 16), ""},
		{"16kHz mono PCM", wavHeader(1, 1, 16000, 16), ""},
		{"extensible PCM", wavExtensibleHeader(1, 1, 24000, 16), ""},
		{"extensible float", wavExtensibleHeader(3, 1, 24000, 16), "subformat 3"},
		{"extensible without its extension", wavHeader(0xFFFE, 1, 24000, 16), "too short to hold the subformat"},
		{"stereo", wavHeader(1, 2, 24000, 16), "2 channels"},
		{"44.1kHz", wavHeader(1, 1, 44100, 16), "44100 Hz"},
		{"24-bit", wavHeader(1, 1, 24000, 24), "24-bit"},
		{"float samples", wavHeader(3, 1, 24000, 16), "format 3"},
		{"mp3 bytes", []byte("ID3\x04\x00\x00\x00\x00\x00\x00 padding to forty four bytes!!"), "not a RIFF/WAVE"},
		{"truncated", []byte("RIFF"), "not a RIFF/WAVE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTranscriptionWAV(tt.audio)
			if tt.want == "" {
				assert.NoError(t, err)
				return
			}
			assert.Error(t, err)
			assert.True(t, strings.Contains(err.Error(), tt.want),
				"error %q does not name %q", err.Error(), tt.want)
			// Every rejection has to carry the fix, not just the diagnosis.
			assert.True(t, strings.Contains(err.Error(), "ffmpeg"),
				"error %q does not say how to convert", err.Error())
		})
	}
}

// A WAV written with a LIST or JUNK chunk ahead of fmt is still a valid WAV, so
// the fmt chunk is walked to rather than read at a fixed offset.
func TestValidateTranscriptionWAVSkipsLeadingChunks(t *testing.T) {
	header := wavHeader(1, 1, 24000, 16)
	var buf bytes.Buffer
	buf.Write(header[:12])
	buf.WriteString("JUNK")
	binary.Write(&buf, binary.LittleEndian, uint32(5))
	buf.WriteString("hello")
	buf.WriteByte(0) // pad to a word boundary
	buf.Write(header[12:])
	assert.NoError(t, validateTranscriptionWAV(buf.Bytes()))
}

// The request part has to be typed application/json; Meta rejects it as a
// plain form field.
func TestBuildTranscribeBody(t *testing.T) {
	request := transcribeRequest{
		Model:         ModelMuseVoiceTranscribe,
		AudioEncoding: "WAV",
		Mode:          string(TranscriptionModeDiarization),
		Keywords:      []string{"eSIM"},
		LanguageBias:  []string{"English"},
	}
	body, contentType, err := buildTranscribeBody(request, []byte("audio-bytes"))
	assert.NoError(t, err)

	mediaType, params, err := mime.ParseMediaType(contentType)
	assert.NoError(t, err)
	assert.Equal(t, mediaType, "multipart/form-data")

	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])

	requestPart, err := reader.NextPart()
	assert.NoError(t, err)
	assert.Equal(t, requestPart.FormName(), "request")
	assert.Equal(t, requestPart.Header.Get("Content-Type"), "application/json")
	var decoded transcribeRequest
	assert.NoError(t, json.NewDecoder(requestPart).Decode(&decoded))
	assert.Equal(t, decoded, request)

	audioPart, err := reader.NextPart()
	assert.NoError(t, err)
	assert.Equal(t, audioPart.FormName(), "audio")
	assert.Equal(t, audioPart.FileName(), "audio.wav")
}

// Every other provider Dive speaks to takes an ISO code here; Meta takes a
// language name, and answers a code with worse recognition rather than an error.
func TestLanguageNameMapping(t *testing.T) {
	assert.Equal(t, museLanguageName("en"), "English")
	assert.Equal(t, museLanguageName("en-US"), "English")
	assert.Equal(t, museLanguageName("pt_BR"), "Portuguese")
	assert.Equal(t, museLanguageName("ZH"), "Mandarin Chinese")
	// Already a name, or one Meta adds later: passed through untouched.
	assert.Equal(t, museLanguageName("English"), "English")
	assert.Equal(t, museLanguageName("Brazilian Portuguese"), "Brazilian Portuguese")
}

// Silently dropping the prompt would change the transcript the caller expected
// with nothing to show for it.
func TestTranscribeRejectsFreeFormPrompt(t *testing.T) {
	provider := NewMediaProvider(WithAPIKey("test"))
	_, err := provider.Transcribe(context.Background(), wavHeader(1, 1, 24000, 16), &media.Config{
		TranscriptionPrompt: "a call about eSIM activation",
	})
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "WithTranscriptionKeywords"),
		"error %q does not name the option to use instead", err.Error())
}

func TestTranscriptionRegistryRoutes(t *testing.T) {
	provider, err := media.DefaultRegistry().ResolveTranscription(ModelMuseVoiceTranscribe)
	assert.NoError(t, err)
	_, ok := provider.(*MediaProvider)
	assert.True(t, ok, "routed to %T, not the Meta provider", provider)
}
