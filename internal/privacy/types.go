package privacy

import (
	"context"
	"sync"
)

// PrivacyMode controls the level of anonymization.
type PrivacyMode string

const (
	ModeStandard PrivacyMode = "standard" // emails, phones, addresses, names
	ModeStrict   PrivacyMode = "strict"   // + company names, product names, context
)

// PIIType represents a category of personally identifiable information.
type PIIType string

const (
	PIIEmail   PIIType = "EMAIL"
	PIIPhone   PIIType = "PHONE"
	PIIName    PIIType = "NAME"
	PIIAddress PIIType = "ADDRESS"
	PIICompany PIIType = "COMPANY"
	PIIProduct PIIType = "PRODUCT"
)

// PIIMatch represents a detected PII occurrence in text.
type PIIMatch struct {
	Type       PIIType
	Value      string
	StartIndex int
	EndIndex   int
}

// Detector is the interface for PII detection.
// Implementations can use regex, keyword matching, NLP, etc.
type Detector interface {
	Detect(text string) []PIIMatch
}

// ---- Token Mapping (request-scoped via context) ----

// tokenMappingKey is the context key for token mapping.
type tokenMappingKey struct{}

// TokenMapping holds the bidirectional mapping between original values and tokens.
// This is request-scoped: created per request, stored in context, and garbage collected
// when the request context is done. No global state.
type TokenMapping struct {
	mu       sync.RWMutex
	forward  map[string]string  // original value -> token (e.g., "abc@gmail.com" -> "[EMAIL_1]")
	reverse  map[string]string  // token -> original value (e.g., "[EMAIL_1]" -> "abc@gmail.com")
	counters map[PIIType]int    // per-type counter for generating unique tokens
}

// NewTokenMapping creates a new empty mapping.
func NewTokenMapping() *TokenMapping {
	return &TokenMapping{
		forward:  make(map[string]string),
		reverse:  make(map[string]string),
		counters: make(map[PIIType]int),
	}
}

// WithTokenMapping attaches a TokenMapping to a context.
func WithTokenMapping(ctx context.Context, tm *TokenMapping) context.Context {
	return context.WithValue(ctx, tokenMappingKey{}, tm)
}

// GetTokenMapping retrieves the TokenMapping from context.
func GetTokenMapping(ctx context.Context) *TokenMapping {
	if tm, ok := ctx.Value(tokenMappingKey{}).(*TokenMapping); ok {
		return tm
	}
	return nil
}

// GetOrCreateToken returns the token for a value, creating one if it doesn't exist.
func (m *TokenMapping) GetOrCreateToken(piiType PIIType, originalValue string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already tokenized
	if token, exists := m.forward[originalValue]; exists {
		return token
	}

	// Create new token
	m.counters[piiType]++
	token := "[" + string(piiType) + "_" + itoa(m.counters[piiType]) + "]"

	m.forward[originalValue] = token
	m.reverse[token] = originalValue

	return token
}

// GetOriginal returns the original value for a token.
func (m *TokenMapping) GetOriginal(token string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	original, exists := m.reverse[token]
	return original, exists
}

// AllReverse returns a copy of all token -> original mappings (for rehydration).
func (m *TokenMapping) AllReverse() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]string, len(m.reverse))
	for k, v := range m.reverse {
		result[k] = v
	}
	return result
}

// Size returns the number of tokenized values.
func (m *TokenMapping) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.forward)
}

// itoa is a simple int-to-string without importing strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
