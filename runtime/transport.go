package runtime

import (
	"context"

	"github.com/GrayCodeAI/eyrie/client"
)

// ChatTransportOpts supplies host-side overrides while transport ownership
// moves into the runtime package.
type ChatTransportOpts struct {
	Selection SelectionOpts
}

// ChatTransport is the runtime-owned chat transport plan that host apps adapt
// into their local session/client abstractions.
type ChatTransport struct {
	Selection SelectionState
	Provider  client.Provider
}

// ResolveChatTransport resolves the effective selection and constructs the
// runtime-owned provider transport for both deployment-routed and direct
// provider execution.
func ResolveChatTransport(ctx context.Context, opts ChatTransportOpts) (ChatTransport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	selection := EffectiveSelection(ctx, opts.Selection)
	return resolveChatTransportSelection(ctx, selection)
}

// ResolveChatTransportFromSelection constructs a transport from an already
// resolved selection state, avoiding redundant provider/model discovery.
func ResolveChatTransportFromSelection(ctx context.Context, selection SelectionState) (ChatTransport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return resolveChatTransportSelection(ctx, selection)
}

func resolveChatTransportSelection(ctx context.Context, selection SelectionState) (ChatTransport, error) {
	transport := ChatTransport{Selection: selection}
	if !selection.DeploymentRouting {
		transport.Provider = directChatProvider(ctx, selection.Provider)
		return transport, nil
	}
	provider, err := ChatProvider(ctx)
	if err != nil {
		transport.Provider = directChatProvider(ctx, selection.Provider)
		if transport.Provider != nil {
			transport.Selection.DeploymentRouting = false
			return transport, nil
		}
		return transport, err
	}
	transport.Provider = provider
	return transport, nil
}

func directChatProvider(_ context.Context, primary string) client.Provider {
	primary = NormalizeProviderID(primary)
	if primary == "" {
		return nil
	}
	return client.NewLazyProvider(&client.EyrieConfig{Provider: primary})
}
