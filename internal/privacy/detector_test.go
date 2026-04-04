package privacy

import (
	"testing"
)

func TestRegexDetector_Email(t *testing.T) {
	d := NewRegexDetector()

	tests := []struct {
		input    string
		expected int
		value    string
	}{
		{"My email is john@example.com", 1, "john@example.com"},
		{"Contact me at user.name+tag@domain.co.uk", 1, "user.name+tag@domain.co.uk"},
		{"No email here", 0, ""},
		{"Two emails: a@b.com and c@d.org", 2, ""},
		{"edge@case.io end", 1, "edge@case.io"},
	}

	for _, tt := range tests {
		matches := d.Detect(tt.input)
		emailMatches := filterByType(matches, PIIEmail)
		if len(emailMatches) != tt.expected {
			t.Errorf("input=%q: got %d email matches, want %d", tt.input, len(emailMatches), tt.expected)
		}
		if tt.expected == 1 && tt.value != "" && emailMatches[0].Value != tt.value {
			t.Errorf("input=%q: got value %q, want %q", tt.input, emailMatches[0].Value, tt.value)
		}
	}
}

func TestRegexDetector_Phone(t *testing.T) {
	d := NewRegexDetector()

	tests := []struct {
		input    string
		expected int
	}{
		{"Call me at +1-234-567-8901", 1},
		{"Phone: (555) 123-4567", 1},
		{"No phone here", 0},
		{"+44 20 7946 0958", 1},
	}

	for _, tt := range tests {
		matches := d.Detect(tt.input)
		phoneMatches := filterByType(matches, PIIPhone)
		if len(phoneMatches) != tt.expected {
			t.Errorf("input=%q: got %d phone matches, want %d", tt.input, len(phoneMatches), tt.expected)
		}
	}
}

func TestKeywordDetector_Names(t *testing.T) {
	d := NewKeywordDetector()

	tests := []struct {
		input    string
		expected int
		name     string
	}{
		{"My name is John Smith", 1, "John Smith"},
		{"I'm Jane Doe", 1, "Jane Doe"},
		{"Call me Bob", 1, "Bob"},
		{"Dear Alice Johnson, thank you", 1, "Alice Johnson"},
		{"No names here at all", 0, ""},
		{"Regards, Charlie Brown", 1, "Charlie Brown"},
	}

	for _, tt := range tests {
		matches := d.Detect(tt.input)
		if len(matches) != tt.expected {
			t.Errorf("input=%q: got %d matches, want %d (matches=%v)", tt.input, len(matches), tt.expected, matches)
		}
		if tt.expected == 1 && tt.name != "" && matches[0].Value != tt.name {
			t.Errorf("input=%q: got name %q, want %q", tt.input, matches[0].Value, tt.name)
		}
	}
}

func TestCompositeDetector(t *testing.T) {
	regex := NewRegexDetector()
	keyword := NewKeywordDetector()
	composite := NewCompositeDetector(regex, keyword) // keyword has higher priority

	text := "My name is John Smith and my email is john@example.com"
	matches := composite.Detect(text)

	if len(matches) < 2 {
		t.Fatalf("expected at least 2 matches, got %d: %v", len(matches), matches)
	}

	// Should have both a name and an email
	hasName := false
	hasEmail := false
	for _, m := range matches {
		if m.Type == PIIName {
			hasName = true
		}
		if m.Type == PIIEmail {
			hasEmail = true
		}
	}

	if !hasName {
		t.Error("expected NAME match")
	}
	if !hasEmail {
		t.Error("expected EMAIL match")
	}
}

func TestCompositeDetector_Deduplication(t *testing.T) {
	// Create two detectors that detect overlapping regions
	regex := NewRegexDetector()
	composite := NewCompositeDetector(regex, regex)

	text := "Email: test@example.com"
	matches := composite.Detect(text)

	// Should not have duplicate matches for the same position
	emailMatches := filterByType(matches, PIIEmail)
	if len(emailMatches) != 1 {
		t.Errorf("expected 1 deduplicated email match, got %d", len(emailMatches))
	}
}

func filterByType(matches []PIIMatch, piiType PIIType) []PIIMatch {
	var filtered []PIIMatch
	for _, m := range matches {
		if m.Type == piiType {
			filtered = append(filtered, m)
		}
	}
	return filtered
}
