package privacy

import (
	"context"

	"github.com/ckdash-git/MindSync-AI-Server/internal/logger"
)

// Processor is the top-level privacy processing engine.
// It chains: detection → tokenization → semantic anonymization.
type Processor struct {
	tokenizer  *Tokenizer
	anonymizer *SemanticAnonymizer
	log        *logger.Logger
}

// NewProcessor creates a new privacy processor with all layers.
func NewProcessor(mode PrivacyMode, log *logger.Logger) *Processor {
	// Build multi-layer detector
	regexDetector := NewRegexDetector()
	keywordDetector := NewKeywordDetector()
	compositeDetector := NewCompositeDetector(regexDetector, keywordDetector)

	return &Processor{
		tokenizer:  NewTokenizer(compositeDetector),
		anonymizer: NewSemanticAnonymizer(mode),
		log:        log,
	}
}

// Sanitize runs the full privacy pipeline on input text:
// 1. Detect and tokenize PII
// 2. Apply semantic anonymization (if strict mode)
func (p *Processor) Sanitize(ctx context.Context, text string) (string, error) {
	// Step 1: Tokenize PII (uses mapping from context)
	tokenized, err := p.tokenizer.Tokenize(ctx, text)
	if err != nil {
		return "", err
	}

	// Step 2: Semantic anonymization
	anonymized := p.anonymizer.Anonymize(tokenized)

	mapping := GetTokenMapping(ctx)
	if mapping != nil && mapping.Size() > 0 {
		p.log.DebugContext(ctx, "privacy sanitization applied",
			"tokens_created", mapping.Size(),
		)
	}

	return anonymized, nil
}
