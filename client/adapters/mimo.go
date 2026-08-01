package adapters

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog/xiaomi"
	"github.com/GrayCodeAI/eyrie/client/core"
)

// MiMoClient uses the OpenAI-compatible MiMo endpoint.
type MiMoClient struct {
	openAI     *OpenAIClient
	providerID string
}

// NewMiMoClient builds a MiMo provider client (payg or token_plan gateway).
func NewMiMoClient(apiKey, openAIBase string, compat *OpenAICompatConfig, providerID string, opts ...core.ClientOption) *MiMoClient {
	openAIBase = strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	mimoOpts := append(append([]core.ClientOption{}, opts...), core.WithMimoAuth(), core.WithProviderName(providerID))
	return &MiMoClient{
		openAI:     NewOpenAIClient(apiKey, openAIBase, compat, mimoOpts...),
		providerID: providerID,
	}
}

func (c *MiMoClient) Name() string { return c.openAI.Name() }

func (c *MiMoClient) Chat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.EyrieResponse, error) {
	return c.openAI.Chat(ctx, messages, opts)
}

func (c *MiMoClient) StreamChat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	return c.openAI.StreamChat(ctx, messages, opts)
}

func (c *MiMoClient) Ping(ctx context.Context) error {
	return c.openAI.Ping(ctx)
}

var _ core.Provider = (*MiMoClient)(nil)

// parseHTTPStatusFromError extracts an HTTP status code from an error message.
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

// mimoAuthHeaders sets MiMo-preferred authentication on outbound requests.
func mimoAuthHeaders(req *http.Request, apiKey string) {
	xiaomi.SetMimoRequestAuth(req, apiKey)
}

// ProviderID reports the configured MiMo gateway identity.
func (c *MiMoClient) ProviderID() string { return c.providerID }
