package privacy

import (
	"testing"
)

func TestStreamRehydrator_BasicRehydration(t *testing.T) {
	mapping := NewTokenMapping()
	mapping.GetOrCreateToken(PIIEmail, "john@example.com") // Creates [EMAIL_1]

	rh := NewStreamRehydrator(mapping)

	// Complete token in single chunk
	result := rh.Process("Your email is [EMAIL_1] confirmed")
	if result != "Your email is john@example.com confirmed" {
		t.Errorf("got %q", result)
	}
}

func TestStreamRehydrator_SplitToken(t *testing.T) {
	mapping := NewTokenMapping()
	mapping.GetOrCreateToken(PIIEmail, "abc@gmail.com") // Creates [EMAIL_1]

	rh := NewStreamRehydrator(mapping)

	// Token split across two chunks: "[EMA" + "IL_1]"
	result1 := rh.Process("Your email is [EMA")
	// Should flush "Your email is " and buffer "[EMA"
	if result1 != "Your email is " {
		t.Errorf("chunk1: got %q, want %q", result1, "Your email is ")
	}

	result2 := rh.Process("IL_1] confirmed")
	// Should complete the token and rehydrate
	if result2 != "abc@gmail.com confirmed" {
		t.Errorf("chunk2: got %q, want %q", result2, "abc@gmail.com confirmed")
	}
}

func TestStreamRehydrator_MultipleTokens(t *testing.T) {
	mapping := NewTokenMapping()
	mapping.GetOrCreateToken(PIIEmail, "a@b.com")   // [EMAIL_1]
	mapping.GetOrCreateToken(PIIName, "John Smith")  // [NAME_1]

	rh := NewStreamRehydrator(mapping)

	result := rh.Process("Hi [NAME_1], your email [EMAIL_1] is set")
	if result != "Hi John Smith, your email a@b.com is set" {
		t.Errorf("got %q", result)
	}
}

func TestStreamRehydrator_NoTokens(t *testing.T) {
	mapping := NewTokenMapping()
	rh := NewStreamRehydrator(mapping)

	result := rh.Process("No tokens here at all")
	if result != "No tokens here at all" {
		t.Errorf("got %q", result)
	}
}

func TestStreamRehydrator_Flush(t *testing.T) {
	mapping := NewTokenMapping()
	rh := NewStreamRehydrator(mapping)

	// Buffer a partial bracket
	result := rh.Process("Partial [maybe")
	if result != "Partial " {
		t.Errorf("got %q, want %q", result, "Partial ")
	}

	// Flush should return the buffered content
	flushed := rh.Flush()
	if flushed != "[maybe" {
		t.Errorf("flushed %q, want %q", flushed, "[maybe")
	}
}

func TestStreamRehydrator_TokenAtEnd(t *testing.T) {
	mapping := NewTokenMapping()
	mapping.GetOrCreateToken(PIIEmail, "x@y.com") // [EMAIL_1]

	rh := NewStreamRehydrator(mapping)

	result := rh.Process("Email: [EMAIL_1]")
	if result != "Email: x@y.com" {
		t.Errorf("got %q", result)
	}
}

func TestStreamRehydrator_ConsecutiveChunks(t *testing.T) {
	mapping := NewTokenMapping()
	mapping.GetOrCreateToken(PIIEmail, "test@test.com") // [EMAIL_1]

	rh := NewStreamRehydrator(mapping)

	// Simulate many small chunks
	chunks := []string{"Hello", " ", "[", "EMAIL", "_1", "]", " world"}
	var accumulated string

	for _, chunk := range chunks {
		result := rh.Process(chunk)
		accumulated += result
	}
	accumulated += rh.Flush()

	expected := "Hello test@test.com world"
	if accumulated != expected {
		t.Errorf("accumulated %q, want %q", accumulated, expected)
	}
}

func TestStreamRehydrator_UnknownToken(t *testing.T) {
	mapping := NewTokenMapping()
	// Don't add any mapping

	rh := NewStreamRehydrator(mapping)

	// Unknown token should be left as-is
	result := rh.Process("Unknown [EMAIL_99] token")
	if result != "Unknown [EMAIL_99] token" {
		t.Errorf("got %q", result)
	}
}
