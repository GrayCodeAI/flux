package adapters

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog/xiaomi"
	"github.com/GrayCodeAI/eyrie/client/core"
)

// MiMoClient uses the official OpenAI-compatible Xiaomi MiMo surface only.
type MiMoClient struct {
	openai     *OpenAIClient
	providerID string
}

// NewMiMoClient builds an OpenAI-compatible MiMo client (payg or token_plan).
func NewMiMoClient(apiKey, openAIBase string, compat *OpenAICompatConfig, providerID string, opts ...core.ClientOption) *MiMoClient {
	openAIBase = strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	mimoOpts := append(append([]core.ClientOption{}, opts...), core.WithMimoAuth(), core.WithProviderName(providerID))
	return &MiMoClient{
		openai:     NewOpenAIClient(apiKey, openAIBase, compat, mimoOpts...),
		providerID: providerID,
	}
}

func (c *MiMoClient) Name() string {
	if c.openai != nil {
		return c.openai.Name()
	}
	return c.providerID
}

func (c *MiMoClient) Chat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.EyrieResponse, error) {
	return c.openai.Chat(ctx, messages, opts)
}

func (c *MiMoClient) StreamChat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	return c.openai.StreamChat(ctx, messages, opts)
}

func (c *MiMoClient) Ping(ctx context.Context) error {
	return c.openai.Ping(ctx)
}

var _ core.Provider = (*MiMoClient)(nil)

// mimoAuthHeaders sets MiMo-preferred authentication on outbound requests.
func mimoAuthHeaders(req *http.Request, apiKey string) {
	xiaomi.SetMimoRequestAuth(req, apiKey)
}

// ProviderID reports the configured MiMo gateway identity.
func (c *MiMoClient) ProviderID() string { return c.providerID }

// MimoRetryableChatError reports whether an error is retryable for MiMo HTTP status rules.
// Kept for callers that classify MiMo errors outside the transport.
func MimoRetryableChatError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if n := parseHTTPStatusFromError(msg); n > 0 {
		return xiaomi.IsRetryableHTTPStatus(n)
	}
	return false
}

func parseHTTPStatusFromError(msg string) int {
	for _, prefix := range []string{"HTTP ", "status ", "error ("} {
		if i := strings.Index(msg, prefix); i >= 0 {
			rest := msg[i+len(prefix):]
			for j := 0; j < len(rest); j++ {
				if rest[j] < '0' || rest[j] > '9' {
					rest = rest[:j]
					break
				}
			}
			if n, err := strconv.Atoi(strings.TrimSpace(rest)); err == nil {
				return n
			}
		}
	}
	return 0
}
