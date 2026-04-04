package privacy

import (
	"regexp"
	"sort"
	"strings"
)

// ---- Layer 1: Regex Detector ----

// RegexDetector detects PII using regular expressions.
type RegexDetector struct {
	patterns map[PIIType][]*regexp.Regexp
}

// NewRegexDetector creates a regex-based PII detector.
func NewRegexDetector() *RegexDetector {
	return &RegexDetector{
		patterns: map[PIIType][]*regexp.Regexp{
			PIIEmail: {
				// RFC 5322 simplified
				regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
			},
			PIIPhone: {
				// International: +1-234-567-8901, +44 20 7946 0958
				regexp.MustCompile(`\+?\d{1,3}[\s\-.]?\(?\d{1,4}\)?[\s\-.]?\d{1,4}[\s\-.]?\d{1,9}`),
			},
			PIIAddress: {
				// US-style street addresses: 123 Main St, 456 Elm Ave
				regexp.MustCompile(`\d{1,5}\s+[A-Z][a-zA-Z]*(?:\s+[A-Z][a-zA-Z]*)*\s+(?:St|Street|Ave|Avenue|Blvd|Boulevard|Dr|Drive|Ln|Lane|Rd|Road|Ct|Court|Way|Pl|Place)\.?`),
			},
		},
	}
}

func (d *RegexDetector) Detect(text string) []PIIMatch {
	var matches []PIIMatch

	for piiType, patterns := range d.patterns {
		for _, pattern := range patterns {
			locs := pattern.FindAllStringIndex(text, -1)
			for _, loc := range locs {
				matches = append(matches, PIIMatch{
					Type:       piiType,
					Value:      text[loc[0]:loc[1]],
					StartIndex: loc[0],
					EndIndex:   loc[1],
				})
			}
		}
	}

	return matches
}

// ---- Layer 2: Keyword/Context Detector ----

// KeywordDetector detects PII using keyword dictionaries and contextual heuristics.
type KeywordDetector struct {
	namePatterns []*regexp.Regexp
}

// NewKeywordDetector creates a keyword/context-based detector.
func NewKeywordDetector() *KeywordDetector {
	return &KeywordDetector{
		namePatterns: []*regexp.Regexp{
			// "My name is John Smith"
			regexp.MustCompile(`(?i)(?:my name is|i'?m|i am|call me|this is)\s+([A-Z][a-z]+(?:\s+[A-Z][a-z]+){0,2})`),
			// "From: John Smith" or "Dear John Smith"
			regexp.MustCompile(`(?i)(?:from|dear|hi|hello|hey|to|attn|attention)\s*:?\s+([A-Z][a-z]+(?:\s+[A-Z][a-z]+){0,2})`),
			// "signed by John Smith" or "regards, John Smith"
			regexp.MustCompile(`(?i)(?:signed by|regards|sincerely|best|cheers)\s*,?\s+([A-Z][a-z]+(?:\s+[A-Z][a-z]+){0,2})`),
		},
	}
}

func (d *KeywordDetector) Detect(text string) []PIIMatch {
	var matches []PIIMatch

	for _, pattern := range d.namePatterns {
		allMatches := pattern.FindAllStringSubmatchIndex(text, -1)
		for _, loc := range allMatches {
			if len(loc) >= 4 {
				// Group 1 is the name
				nameStart := loc[2]
				nameEnd := loc[3]
				name := text[nameStart:nameEnd]

				// Filter out common false positives
				if isCommonWord(name) {
					continue
				}

				matches = append(matches, PIIMatch{
					Type:       PIIName,
					Value:      name,
					StartIndex: nameStart,
					EndIndex:   nameEnd,
				})
			}
		}
	}

	return matches
}

// ---- Composite Detector ----

// CompositeDetector chains multiple detectors and deduplicates overlapping matches.
// Higher-priority detectors (added later) win on overlap.
type CompositeDetector struct {
	detectors []Detector
}

// NewCompositeDetector creates a composite detector from multiple detectors.
// Detectors are applied in order; later detectors have higher priority.
func NewCompositeDetector(detectors ...Detector) *CompositeDetector {
	return &CompositeDetector{detectors: detectors}
}

func (d *CompositeDetector) Detect(text string) []PIIMatch {
	var allMatches []PIIMatch

	// Collect all matches with priority (later detectors = higher priority)
	type prioritizedMatch struct {
		PIIMatch
		priority int
	}

	var pMatches []prioritizedMatch

	for i, detector := range d.detectors {
		matches := detector.Detect(text)
		for _, m := range matches {
			pMatches = append(pMatches, prioritizedMatch{
				PIIMatch: m,
				priority: i,
			})
		}
	}

	// Sort by start index, then by priority (higher wins)
	sort.Slice(pMatches, func(i, j int) bool {
		if pMatches[i].StartIndex != pMatches[j].StartIndex {
			return pMatches[i].StartIndex < pMatches[j].StartIndex
		}
		return pMatches[i].priority > pMatches[j].priority
	})

	// Deduplicate: remove overlapping matches (keep higher priority)
	lastEnd := -1
	for _, pm := range pMatches {
		if pm.StartIndex >= lastEnd {
			allMatches = append(allMatches, pm.PIIMatch)
			lastEnd = pm.EndIndex
		}
	}

	return allMatches
}

// ---- Helpers ----

// commonWords that should not be treated as names
var commonWords = map[string]bool{
	"The": true, "This": true, "That": true, "Here": true,
	"There": true, "What": true, "When": true, "Where": true,
	"Which": true, "Who": true, "How": true, "Each": true,
	"Every": true, "Some": true, "Any": true, "All": true,
	"Most": true, "Other": true, "Another": true,
}

func isCommonWord(word string) bool {
	parts := strings.Fields(word)
	if len(parts) == 0 {
		return true
	}
	return commonWords[parts[0]]
}
