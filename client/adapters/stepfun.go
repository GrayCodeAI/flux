package adapters

import (
	"context"
	"strings"

	"github.com/GrayCodeAI/graycode-router/client/core"
)

// StepFunClient uses the OpenAI-compatible StepFun endpoint.
// Official docs: https://platform.stepfun.com/docs/en/api-reference
type StepFunClient struct {
	openAI *OpenAIClient
}

// NewStepFunClient builds a StepFun provider client.
// openAIBase is typically "https://api.stepfun.ai/v1".
func NewStepFunClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...core.ClientOption) *StepFunClient {
	openAIBase = strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	sfOpts := append(append([]core.ClientOption{}, opts...), core.WithProviderName("stepfun"))
	return &StepFunClient{
		openAI: NewOpenAIClient(apiKey, openAIBase, compat, sfOpts...),
	}
}

func (c *StepFunClient) Name() string { return "stepfun" }

func (c *StepFunClient) Chat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.GraycodeRouterResponse, error) {
	return c.openAI.Chat(ctx, messages, opts)
}

func (c *StepFunClient) StreamChat(ctx context.Context, messages []core.GraycodeRouterMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	return c.openAI.StreamChat(ctx, messages, opts)
}

func (c *StepFunClient) Ping(ctx context.Context) error {
	return c.openAI.Ping(ctx)
}

var _ core.Provider = (*StepFunClient)(nil)
