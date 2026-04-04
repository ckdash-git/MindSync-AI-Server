package privacy

import (
	"context"
	"sort"
	"strings"
)

// Tokenizer replaces PII matches with tokens using a request-scoped TokenMapping.
type Tokenizer struct {
	detector Detector
}

// NewTokenizer creates a new Tokenizer with the given detector.
func NewTokenizer(detector Detector) *Tokenizer {
	return &Tokenizer{detector: detector}
}

// Tokenize detects PII in text and replaces each match with a token.
// The mapping is stored in the TokenMapping retrieved from context.
func (t *Tokenizer) Tokenize(ctx context.Context, text string) (string, error) {
	mapping := GetTokenMapping(ctx)
	if mapping == nil {
		// No mapping in context — return text as-is (privacy not configured)
		return text, nil
	}

	matches := t.detector.Detect(text)
	if len(matches) == 0 {
		return text, nil
	}

	// Sort matches by start index (descending) to replace from end to start
	// This preserves indices during replacement.
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].StartIndex > matches[j].StartIndex
	})

	result := text
	for _, match := range matches {
		token := mapping.GetOrCreateToken(match.Type, match.Value)
		result = result[:match.StartIndex] + token + result[match.EndIndex:]
	}

	return result, nil
}

// Detokenize replaces tokens back to original values (for non-streaming use).
func (t *Tokenizer) Detokenize(ctx context.Context, text string) string {
	mapping := GetTokenMapping(ctx)
	if mapping == nil {
		return text
	}

	reverseMap := mapping.AllReverse()
	result := text
	for token, original := range reverseMap {
		result = strings.ReplaceAll(result, token, original)
	}

	return result
}
