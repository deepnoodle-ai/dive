package llm

import (
	"encoding/base64"
	"encoding/json"
)

// Token cost of embedded media, which is not proportional to its byte size.
const (
	// base64ImageTokens is roughly what one image costs on the major providers
	// — on the order of a thousand tokens however many megabytes the base64
	// encoding runs to (Anthropic tops out near 1600 for a full-size image,
	// OpenAI near 1100, Gemini lower still). This is the high end, so the
	// estimate errs toward compacting early rather than overflowing.
	base64ImageTokens = 1600

	// base64DocumentBytesPerToken approximates a PDF or video from its decoded
	// size. Providers bill a document per page — its text plus a rendered image
	// of the page — which lands near 2k tokens for every ~50 KB of PDF. Crude,
	// but within an order of magnitude, which byte-count sizing is not.
	base64DocumentBytesPerToken = 25
)

// EstimateTokens approximates a message's token footprint. Text, tool inputs,
// and tool results are sized from their serialized JSON (~4 bytes per token);
// counting the serialization rather than Message.Text is what catches tool
// payloads, often the largest part of a turn.
//
// Base64 media is sized separately, because its token cost has almost nothing
// to do with its encoded length: a 1.4 MB screenshot serializes to ~1.9 MB of
// base64 but costs ~1.6k tokens. Charging it 4 bytes per token would call that
// half a million, enough for a single attached image to look like a full
// context window and trigger an early compaction or an exhausted task budget.
//
// Best effort throughout: a marshal error yields 0.
func EstimateTokens(m *Message) int {
	total := 0
	for _, content := range m.Content {
		total += EstimateContentTokens(content)
	}
	return total
}

// EstimateContentTokens approximates one content block's token footprint.
func EstimateContentTokens(c Content) int {
	switch cc := c.(type) {
	case *ImageContent:
		if isBase64Source(cc.Source) {
			return base64ImageTokens
		}
	case *DocumentContent:
		if isBase64Source(cc.Source) {
			decoded := base64.StdEncoding.DecodedLen(len(cc.Source.Data))
			if tokens := decoded / base64DocumentBytesPerToken; tokens > base64ImageTokens {
				return tokens
			}
			return base64ImageTokens
		}
	}
	data, err := json.Marshal(c)
	if err != nil {
		return 0
	}
	return len(data) / 4
}

func isBase64Source(s *ContentSource) bool {
	return s != nil && s.Type == ContentSourceTypeBase64 && s.Data != ""
}
