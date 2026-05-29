package media

import (
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestAudioFormat(t *testing.T) {
	assert.Equal(t, "audio/mpeg", AudioFormatMP3.MIMEType())
	assert.Equal(t, ".mp3", AudioFormatMP3.FileExtension())
	assert.Equal(t, "audio/wav", AudioFormatWAV.MIMEType())
	assert.Equal(t, ".wav", AudioFormatWAV.FileExtension())
	assert.NoError(t, ValidateAudioFormat(AudioFormatFLAC))
	assert.Error(t, ValidateAudioFormat(AudioFormat("bad")))
}

func TestAudioFormatFromMIME(t *testing.T) {
	assert.Equal(t, AudioFormatMP3, AudioFormatFromMIME("audio/mpeg"))
	assert.Equal(t, AudioFormatOpus, AudioFormatFromMIME("audio/ogg; codecs=opus"))
	assert.Equal(t, AudioFormatWAV, AudioFormatFromMIME("audio/wav"))
	assert.Equal(t, AudioFormatPCM, AudioFormatFromMIME("audio/L16; rate=24000"))
	assert.Equal(t, ".webm", AudioExtensionFromMIME("audio/webm"))
}

func TestDetectAudioMIMEFromBytes(t *testing.T) {
	assert.Equal(t, "audio/wav", DetectAudioMIMEFromBytes([]byte("RIFFxxxxWAVEdata")))
	assert.Equal(t, "audio/flac", DetectAudioMIMEFromBytes([]byte("fLaCxxxx")))
	assert.Equal(t, "audio/ogg", DetectAudioMIMEFromBytes([]byte("OggSxxxx")))
	assert.Equal(t, "audio/mpeg", DetectAudioMIMEFromBytes([]byte("ID3xxxx")))
	assert.Equal(t, "audio/mpeg", DetectAudioMIMEFromBytes([]byte{0xFF, 0xFB, 0x90, 0x64}))
}

func TestPCMToWAV(t *testing.T) {
	wav := PCMToWAV([]byte{1, 2, 3, 4}, 24000, 1, 16)
	assert.Equal(t, 48, len(wav))
	assert.Equal(t, "RIFF", string(wav[0:4]))
	assert.Equal(t, "WAVE", string(wav[8:12]))
	assert.Equal(t, "data", string(wav[36:40]))
	assert.Equal(t, []byte{1, 2, 3, 4}, wav[44:])
}
