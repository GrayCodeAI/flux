package client

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var clientTracer = otel.Tracer("graycode-router/client")

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

func (tp *TracingProvider) Chat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*GraycodeRouterResponse, error) {
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

func (tp *TracingProvider) StreamChat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*StreamResult, error) {
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

	// Wrap the events channel so the span ends when the stream finishes.
	origEvents := sr.Events
	wrappedEvents := make(chan GraycodeRouterStreamEvent, cap(origEvents))
	go func() {
		defer span.End()
		defer close(wrappedEvents)
		for evt := range origEvents {
			switch evt.Type {
			case "error":
				if evt.Warning != "" {
					// Non-fatal health diagnostic: record it without
					// failing the span (the stream still completes).
					span.SetAttributes(attribute.String("warning", evt.Warning))
				} else {
					span.SetStatus(codes.Error, evt.Error)
					span.SetAttributes(attribute.Bool("error", true))
				}
			case "usage":
				// Token usage is delivered on the "usage" event, not "done".
				if evt.Usage != nil {
					span.SetAttributes(
						attribute.Int("usage.prompt_tokens", evt.Usage.PromptTokens),
						attribute.Int("usage.completion_tokens", evt.Usage.CompletionTokens),
						attribute.Int("usage.total_tokens", evt.Usage.TotalTokens),
					)
				}
			case "done":
				span.SetStatus(codes.Ok, "")
			}
			// Respect cancellation on the send: if the consumer abandons the
			// stream, this goroutine must not block forever forwarding events
			// (which would leak the goroutine and keep the span open).
			select {
			case wrappedEvents <- evt:
			case <-ctx.Done():
				sr.Close()
				return
			}
		}
	}()

	return &StreamResult{
		Events:    wrappedEvents,
		RequestID: sr.RequestID,
	}, nil
}
