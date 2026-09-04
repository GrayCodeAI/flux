package client

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
)

// ModerationProvider wraps any Provider and validates input messages before
// forwarding requests. It checks for blocked patterns (regex), token count
// limits, and custom safety checks.
//
// ModerationProvider is safe for concurrent use.
type ModerationProvider struct {
	inner          Provider
	blockedRegexps []*regexp.Regexp
	maxTokens      int // 0 means no limit
	customChecker  func(string) error
}

// Compile-time check that ModerationProvider implements Provider.
var _ Provider = (*ModerationProvider)(nil)

// ModerationOption configures a ModerationProvider.
type ModerationOption func(*ModerationProvider)

// WithBlockedPatterns sets regex patterns that will block matching input.
// Each pattern is compiled as a Go regexp.
func WithBlockedPatterns(patterns []string) ModerationOption {
	return func(mp *ModerationProvider) {
		mp.blockedRegexps = make([]*regexp.Regexp, 0, len(patterns))
		for _, p := range patterns {
			re, err := regexp.Compile(p)
			if err != nil {
				slog.Error("WithBlockedPatterns: invalid regex, skipping", "pattern", p, "error", err)
				continue
			}
			mp.blockedRegexps = append(mp.blockedRegexps, re)
		}
	}
}

// WithModerationMaxTokens sets the maximum allowed total token count across
// all input messages. Token count is estimated as len(strings.Fields(text))
// for simplicity. A value of 0 (default) means no limit.
func WithModerationMaxTokens(n int) ModerationOption {
	return func(mp *ModerationProvider) {
		mp.maxTokens = n
	}
}

// WithCustomChecker sets a custom validation function that receives the
// concatenated text content of all messages. If the function returns a
// non-nil error, the request is blocked.
func WithCustomChecker(fn func(string) error) ModerationOption {
	return func(mp *ModerationProvider) {
		mp.customChecker = fn
	}
}

// NewModerationProvider wraps inner with content moderation. At least one
// moderation option should be provided or the wrapper is a no-op.
func NewModerationProvider(inner Provider, opts ...ModerationOption) *ModerationProvider {
	if inner == nil {
		slog.Error("NewModerationProvider inner provider must not be nil; returning nil")
		return nil
	}
	mp := &ModerationProvider{inner: inner}
	for _, opt := range opts {
		opt(mp)
	}
	return mp
}

// Name returns the inner provider name suffixed with "/moderation".
func (mp *ModerationProvider) Name() string {
	return mp.inner.Name() + "/moderation"
}

// Ping delegates to the inner provider.
func (mp *ModerationProvider) Ping(ctx context.Context) error {
	return mp.inner.Ping(ctx)
}

// Chat validates the input messages and forwards to the inner provider.
func (mp *ModerationProvider) Chat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*GraycodeRouterResponse, error) {
	if err := mp.moderate(messages); err != nil {
		return nil, err
	}
	return mp.inner.Chat(ctx, messages, opts)
}

// StreamChat validates the input messages and forwards to the inner provider.
func (mp *ModerationProvider) StreamChat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*StreamResult, error) {
	if err := mp.moderate(messages); err != nil {
		return nil, err
	}
	return mp.inner.StreamChat(ctx, messages, opts)
}

// moderate runs all validation checks on the input messages.
func (mp *ModerationProvider) moderate(messages []GraycodeRouterMessage) error {
	text := extractText(messages)

	// Check blocked patterns.
	for _, re := range mp.blockedRegexps {
		if re.MatchString(text) {
			return fmt.Errorf("graycode-router: content moderation blocked: message matches blocked pattern %q", re.String())
		}
	}

	// Check token limit.
	if mp.maxTokens > 0 {
		tokens := estimateTokens(text)
		if tokens > mp.maxTokens {
			return fmt.Errorf("graycode-router: content moderation blocked: estimated token count %d exceeds limit %d", tokens, mp.maxTokens)
		}
	}

	// Check custom checker.
	if mp.customChecker != nil {
		if err := mp.customChecker(text); err != nil {
			return fmt.Errorf("graycode-router: content moderation blocked: %w", err)
		}
	}

	return nil
}

// extractText concatenates the text content of all messages.
func extractText(messages []GraycodeRouterMessage) string {
	var b strings.Builder
	for _, m := range messages {
		if m.Content != "" {
			b.WriteString(m.Content)
			b.WriteByte(' ')
		}
		for _, cp := range m.ContentParts {
			if cp.Type == "text" && cp.Text != "" {
				b.WriteString(cp.Text)
				b.WriteByte(' ')
			}
		}
	}
	return b.String()
}

// estimateTokens returns a rough token count based on whitespace-split words.
func estimateTokens(text string) int {
	return len(strings.Fields(text))
}
