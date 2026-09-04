package client

import (
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	tiktoken "github.com/tiktoken-go/tokenizer"
)

var whitespacePattern = regexp.MustCompile(`\s+`)

var (
	tokenizerOnce sync.Once
	tokenizerBPE  tiktoken.Codec
	tokenizerErr  error
)

// estimateTextTokens uses a lightweight character-based heuristic. GraycodeRouter needs
// cheap local budgeting, not shared cross-repo token infrastructure.
func estimateTextTokens(text string) int {
	if count, ok := preciseTokenCount(text); ok {
		return count
	}
	return fallbackTokenCount(text)
}

func preciseTokenCount(text string) (int, bool) {
	if text == "" {
		return 0, true
	}

	tokenizerOnce.Do(func() {
		tokenizerBPE, tokenizerErr = tiktoken.Get(tiktoken.Cl100kBase)
	})
	if tokenizerErr != nil || tokenizerBPE == nil {
		return 0, false
	}
	count, err := tokenizerBPE.Count(text)
	if err != nil {
		return 0, false
	}
	return count, true
}

func fallbackTokenCount(text string) int {
	if text == "" {
		return 0
	}
	length := utf8.RuneCountInString(text)
	if length < 30 {
		return (length + 2) / 3
	}
	if length < 100 {
		return (length + 3) / 4
	}

	spaces := 0
	sample := len(text)
	if sample > 200 {
		sample = 200
	}
	for i := 0; i < sample; i++ {
		switch text[i] {
		case ' ', '\n', '\t', '\r':
			spaces++
		}
	}

	spaceRatio := float64(spaces) / float64(sample)
	nonSpaceChars := float64(length) * (1 - spaceRatio)
	spaceTokens := float64(length) * spaceRatio
	return int(nonSpaceChars/3.5 + spaceTokens)
}

// compressForSummary keeps PromptOptimizer self-contained. It performs a small
// whitespace-normalizing reduction instead of depending on shrike's full pipeline.
func compressForSummary(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return whitespacePattern.ReplaceAllString(text, " ")
}
