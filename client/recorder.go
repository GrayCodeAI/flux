package client

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// RecorderMode controls whether the recorder records new interactions or replays existing ones.
type RecorderMode string

const (
	// RecordModeRecord always records new interactions from the inner provider.
	RecordModeRecord RecorderMode = "record"
	// RecordModeReplay always replays from the cassette; never calls the inner provider.
	RecordModeReplay RecorderMode = "replay"
	// RecordModeAuto replays if the cassette file exists, otherwise records.
	RecordModeAuto RecorderMode = "auto"
)

// RecorderProvider wraps any Provider to record or replay LLM interactions.
// It implements the Provider interface and stores interactions in a Cassette.
type RecorderProvider struct {
	inner    Provider
	mode     RecorderMode
	cassette *Cassette
	path     string
	mu       sync.Mutex
	position int
	redactor func(string) string
}

// Compile-time check that RecorderProvider implements Provider.
var _ Provider = (*RecorderProvider)(nil)

// NewRecorderProvider creates a RecorderProvider wrapping inner.
// In auto mode, if the cassette file exists it loads and replays; otherwise it records.
// In record mode, a fresh cassette is created.
// In replay mode, the cassette must exist or an error is returned.
func NewRecorderProvider(inner Provider, cassettePath string, mode RecorderMode) (*RecorderProvider, error) {
	r := &RecorderProvider{
		inner: inner,
		path:  cassettePath,
	}

	switch mode {
	case RecordModeAuto:
		if _, err := os.Stat(cassettePath); err == nil {
			c, err := LoadCassette(cassettePath)
			if err != nil {
				return nil, fmt.Errorf("recorder: failed to load cassette in auto mode: %w", err)
			}
			r.cassette = c
			r.mode = RecordModeReplay
		} else {
			r.cassette = &Cassette{
				Name:       cassettePath,
				RecordedAt: time.Now(),
				Provider:   inner.Name(),
			}
			r.mode = RecordModeRecord
		}

	case RecordModeRecord:
		r.cassette = &Cassette{
			Name:       cassettePath,
			RecordedAt: time.Now(),
			Provider:   inner.Name(),
		}
		r.mode = RecordModeRecord

	case RecordModeReplay:
		c, err := LoadCassette(cassettePath)
		if err != nil {
			return nil, fmt.Errorf("recorder: cassette not found for replay: %w", err)
		}
		r.cassette = c
		r.mode = RecordModeReplay

	default:
		return nil, fmt.Errorf("recorder: unknown mode: %s", mode)
	}

	return r, nil
}

// Name returns the inner provider name suffixed with "/recorder".
func (r *RecorderProvider) Name() string {
	return r.inner.Name() + "/recorder"
}

// Ping delegates to the inner provider.
func (r *RecorderProvider) Ping(ctx context.Context) error {
	return r.inner.Ping(ctx)
}

// Chat either records a new interaction or replays a stored one.
func (r *RecorderProvider) Chat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*GraycodeRouterResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	hash := requestHash(messages, opts)

	if r.mode == RecordModeReplay {
		return r.replay(hash)
	}

	// Record mode: call the inner provider
	resp, err := r.inner.Chat(ctx, messages, opts)

	interaction := Interaction{
		Request: RecordedRequest{
			Messages: messages,
			Model:    opts.Model,
			System:   opts.System,
			Hash:     hash,
		},
	}

	if err != nil {
		interaction.Response = RecordedResponse{
			Error: err.Error(),
		}
	} else {
		interaction.Response = RecordedResponse{
			Content:      r.redact(resp.Content),
			ToolCalls:    resp.ToolCalls,
			Usage:        resp.Usage,
			FinishReason: resp.FinishReason,
		}
	}

	r.cassette.Interactions = append(r.cassette.Interactions, interaction)

	if err != nil {
		return nil, err
	}
	return resp, nil
}

// StreamChat either records a new streaming interaction or replays a stored one.
// In record mode, the real stream is drained and the accumulated response is saved,
// then a synthetic stream is created to return to the caller.
// In replay mode, a synthetic stream is created from the stored response.
func (r *RecorderProvider) StreamChat(ctx context.Context, messages []GraycodeRouterMessage, opts ChatOptions) (*StreamResult, error) {
	hash, replayResult, err := r.checkReplay(messages, opts)
	if err != nil {
		return nil, err
	}
	if replayResult != nil {
		return replayResult, nil
	}

	// Record mode: call the inner provider's StreamChat
	result, err := r.inner.StreamChat(ctx, messages, opts)
	if err != nil {
		r.mu.Lock()
		r.cassette.Interactions = append(r.cassette.Interactions, Interaction{
			Request: RecordedRequest{
				Messages: messages,
				Model:    opts.Model,
				System:   opts.System,
				Hash:     hash,
			},
			Response: RecordedResponse{
				Error: err.Error(),
			},
		})
		r.mu.Unlock()
		return nil, err
	}

	// Drain events from the real stream and accumulate the response
	return r.recordStream(ctx, result, messages, opts, hash), nil
}

