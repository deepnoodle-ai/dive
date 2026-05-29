package media

import (
	"fmt"
	"strings"
)

// AudioFormat represents an audio output format.
type AudioFormat string

const (
	AudioFormatMP3  AudioFormat = "mp3"
	AudioFormatOpus AudioFormat = "opus"
	AudioFormatAAC  AudioFormat = "aac"
	AudioFormatFLAC AudioFormat = "flac"
	AudioFormatWAV  AudioFormat = "wav"
	AudioFormatPCM  AudioFormat = "pcm"
)

// MIMEType returns the MIME type string for the audio format.
func (f AudioFormat) MIMEType() string {
	switch f {
	case AudioFormatMP3:
		return "audio/mpeg"
	case AudioFormatOpus:
		return "audio/opus"
	case AudioFormatAAC:
		return "audio/aac"
	case AudioFormatFLAC:
		return "audio/flac"
	case AudioFormatWAV:
		return "audio/wav"
	case AudioFormatPCM:
		return "audio/pcm"
	default:
		return "audio/mpeg"
	}
}

// FileExtension returns the file extension (with dot) for the audio format.
func (f AudioFormat) FileExtension() string {
	switch f {
	case AudioFormatMP3:
		return ".mp3"
	case AudioFormatOpus:
		return ".opus"
	case AudioFormatAAC:
		return ".aac"
	case AudioFormatFLAC:
		return ".flac"
	case AudioFormatWAV:
		return ".wav"
	case AudioFormatPCM:
		return ".pcm"
	default:
		return ".mp3"
	}
}

// String returns the string representation.
func (f AudioFormat) String() string {
	return string(f)
}

// ValidateAudioFormat returns an error if the format is not recognized.
func ValidateAudioFormat(f AudioFormat) error {
	switch f {
	case AudioFormatMP3, AudioFormatOpus, AudioFormatAAC, AudioFormatFLAC, AudioFormatWAV, AudioFormatPCM, "":
		return nil
	default:
		return fmt.Errorf("invalid audio format %q; must be mp3, opus, aac, flac, wav, or pcm", f)
	}
}

// AudioFormatFromMIME returns the AudioFormat corresponding to a MIME type.
func AudioFormatFromMIME(mime string) AudioFormat {
	mime = strings.ToLower(strings.TrimSpace(strings.Split(mime, ";")[0]))
	switch mime {
	case "audio/mpeg", "audio/mp3", "audio/mpga":
		return AudioFormatMP3
	case "audio/opus", "audio/ogg":
		return AudioFormatOpus
	case "audio/aac":
		return AudioFormatAAC
	case "audio/flac":
		return AudioFormatFLAC
	case "audio/wav", "audio/wave", "audio/x-wav":
		return AudioFormatWAV
	case "audio/pcm", "audio/l16":
		return AudioFormatPCM
	default:
		return AudioFormatMP3
	}
}

// AudioExtensionFromMIME returns a suitable file extension for a MIME type.
func AudioExtensionFromMIME(mime string) string {
	switch strings.ToLower(strings.TrimSpace(strings.Split(mime, ";")[0])) {
	case "audio/mp4":
		return ".mp4"
	case "audio/m4a", "audio/x-m4a":
		return ".m4a"
	case "audio/webm":
		return ".webm"
	case "audio/mpeg":
		return ".mp3"
	}
	return AudioFormatFromMIME(mime).FileExtension()
}

// DetectAudioMIMEFromBytes returns the MIME type based on common audio magic bytes.
// It falls back to audio/mpeg because MP3 is the most common compressed input.
func DetectAudioMIMEFromBytes(data []byte) string {
	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WAVE" {
		return "audio/wav"
	}
	if len(data) >= 4 && string(data[0:4]) == "fLaC" {
		return "audio/flac"
	}
	if len(data) >= 4 && string(data[0:4]) == "OggS" {
		return "audio/ogg"
	}
	if len(data) >= 3 && string(data[0:3]) == "ID3" {
		return "audio/mpeg"
	}
	if len(data) >= 2 && data[0] == 0xFF && data[1]&0xE0 == 0xE0 {
		return "audio/mpeg"
	}
	if len(data) >= 12 && string(data[4:8]) == "ftyp" {
		return "audio/mp4"
	}
	if len(data) >= 4 && data[0] == 0x1A && data[1] == 0x45 && data[2] == 0xDF && data[3] == 0xA3 {
		return "audio/webm"
	}
	return "audio/mpeg"
}

// PCMToWAV wraps signed 16-bit little-endian PCM data in a WAV container.
func PCMToWAV(pcm []byte, sampleRate, channels, bitsPerSample int) []byte {
	if sampleRate <= 0 {
		sampleRate = 24000
	}
	if channels <= 0 {
		channels = 1
	}
	if bitsPerSample <= 0 {
		bitsPerSample = 16
	}

	byteRate := sampleRate * channels * bitsPerSample / 8
	blockAlign := channels * bitsPerSample / 8
	dataLen := len(pcm)
	out := make([]byte, 44+dataLen)

	copy(out[0:4], "RIFF")
	putLE32(out[4:8], uint32(36+dataLen))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	putLE32(out[16:20], 16)
	putLE16(out[20:22], 1)
	putLE16(out[22:24], uint16(channels))
	putLE32(out[24:28], uint32(sampleRate))
	putLE32(out[28:32], uint32(byteRate))
	putLE16(out[32:34], uint16(blockAlign))
	putLE16(out[34:36], uint16(bitsPerSample))
	copy(out[36:40], "data")
	putLE32(out[40:44], uint32(dataLen))
	copy(out[44:], pcm)
	return out
}

func putLE16(dst []byte, v uint16) {
	dst[0] = byte(v)
	dst[1] = byte(v >> 8)
}

func putLE32(dst []byte, v uint32) {
	dst[0] = byte(v)
	dst[1] = byte(v >> 8)
	dst[2] = byte(v >> 16)
	dst[3] = byte(v >> 24)
}
