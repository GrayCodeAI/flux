package adapters

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/GrayCodeAI/graycode-router/catalog/xiaomi"
	"github.com/GrayCodeAI/graycode-router/client/core"
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

func (c *MiMoClient) Chat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.GraycodeRouterResponse, error) {
	return c.openAI.Chat(ctx, messages, opts)
}

func (c *MiMoClient) StreamChat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.StreamResult, error) {
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

// doWithMimoAuthRetry runs the HTTP request through core.DoWithRetry; on 401
// with MiMo api-key auth it rebuilds the request with the provider's Bearer
// headers (set by setRetryHeaders) and retries once. Shared by the OpenAI and
// Anthropic adapters, which differ only in the headers applied to the retry.
func doWithMimoAuthRetry(
	ctx context.Context,
	httpClient *http.Client,
	retry core.RetryConfig,
	logger *slog.Logger,
	useMimoAuth bool,
	req *http.Request,
	body []byte,
	setRetryHeaders func(*http.Request),
) (*http.Response, error) {
	resp, err := core.DoWithRetry(ctx, httpClient, req, retry, logger)
	if err != nil {
		return nil, err
	}
	if !useMimoAuth || resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}
	_ = resp.Body.Close()
	req2, err := http.NewRequestWithContext(ctx, req.Method, req.URL.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	setRetryHeaders(req2)
	if req.Header.Get("Accept") != "" {
		req2.Header.Set("Accept", req.Header.Get("Accept"))
	}
	req2.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	return core.DoWithRetry(ctx, httpClient, req2, retry, logger)
}

// ProviderID reports the configured MiMo gateway identity.
func (c *MiMoClient) ProviderID() string { return c.providerID }
