package meta

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"slices"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive/media"
	"github.com/openai/openai-go/v3/option"
)

var _ media.TranscriptionProvider = &MediaProvider{}

// TranscriptionMode selects how Muse Voice Transcribe segments the audio.
type TranscriptionMode string

const (
	// TranscriptionModePushToTalk transcribes the recording as a single turn.
	// It is Meta's default and leaves the response's turn list empty.
	TranscriptionModePushToTalk TranscriptionMode = "PUSH_TO_TALK"

	// TranscriptionModeEndpointing asks the model to find utterance
	// boundaries, returning one turn per detected speech segment.
	TranscriptionModeEndpointing TranscriptionMode = "ENDPOINTING"

	// TranscriptionModeDiarization adds speaker attribution on top of
	// endpointing. Labels such as "A" and "B" are scoped to one transcription:
	// the same label in a later request is not the same person.
	TranscriptionModeDiarization TranscriptionMode = "DIARIZATION"
)

// Turn is one detected speech segment. Meta returns turn-level timestamps
// only; word-level timestamps are not available from this model.
type Turn struct {
	TurnID     int    `json:"turnId"`
	StartMs    int64  `json:"startMs"`
	EndMs      int64  `json:"endMs"`
	Transcript string `json:"transcript"`
	Speaker    string `json:"speaker,omitempty"`
}

// transcribeResponse is the JSON body of POST /v1/asr/transcribe.
type transcribeResponse struct {
	SessionID       string `json:"sessionId"`
	Transcript      string `json:"transcript"`
	AudioDurationMs int64  `json:"audioDurationMs"`
	Turns           []Turn `json:"turns"`
}

// transcribeRequest is the JSON "request" part of the multipart upload.
type transcribeRequest struct {
	Model         string   `json:"model"`
	AudioEncoding string   `json:"audioEncoding"`
	Mode          string   `json:"mode,omitempty"`
	LanguageBias  []string `json:"languageBias,omitempty"`
	Keywords      []string `json:"keywords,omitempty"`
}

// WithTranscriptionMode selects push-to-talk, endpointing, or diarization.
func WithTranscriptionMode(mode TranscriptionMode) Option {
	return func(c *config) { c.transcriptionMode = mode }
}

// WithTranscriptionKeywords biases recognition toward product names, people,
// places, acronyms, and other terms the model might otherwise mishear. It is a
// list of terms, not free-form context, and it biases rather than guarantees a
// spelling.
func WithTranscriptionKeywords(keywords ...string) Option {
	return func(c *config) { c.transcriptionKeywords = append(c.transcriptionKeywords, keywords...) }
}

// WithLanguageBias names the languages to expect, steering recognition without
// forcing it. Pass Meta's language names ("English", "Brazilian Portuguese").
// media.Config.Language sets a single language and is mapped from an ISO code
// when it looks like one; this option is the way to name several at once.
func WithLanguageBias(languages ...string) Option {
	return func(c *config) { c.languageBias = append(c.languageBias, languages...) }
}

