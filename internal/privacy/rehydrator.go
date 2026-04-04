package privacy

import (
	"regexp"
	"strings"
)

// tokenPattern matches token placeholders like [EMAIL_1], [PHONE_2], [NAME_3]
var tokenPattern = regexp.MustCompile(`\[[A-Z]+_\d+\]`)

// StreamRehydrator performs buffer-based token rehydration for streaming responses.
//
// It maintains a rolling buffer that accumulates streaming chunks,
// only flushing content that is confirmed to not contain partial tokens.
// A partial token at the buffer tail (e.g., "[EMA") is retained for the next chunk.
type StreamRehydrator struct {
	mapping *TokenMapping
	buffer  strings.Builder
}

// NewStreamRehydrator creates a new rehydrator with the given token mapping.
func NewStreamRehydrator(mapping *TokenMapping) *StreamRehydrator {
	return &StreamRehydrator{
		mapping: mapping,
	}
}

// Process adds a chunk to the buffer and returns any content that can be safely flushed.
// Returns empty string if all content is still buffered (waiting for complete token).
func (r *StreamRehydrator) Process(chunk string) string {
	r.buffer.WriteString(chunk)

	content := r.buffer.String()

	// Find the last potential partial token start
	// A partial token looks like an opening '[' without a matching ']'
	lastOpen := strings.LastIndexByte(content, '[')

	if lastOpen == -1 {
		// No '[' in buffer — everything is safe to flush
		r.buffer.Reset()
		return r.rehydrate(content)
	}

	// Check if the '[' is part of a complete token
	lastClose := strings.LastIndexByte(content, ']')

	if lastClose > lastOpen {
		// The last '[' has a matching ']' — everything is safe
		r.buffer.Reset()
		return r.rehydrate(content)
	}

	// The last '[' has no matching ']' — it might be a partial token
	// Flush everything before it, keep the rest in buffer
	safe := content[:lastOpen]
	partial := content[lastOpen:]

	r.buffer.Reset()
	r.buffer.WriteString(partial)

	if safe == "" {
		return ""
	}

	return r.rehydrate(safe)
}

// Flush returns any remaining buffered content (call at end of stream).
func (r *StreamRehydrator) Flush() string {
	if r.buffer.Len() == 0 {
		return ""
	}

	content := r.buffer.String()
	r.buffer.Reset()
	return r.rehydrate(content)
}

// rehydrate replaces all tokens in text with their original values.
func (r *StreamRehydrator) rehydrate(text string) string {
	if r.mapping == nil || r.mapping.Size() == 0 {
		return text
	}

	return tokenPattern.ReplaceAllStringFunc(text, func(token string) string {
		if original, ok := r.mapping.GetOriginal(token); ok {
			return original
		}
		// Token not in mapping — leave as-is (log warning in production)
		return token
	})
}
