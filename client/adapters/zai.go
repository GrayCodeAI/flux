package adapters

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GrayCodeAI/eyrie/client/core"

	"github.com/GrayCodeAI/eyrie/types"
)

// ZAIClient uses the OpenAI-compatible endpoint (paas/v4 or coding/paas/v4) first,
// with Anthropic-compatible fallback (/api/anthropic) on retriable errors.
// This provides proper separation for General vs Coding Plan while giving
// both protocol surfaces (exactly as Xiaomi MiMo does for its plans).
type ZAIClient struct {
	router     ProtocolRouter
	providerID string
	logger     *slog.Logger
}

// NewZAIClient builds a Z.AI dual-protocol client for a given plan/region gateway.
// openAIBase should be the resolved general or coding paas base.
// anthropicBase should be the resolved /api/anthropic (global or cn).
// The same apiKey is used for both sides; the plan subscription attached to the key
// controls quota/billing when using the coding path.
func NewZAIClient(apiKey, openAIBase, anthropicBase string, compat *OpenAICompatConfig, providerID string, opts ...core.ClientOption) *ZAIClient {
	openAIBase = strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	anthropicBase = strings.TrimRight(strings.TrimSpace(anthropicBase), "/")

	zaiOpts := append([]core.ClientOption{core.WithProviderName(providerID)}, opts...)
	o := NewOpenAIClient(apiKey, openAIBase, compat, zaiOpts...)

	var a *AnthropicClient
	if anthropicBase != "" {
		a = NewAnthropicClient(apiKey, anthropicBase, zaiOpts...)
	}

	return &ZAIClient{
		router:     ProtocolRouter{OpenAI: o, Anthropic: a},
		providerID: providerID,
		logger:     slog.Default(),
	}
}

func (c *ZAIClient) Name() string {
	if c.router.OpenAI != nil {
		return c.router.OpenAI.Name()
	}
	return c.providerID
}

func (c *ZAIClient) Chat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.EyrieResponse, error) {
	return c.router.Chat(ctx, messages, opts, ChatProtocolCompletions, func(err error, _ *core.EyrieResponse) bool {
		if err != nil && c.router.Anthropic != nil && zaiFallbackChatError(err) {
			c.logger.Info("Z.AI: OpenAI endpoint failed; retrying via Anthropic compatibility",
				"provider", c.providerID, "error", err)
			return true
		}
		return false
	})
}

func (c *ZAIClient) StreamChat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	return c.router.StreamChat(ctx, messages, opts, ProtocolStreamConfig{
		Primary: ChatProtocolCompletions,
		FallbackOnError: func(err error) bool {
			if c.router.Anthropic != nil && zaiFallbackChatError(err) {
				c.logger.Info("Z.AI: OpenAI stream failed; retrying via Anthropic compatibility",
					"provider", c.providerID, "error", err)
				return true
			}
			return false
		},
	})
}

func (c *ZAIClient) Ping(ctx context.Context) error {
	if err := c.router.OpenAI.Ping(ctx); err == nil {
		return nil
	} else if c.router.Anthropic == nil || !zaiRetryableChatError(err) {
		return err
	}
	return c.router.Anthropic.Ping(ctx)
}

func zaiFallbackChatError(err error) bool {
	if zaiRetryableChatError(err) {
		return true
	}
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	// Common Z.AI / GLM specific transient or format issues seen in the wild
	// (similar to reasoning_content or param problems on other compat layers).
	return strings.Contains(msg, "param incorrect") ||
		strings.Contains(msg, "invalid format") ||
		strings.Contains(msg, "reasoning_content") ||
		(strings.Contains(msg, "http 400") && strings.Contains(msg, "zai"))
}

func zaiRetryableChatError(err error) bool {
	if err == nil {
		return false
	}
	// Z.AI-specific: check HTTP status codes
	msg := err.Error()
	if n := parseHTTPStatusFromError(msg); n > 0 {
		return n >= 500 || n == http.StatusUnauthorized || n == http.StatusForbidden
	}
	// Structured path: trust core.EyrieError's IsRetriable
	var eyrieErr *core.EyrieError
	if errors.As(err, &eyrieErr) {
		return eyrieErr.IsRetriable()
	}
	// Conservative: only retry on explicitly transient errors
	return types.IsTransient(err)
}

var _ core.Provider = (*ZAIClient)(nil)