// Transcribe converts speech to text with Muse Voice Transcribe.
//
// This is the file endpoint (POST /v1/asr/transcribe), which takes a complete
// recording and returns a complete transcript. Meta's live path is a WebSocket
// at wss://api.meta.ai/v1/asr/realtime with its own event protocol, which
// media.TranscriptionProvider does not model; it is not wired up here.
func (p *MediaProvider) Transcribe(ctx context.Context, audio []byte, config *media.Config) (*media.TranscriptionResult, error) {
	if len(audio) == 0 {
		return nil, fmt.Errorf("audio data is required for transcription")
	}
	if err := validateTranscriptionWAV(audio); err != nil {
		return nil, err
	}
	if len(audio) > maxTranscribeBytes {
		return nil, fmt.Errorf("audio is %d bytes; Meta rejects a transcribe body over %d bytes, so split the recording into shorter clips",
			len(audio), maxTranscribeBytes)
	}

	model := config.Model
	if model == "" {
		model = ModelMuseVoiceTranscribe
	}

	// Muse Voice Transcribe has no free-form prompt field: context is given as
	// a keyword list instead. Dropping the prompt silently would quietly change
	// the transcript the caller expected, so say what to use in its place.
	if config.TranscriptionPrompt != "" {
		return nil, fmt.Errorf("%s takes no free-form transcription prompt; pass the terms to bias toward with meta.WithTranscriptionKeywords", model)
	}

	request := transcribeRequest{
		Model:         model,
		AudioEncoding: "WAV",
		Mode:          string(p.transcriptionMode),
		Keywords:      p.transcriptionKeywords,
		// Cloned because the per-request language is appended below: a
		// configured slice with spare capacity would otherwise be written
		// through by concurrent Transcribe calls sharing this provider.
		LanguageBias: slices.Clone(p.languageBias),
	}
	if config.Language != "" {
		request.LanguageBias = append(request.LanguageBias, museLanguageName(config.Language))
	}

	body, contentType, err := buildTranscribeBody(request, audio)
	if err != nil {
		return nil, err
	}
	// The audio check above is a cheap early out; this is the one that matches
	// what Meta measures, since the multipart envelope and the keyword and
	// language-bias fields all count against the same cap.
	if len(body) > maxTranscribeBytes {
		return nil, fmt.Errorf("request body is %d bytes; Meta rejects a transcribe body over %d bytes, so split the recording into shorter clips",
			len(body), maxTranscribeBytes)
	}

	var response transcribeResponse
	if err := p.client.Post(ctx, "asr/transcribe", nil, &response,
		option.WithRequestBody(contentType, body),
	); err != nil {
		return nil, fmt.Errorf("meta transcription: %w", err)
	}

	result := &media.TranscriptionResult{
		Text:     response.Transcript,
		Model:    model,
		Language: config.Language,
		Duration: time.Duration(response.AudioDurationMs) * time.Millisecond,
		Metadata: map[string]any{
			"provider":   "meta",
			"session_id": response.SessionID,
		},
	}
	if len(response.Turns) > 0 {
		result.Metadata["turns"] = response.Turns
	}
	return result, nil
}

// maxTranscribeBytes is the request body cap Meta answers with a 413. It
// applies to the whole multipart body, not the audio alone, so the audio is
// checked early and the assembled body is checked again before the POST.
const maxTranscribeBytes = 32 << 20

