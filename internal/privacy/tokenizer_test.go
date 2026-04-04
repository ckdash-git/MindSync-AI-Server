package privacy

import (
	"context"
	"strings"
	"testing"
)

func TestTokenizer_Basic(t *testing.T) {
	detector := NewCompositeDetector(NewRegexDetector(), NewKeywordDetector())
	tokenizer := NewTokenizer(detector)

	mapping := NewTokenMapping()
	ctx := WithTokenMapping(context.Background(), mapping)

	input := "My email is abc@gmail.com and call me at +1-555-123-4567"
	result, err := tokenizer.Tokenize(ctx, input)
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}

	// Should not contain original PII
	if strings.Contains(result, "abc@gmail.com") {
		t.Error("result still contains email")
	}

	// Should contain tokens
	if !strings.Contains(result, "[EMAIL_1]") {
		t.Errorf("result missing EMAIL token: %q", result)
	}

	// Mapping should be populated
	if mapping.Size() == 0 {
		t.Error("mapping is empty")
	}

	// Detokenize should restore original
	restored := tokenizer.Detokenize(ctx, result)
	if !strings.Contains(restored, "abc@gmail.com") {
		t.Errorf("detokenized missing email: %q", restored)
	}
}

func TestTokenizer_SameValueSameToken(t *testing.T) {
	detector := NewCompositeDetector(NewRegexDetector())
	tokenizer := NewTokenizer(detector)

	mapping := NewTokenMapping()
	ctx := WithTokenMapping(context.Background(), mapping)

	// Same email appears twice
	input := "Email: foo@bar.com and again foo@bar.com"
	result, err := tokenizer.Tokenize(ctx, input)
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}

	// Should use same token for same value
	count := strings.Count(result, "[EMAIL_1]")
	if count != 2 {
		t.Errorf("expected 2 occurrences of [EMAIL_1], got %d in %q", count, result)
	}

	// Mapping size should be 1 (same value = same token)
	if mapping.Size() != 1 {
		t.Errorf("expected mapping size 1, got %d", mapping.Size())
	}
}

func TestTokenizer_NoMapping(t *testing.T) {
	detector := NewCompositeDetector(NewRegexDetector())
	tokenizer := NewTokenizer(detector)

	// No mapping in context = passthrough
	ctx := context.Background()
	input := "Email: foo@bar.com"
	result, err := tokenizer.Tokenize(ctx, input)
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}
	if result != input {
		t.Errorf("expected passthrough, got %q", result)
	}
}

func TestTokenizer_NameDetection(t *testing.T) {
	detector := NewCompositeDetector(NewRegexDetector(), NewKeywordDetector())
	tokenizer := NewTokenizer(detector)

	mapping := NewTokenMapping()
	ctx := WithTokenMapping(context.Background(), mapping)

	input := "My name is John Smith and my email is john@example.com"
	result, err := tokenizer.Tokenize(ctx, input)
	if err != nil {
		t.Fatalf("Tokenize: %v", err)
	}

	if strings.Contains(result, "John Smith") {
		t.Errorf("result still contains name: %q", result)
	}
	if strings.Contains(result, "john@example.com") {
		t.Errorf("result still contains email: %q", result)
	}
}