// checkReplay checks if we're in replay mode and returns the replay result.
// Returns (hash, nil, nil) if not in replay mode.
func (r *RecorderProvider) checkReplay(messages []GraycodeRouterMessage, opts ChatOptions) (string, *StreamResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	hash := requestHash(messages, opts)
	if r.mode == RecordModeReplay {
		resp, err := r.replay(hash)
		if err != nil {
			return hash, nil, err
		}
		return hash, r.syntheticStream(context.Background(), resp), nil
	}
	return hash, nil, nil
}

// Save writes the cassette to its file path.
func (r *RecorderProvider) Save() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return SaveCassette(r.cassette, r.path)
}

// SetRedactor sets a function that redacts sensitive content before recording.
func (r *RecorderProvider) SetRedactor(fn func(string) string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.redactor = fn
}

// replay finds a matching interaction by hash, falling back to position-based lookup.
func (r *RecorderProvider) replay(hash string) (*GraycodeRouterResponse, error) {
	// First try to find by hash
	for _, interaction := range r.cassette.Interactions {
		if interaction.Request.Hash == hash {
			return r.interactionToResponse(interaction.Response)
		}
	}

	// Fall back to position-based lookup
	if r.position >= len(r.cassette.Interactions) {
		return nil, fmt.Errorf("recorder: no more interactions in cassette (position %d, total %d)", r.position, len(r.cassette.Interactions))
	}
	interaction := r.cassette.Interactions[r.position]
	r.position++
	return r.interactionToResponse(interaction.Response)
}

// interactionToResponse converts a RecordedResponse to an GraycodeRouterResponse.
func (r *RecorderProvider) interactionToResponse(resp RecordedResponse) (*GraycodeRouterResponse, error) {
	if resp.Error != "" {
		return nil, fmt.Errorf("recorder: replayed error: %s", resp.Error)
	}
	return &GraycodeRouterResponse{
		Content:      resp.Content,
		ToolCalls:    resp.ToolCalls,
		Usage:        resp.Usage,
		FinishReason: resp.FinishReason,
	}, nil
}

// syntheticStream creates a StreamResult that emits stored content as events.
func (r *RecorderProvider) syntheticStream(ctx context.Context, resp *GraycodeRouterResponse) *StreamResult {
	streamCtx, cancel := context.WithCancel(ctx)
	ch := make(chan GraycodeRouterStreamEvent, 10)

	go func() {
		defer close(ch)
		if resp.Content != "" {
			select {
			case ch <- GraycodeRouterStreamEvent{Type: "content", Content: resp.Content}:
			case <-streamCtx.Done():
				return
			}
		}
		for i := range resp.ToolCalls {
			tc := resp.ToolCalls[i]
			select {
			case ch <- GraycodeRouterStreamEvent{Type: "tool_call", ToolCall: &tc}:
			case <-streamCtx.Done():
				return
			}
		}
		if resp.Usage != nil {
			select {
			case ch <- GraycodeRouterStreamEvent{Type: "usage", Usage: resp.Usage}:
			case <-streamCtx.Done():
				return
			}
		}
		select {
		case ch <- GraycodeRouterStreamEvent{Type: "done", StopReason: resp.FinishReason}:
		case <-streamCtx.Done():
		}
	}()

	return NewStreamResult(ch, cancel)
}

// recordStream drains the real stream, accumulates data, saves the interaction, and
// returns a synthetic stream with the accumulated response.
func (r *RecorderProvider) recordStream(ctx context.Context, result *StreamResult, messages []GraycodeRouterMessage, opts ChatOptions, hash string) *StreamResult {
	streamCtx, cancel := context.WithCancel(ctx)
	ch := make(chan GraycodeRouterStreamEvent, 64)

	go func() {
		defer close(ch)
		defer result.Close()

		var content string
		var toolCalls []ToolCall
		var usage *GraycodeRouterUsage
		var finishReason string

		// Drain the real stream, forwarding events to the caller
		for evt := range result.Events {
			switch evt.Type {
			case "content":
				content += evt.Content
			case "tool_call":
				if evt.ToolCall != nil {
					toolCalls = append(toolCalls, *evt.ToolCall)
				}
			case "usage":
				if evt.Usage != nil {
					usage = evt.Usage
				}
			case "done":
				finishReason = evt.StopReason
			}

			// Forward the event to the caller
			select {
			case ch <- evt:
			case <-streamCtx.Done():
				result.Close()
				return
			}
		}

		// Save the accumulated interaction
		r.mu.Lock()
		interaction := Interaction{
			Request: RecordedRequest{
				Messages: messages,
				Model:    opts.Model,
				System:   opts.System,
				Hash:     hash,
			},
			Response: RecordedResponse{
				Content:      r.redact(content),
				ToolCalls:    toolCalls,
				Usage:        usage,
				FinishReason: finishReason,
			},
		}
		r.cassette.Interactions = append(r.cassette.Interactions, interaction)
		r.mu.Unlock()
	}()

	return NewStreamResult(ch, cancel)
}

// redact applies the redactor function if set.
func (r *RecorderProvider) redact(s string) string {
	if r.redactor != nil {
		return r.redactor(s)
	}
	return s
}
