package observability

import (
	"context"

	"github.com/GrayCodeAI/flux/llm"
	"github.com/GrayCodeAI/flux/provider/core"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var clientTracer = otel.Tracer("flux/client")

// TracingProvider wraps a Provider with OpenTelemetry spans for Chat and
// StreamChat calls. Use NewTracingProvider to create one.
type TracingProvider struct {
	inner Provider
}

// NewTracingProvider wraps the given provider with OTel tracing.
func NewTracingProvider(inner Provider) *TracingProvider {
	return &TracingProvider{inner: inner}
}

var _ Provider = (*TracingProvider)(nil)

func (tp *TracingProvider) Name() string { return tp.inner.Name() }

func (tp *TracingProvider) Ping(ctx context.Context) error {
	return tp.inner.Ping(ctx)
}

func (tp *TracingProvider) Chat(ctx context.Context, messages []FluxMessage, opts ChatOptions) (*FluxResponse, error) {
	ctx, span := clientTracer.Start(
		ctx, "provider.Chat",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("provider.name", tp.inner.Name()),
			attribute.String("model", opts.Model),
			attribute.Int("message_count", len(messages)),
		),
	)
	defer span.End()

	resp, err := tp.inner.Chat(ctx, messages, opts)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
		return nil, err
	}

	if resp != nil {
		span.SetAttributes(
			attribute.String("finish_reason", resp.FinishReason),
			attribute.String("request_id", resp.RequestID),
		)
		if resp.Usage != nil {
			span.SetAttributes(
				attribute.Int("usage.prompt_tokens", resp.Usage.PromptTokens),
				attribute.Int("usage.completion_tokens", resp.Usage.CompletionTokens),
				attribute.Int("usage.total_tokens", resp.Usage.TotalTokens),
			)
		}
	}

	span.SetStatus(codes.Ok, "")
	return resp, nil
}

func (tp *TracingProvider) StreamChat(ctx context.Context, messages []FluxMessage, opts ChatOptions) (*StreamResult, error) {
	ctx, span := clientTracer.Start(
		ctx, "provider.StreamChat",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("provider.name", tp.inner.Name()),
			attribute.String("model", opts.Model),
			attribute.Int("message_count", len(messages)),
		),
	)

	sr, err := tp.inner.StreamChat(ctx, messages, opts)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
		span.End()
		return nil, err
	}

	span.SetAttributes(attribute.String("request_id", sr.RequestID))

	streamCtx, cancel := context.WithCancel(ctx)
	coordinated := core.CoordinateStreamResult(streamCtx, sr)
	wrappedEvents := make(chan FluxStreamEvent, cap(coordinated.Events))
	go func() {
		defer span.End()
		defer close(wrappedEvents)
		defer coordinated.Close()
		terminal := false
		for evt := range coordinated.Events {
			switch evt.Type {
			case "error":
				if evt.Warning != "" {
					span.SetAttributes(attribute.String("warning", evt.Warning))
				} else {
					terminal = true
					span.SetStatus(codes.Error, evt.Error)
					span.SetAttributes(attribute.Bool("error", true))
				}
			case "usage":
				if evt.Usage != nil {
					span.SetAttributes(
						attribute.Int("usage.prompt_tokens", evt.Usage.PromptTokens),
						attribute.Int("usage.completion_tokens", evt.Usage.CompletionTokens),
						attribute.Int("usage.total_tokens", evt.Usage.TotalTokens),
					)
				}
			case "done":
				terminal = true
				span.SetStatus(codes.Ok, "")
			}
			select {
			case wrappedEvents <- evt:
			case <-streamCtx.Done():
				return
			}
		}
		if !terminal {
			if err := streamCtx.Err(); err != nil {
				span.SetStatus(codes.Error, err.Error())
			}
		}
	}()

	return llm.NewStreamResult(wrappedEvents, sr.RequestID, func() {
		cancel()
		coordinated.Close()
	}), nil
}
