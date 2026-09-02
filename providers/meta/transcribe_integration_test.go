//go:build integration

package meta

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/wonton/assert"
)

// loadTestAudio reads the WAV named by META_ASR_WAV. No fixture is committed:
// the endpoint takes mono 16-bit PCM at 16 or 24 kHz, which is a few seconds of
// ffmpeg away from any recording and not worth carrying in the repo.
//
//	say -o speech.aiff "How is the weather in Portland today?"
//	ffmpeg -i speech.aiff -ac 1 -ar 24000 -c:a pcm_s16le -map_metadata -1 speech.wav
func loadTestAudio(t *testing.T) []byte {
	t.Helper()
	path := os.Getenv("META_ASR_WAV")
	if path == "" {
		t.Skip("META_ASR_WAV not set")
	}
	audio, err := os.ReadFile(path)
	assert.NoError(t, err)
	return audio
}

func TestIntegration_Transcribe(t *testing.T) {
	skipIfNoAPIKey(t)
	audio := loadTestAudio(t)

	provider := NewMediaProvider()
	ctx := testContext(t, 120*time.Second)

	result, err := provider.Transcribe(ctx, audio, &media.Config{Language: "en"})
	assert.NoError(t, err)

	t.Logf("transcript: %q (%s)", result.Text, result.Duration)
	assert.True(t, len(result.Text) > 0, "transcript should not be empty")
	assert.Equal(t, result.Model, ModelMuseVoiceTranscribe)
	assert.True(t, result.Duration > 0, "audio duration should be reported")
	assert.True(t, result.Metadata["session_id"] != "", "session id should be reported")
}

// Diarization is the mode with the most machinery behind it, and the only one
// that fills in the turn list, so it is worth exercising against the real API
// rather than trusting the request shape alone.
func TestIntegration_TranscribeDiarization(t *testing.T) {
	skipIfNoAPIKey(t)
	audio := loadTestAudio(t)

	provider := NewMediaProvider(
		WithTranscriptionMode(TranscriptionModeDiarization),
		WithTranscriptionKeywords("Portland"),
	)
	ctx := testContext(t, 120*time.Second)

	result, err := provider.Transcribe(ctx, audio, &media.Config{})
	assert.NoError(t, err)

	turns, ok := result.Metadata["turns"].([]Turn)
	assert.True(t, ok, "diarization should return turns, got %T", result.Metadata["turns"])
	assert.True(t, len(turns) > 0, "diarization should return at least one turn")
	for _, turn := range turns {
		t.Logf("turn %d [%dms-%dms] speaker %q: %s", turn.TurnID, turn.StartMs, turn.EndMs, turn.Speaker, turn.Transcript)
		assert.True(t, turn.EndMs >= turn.StartMs, "turn %d ends before it starts", turn.TurnID)
	}
	assert.True(t, strings.TrimSpace(result.Text) != "", "transcript should not be empty")
}
