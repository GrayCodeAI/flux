package client

import (
	"context"
	"log/slog"
	"strings"
)

// CondenseOptions controls how a ConversationCondenser reduces a message
// history.
type CondenseOptions struct {
	// MaxSize is the message-count threshold above which condensation runs.
	// When len(messages) <= MaxSize, the history is returned unchanged.
	MaxSize int
	// KeepFirst is the number of leading messages preserved verbatim (e.g. a
	// system prompt and the opening turns). The middle span between the kept
	// head and tail is what gets summarized.
	KeepFirst int
}

// ConversationCondenser reduces a message history so that long conversations
// stay within a model's context window while preserving salient information.
type ConversationCondenser interface {
	// Condense returns a reduced copy of messages according to opts. It must
	// not mutate the input slice. When no reduction is needed it may return the
	// input slice unchanged.
	Condense(ctx context.Context, messages []EyrieMessage, opts CondenseOptions) ([]EyrieMessage, error)
}

// LLMSummarizingCondenser condenses a conversation by summarizing its middle
// span via an LLM call, keeping the first KeepFirst messages and the tail
// intact. The summary is inserted as a single system note between the head and
// the tail.
//
// The summary call uses the Weak model role when a ModelRoles is configured
// (see WithModelRoles / ResolveRole), so summarization runs on a cheaper model.
type LLMSummarizingCondenser struct {
	provider Provider
	roles    ModelRoles
	// prompt is the instruction prepended to the messages being summarized.
	prompt string
	// maxTokens caps the summary length; 0 lets the provider default apply.
	maxTokens int
}

// Compile-time check that LLMSummarizingCondenser implements
// ConversationCondenser.
var _ ConversationCondenser = (*LLMSummarizingCondenser)(nil)

const defaultCondenseSummaryPrompt = "Summarize the following conversation excerpt concisely, " +
	"preserving facts, decisions, names, and any open questions. " +
	"Respond with the summary only."

// CondenserOption configures an LLMSummarizingCondenser.
type CondenserOption func(*LLMSummarizingCondenser)

// WithCondenserRoles sets the model roles used by the condenser. The Weak role
// is preferred for the summary call.
func WithCondenserRoles(roles ModelRoles) CondenserOption {
	return func(c *LLMSummarizingCondenser) { c.roles = roles }
}

// WithCondenserPrompt overrides the default summarization instruction.
func WithCondenserPrompt(prompt string) CondenserOption {
	return func(c *LLMSummarizingCondenser) {
		if prompt != "" {
			c.prompt = prompt
		}
	}
}

// WithCondenserMaxTokens caps the number of tokens requested for the summary.
func WithCondenserMaxTokens(n int) CondenserOption {
	return func(c *LLMSummarizingCondenser) { c.maxTokens = n }
}

