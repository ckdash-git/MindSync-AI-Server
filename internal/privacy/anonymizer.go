package privacy

import (
	"strings"
)

// SemanticAnonymizer handles semantic-level anonymization (company names, product names).
// Only active in strict mode. Uses configurable word lists — no NLP.
type SemanticAnonymizer struct {
	companyWords []string
	productWords []string
	enabled      bool
}

// NewSemanticAnonymizer creates a new semantic anonymizer.
// Only performs anonymization if mode is ModeStrict.
func NewSemanticAnonymizer(mode PrivacyMode) *SemanticAnonymizer {
	return &SemanticAnonymizer{
		companyWords: defaultCompanyWords(),
		productWords: defaultProductWords(),
		enabled:      mode == ModeStrict,
	}
}

// Anonymize replaces known company/product names with generic terms.
func (a *SemanticAnonymizer) Anonymize(text string) string {
	if !a.enabled {
		return text
	}

	result := text

	for _, word := range a.companyWords {
		if strings.Contains(result, word) {
			result = strings.ReplaceAll(result, word, "a company")
		}
	}

	for _, word := range a.productWords {
		if strings.Contains(result, word) {
			result = strings.ReplaceAll(result, word, "a product")
		}
	}

	return result
}

// SetCompanyWords replaces the company word list.
func (a *SemanticAnonymizer) SetCompanyWords(words []string) {
	a.companyWords = words
}

// SetProductWords replaces the product word list.
func (a *SemanticAnonymizer) SetProductWords(words []string) {
	a.productWords = words
}

// defaultCompanyWords returns a starter list of common company indicators.
func defaultCompanyWords() []string {
	return []string{
		"Google", "Apple", "Microsoft", "Amazon", "Meta",
		"Facebook", "Netflix", "Tesla", "OpenAI", "Anthropic",
		"Twitter", "LinkedIn", "Uber", "Lyft", "Airbnb",
		"Stripe", "Shopify", "Slack", "Zoom", "Salesforce",
	}
}

// defaultProductWords returns a starter list of common product names.
func defaultProductWords() []string {
	return []string{
		"iPhone", "MacBook", "iPad", "Windows", "Android",
		"Chrome", "Firefox", "Safari", "ChatGPT", "Copilot",
		"Gmail", "Outlook", "Teams", "Notion",
	}
}