func buildTranscribeBody(request transcribeRequest, audio []byte) ([]byte, string, error) {
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return nil, "", fmt.Errorf("encoding transcription request: %w", err)
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// The request part has to be typed application/json; Meta rejects it as a
	// plain form field.
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="request"`)
	header.Set("Content-Type", "application/json")
	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, "", fmt.Errorf("writing transcription request part: %w", err)
	}
	if _, err := part.Write(requestJSON); err != nil {
		return nil, "", fmt.Errorf("writing transcription request part: %w", err)
	}

	audioHeader := make(textproto.MIMEHeader)
	audioHeader.Set("Content-Disposition", `form-data; name="audio"; filename="audio.wav"`)
	audioHeader.Set("Content-Type", "audio/wav")
	audioPart, err := writer.CreatePart(audioHeader)
	if err != nil {
		return nil, "", fmt.Errorf("writing transcription audio part: %w", err)
	}
	if _, err := audioPart.Write(audio); err != nil {
		return nil, "", fmt.Errorf("writing transcription audio part: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("closing transcription request body: %w", err)
	}
	return buf.Bytes(), writer.FormDataContentType(), nil
}

// convertHint is the ffmpeg invocation from Meta's docs, quoted in every
// rejection so the fix does not need a docs lookup.
const convertHint = "convert it with: ffmpeg -i input -ac 1 -ar 24000 -c:a pcm_s16le -map_metadata -1 output.wav"

// validateTranscriptionWAV rejects audio the endpoint cannot read. Meta accepts
// exactly one shape — RIFF/WAVE holding mono 16-bit PCM at 16 or 24 kHz — and
// answers anything else with a bare 400, so the header is worth reading here
// where the specific mismatch can be named.
func validateTranscriptionWAV(audio []byte) error {
	if len(audio) < 44 || !bytes.Equal(audio[0:4], []byte("RIFF")) || !bytes.Equal(audio[8:12], []byte("WAVE")) {
		return fmt.Errorf("audio is not a RIFF/WAVE file; Muse Voice Transcribe reads WAV only, so %s", convertHint)
	}

	chunk, err := wavFormat(audio)
	if err != nil {
		return err
	}
	format := binary.LittleEndian.Uint16(chunk[0:2])
	channels := binary.LittleEndian.Uint16(chunk[2:4])
	sampleRate := binary.LittleEndian.Uint32(chunk[4:8])
	bitsPerSample := binary.LittleEndian.Uint16(chunk[14:16])

	// 1 is WAVE_FORMAT_PCM. 0xFFFE is WAVE_FORMAT_EXTENSIBLE, which ffmpeg
	// writes for some inputs; the tag alone says nothing about the samples, so
	// the extension's subformat GUID is what decides whether they are PCM. An
	// extensible IEEE-float file otherwise passes every field check here and
	// earns the bare 400 this validator exists to replace.
	switch format {
	case 1:
	case 0xFFFE:
		if len(chunk) < extensibleFmtSize {
			return fmt.Errorf("WAV is extensible but its fmt chunk is %d bytes, too short to hold the subformat; %s", len(chunk), convertHint)
		}
		if subFormat := binary.LittleEndian.Uint16(chunk[24:26]); subFormat != 1 {
			return fmt.Errorf("WAV is extensible with subformat %d, not integer PCM; %s", subFormat, convertHint)
		}
	default:
		return fmt.Errorf("WAV holds format %d, not integer PCM; %s", format, convertHint)
	}
	if channels != 1 {
		return fmt.Errorf("WAV has %d channels; Muse Voice Transcribe takes mono, so %s", channels, convertHint)
	}
	if bitsPerSample != 16 {
		return fmt.Errorf("WAV is %d-bit; Muse Voice Transcribe takes 16-bit samples, so %s", bitsPerSample, convertHint)
	}
	if sampleRate != 16000 && sampleRate != 24000 {
		return fmt.Errorf("WAV is %d Hz; Muse Voice Transcribe takes 16000 or 24000 Hz, so %s", sampleRate, convertHint)
	}
	return nil
}

// extensibleFmtSize is the length of a WAVE_FORMAT_EXTENSIBLE fmt chunk: the
// 16-byte common header, a 2-byte cbSize, and the 22-byte extension whose last
// 16 bytes are the subformat GUID.
const extensibleFmtSize = 40

// wavFormat returns the fmt chunk. Chunks are walked rather than read at a
// fixed offset because a WAV may carry LIST or JUNK chunks before fmt.
func wavFormat(audio []byte) ([]byte, error) {
	for offset := 12; offset+8 <= len(audio); {
		id := audio[offset : offset+4]
		size := int(binary.LittleEndian.Uint32(audio[offset+4 : offset+8]))
		payload := offset + 8
		if size < 0 || payload+size > len(audio) {
			break
		}
		if string(id) == "fmt " {
			if size < 16 {
				return nil, fmt.Errorf("WAV fmt chunk is %d bytes, too short to read; %s", size, convertHint)
			}
			return audio[payload : payload+size], nil
		}
		// Chunks are word-aligned: an odd size is followed by a pad byte.
		offset = payload + size + size%2
	}
	return nil, fmt.Errorf("WAV has no fmt chunk; %s", convertHint)
}

// museLanguages maps ISO 639-1 codes to the language names Meta's languageBias
// expects, covering the 25 languages Muse Voice Transcribe supports. A value
// that is not a code in this table is passed through untouched, so a language
// name — or one Meta adds later — still reaches the API.
var museLanguages = map[string]string{
	"ar": "Arabic",
	"bn": "Bengali",
	"de": "German",
	"en": "English",
	"es": "Spanish",
	"fr": "French",
	"he": "Hebrew",
	"hi": "Hindi",
	"id": "Indonesian",
	"it": "Italian",
	"ja": "Japanese",
	"kn": "Kannada",
	"ko": "Korean",
	"mr": "Marathi",
	"ms": "Malay",
	"nl": "Dutch",
	"pl": "Polish",
	"pt": "Portuguese",
	"ta": "Tamil",
	"te": "Telugu",
	"th": "Thai",
	"tl": "Tagalog",
	"tr": "Turkish",
	"vi": "Vietnamese",
	"zh": "Mandarin Chinese",
}

func museLanguageName(language string) string {
	// "en-US" and "en_US" both name English to every other provider Dive
	// speaks to, so they have to reach Meta as a language it recognizes.
	base := strings.ToLower(language)
	if index := strings.IndexAny(base, "-_"); index > 0 {
		base = base[:index]
	}
	if name, ok := museLanguages[base]; ok {
		return name
	}
	return language
}