// NewLLMSummarizingCondenser creates a condenser that summarizes via the given
// provider. The provider must not be nil.
func NewLLMSummarizingCondenser(provider Provider, opts ...CondenserOption) *LLMSummarizingCondenser {
	if provider == nil {
		slog.Error("NewLLMSummarizingCondenser provider must not be nil; returning nil")
		return nil
	}
	c := &LLMSummarizingCondenser{
		provider: provider,
		prompt:   defaultCondenseSummaryPrompt,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Condense implements ConversationCondenser. When len(messages) exceeds
// MaxSize, it keeps the first KeepFirst messages, summarizes the middle span,
// inserts the summary as a system note, and keeps the remaining tail.
func (c *LLMSummarizingCondenser) Condense(ctx context.Context, messages []EyrieMessage, opts CondenseOptions) ([]EyrieMessage, error) {
	if opts.MaxSize <= 0 || len(messages) <= opts.MaxSize {
		return messages, nil
	}

	keepFirst := opts.KeepFirst
	if keepFirst < 0 {
		keepFirst = 0
	}
	if keepFirst > len(messages) {
		keepFirst = len(messages)
	}

	// Keep the tail so that head + tail fit within MaxSize, reserving one slot
	// for the inserted summary note.
	tailCount := opts.MaxSize - keepFirst - 1
	if tailCount < 0 {
		tailCount = 0
	}
	middleEnd := len(messages) - tailCount
	if middleEnd < keepFirst {
		middleEnd = keepFirst
	}

	middle := messages[keepFirst:middleEnd]
	if len(middle) == 0 {
		// Nothing to summarize (e.g. KeepFirst already covers everything).
		return messages, nil
	}

	summary, err := c.summarize(ctx, middle)
	if err != nil {
		return nil, err
	}

	note := EyrieMessage{
		Role:    "system",
		Content: "[summary of earlier conversation]\n" + summary,
	}

	out := make([]EyrieMessage, 0, keepFirst+1+tailCount)
	out = append(out, messages[:keepFirst]...)
	out = append(out, note)
	out = append(out, messages[middleEnd:]...)
	return out, nil
}

// summarize asks the provider (via the Weak role when available) to summarize
// the given span of messages.
func (c *LLMSummarizingCondenser) summarize(ctx context.Context, span []EyrieMessage) (string, error) {
	var b strings.Builder
	for _, m := range span {
		b.WriteString(m.Role)
		b.WriteString(": ")
		// Indent embedded newlines so a message body cannot forge a new
		// "role:" turn at column 0 (e.g. content containing "\nassistant: ...").
		// Without this, a flat "role: content" transcript is a prompt-injection
		// vector into the summarization call.
		b.WriteString(strings.ReplaceAll(m.Content, "\n", "\n    "))
		b.WriteByte('\n')
	}

	req := []EyrieMessage{{Role: "user", Content: b.String()}}
	opts := ChatOptions{
		Model:     ResolveRole(c.roles, RoleWeak),
		System:    c.prompt,
		MaxTokens: c.maxTokens,
	}

	resp, err := c.provider.Chat(ctx, req, opts)
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", nil
	}
	return resp.Content, nil
}

// CondensingProvider wraps a Provider and runs a ConversationCondenser over the
// request messages before delegating to the inner provider. It follows the same
// decorator pattern as BudgetProvider and TracingProvider.
//
// Condensation applies to both Chat and StreamChat. A nil condenser or
// non-positive CondenseOptions.MaxSize disables condensation (pass-through).
type CondensingProvider struct {
	inner     Provider
	condenser ConversationCondenser
	opts      CondenseOptions
}

// Compile-time check that CondensingProvider implements Provider.
var _ Provider = (*CondensingProvider)(nil)

// NewCondensingProvider wraps inner so that request histories are condensed via
// condenser using the given options. The inner provider must not be nil.
func NewCondensingProvider(inner Provider, condenser ConversationCondenser, opts CondenseOptions) *CondensingProvider {
	if inner == nil {
		slog.Error("NewCondensingProvider inner provider must not be nil; returning nil")
		return nil
	}
	return &CondensingProvider{inner: inner, condenser: condenser, opts: opts}
}

// Name returns the inner provider's name.
func (p *CondensingProvider) Name() string { return p.inner.Name() }

// Ping delegates to the inner provider.
func (p *CondensingProvider) Ping(ctx context.Context) error { return p.inner.Ping(ctx) }

// Chat condenses the messages, then delegates to the inner provider.
func (p *CondensingProvider) Chat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*EyrieResponse, error) {
	msgs, err := p.condense(ctx, messages)
	if err != nil {
		return nil, err
	}
	return p.inner.Chat(ctx, msgs, opts)
}

// StreamChat condenses the messages, then delegates to the inner provider.
func (p *CondensingProvider) StreamChat(ctx context.Context, messages []EyrieMessage, opts ChatOptions) (*StreamResult, error) {
	msgs, err := p.condense(ctx, messages)
	if err != nil {
		return nil, err
	}
	return p.inner.StreamChat(ctx, msgs, opts)
}

func (p *CondensingProvider) condense(ctx context.Context, messages []EyrieMessage) ([]EyrieMessage, error) {
	if p.condenser == nil || p.opts.MaxSize <= 0 {
		return messages, nil
	}
	return p.condenser.Condense(ctx, messages, p.opts)
}
