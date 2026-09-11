package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/GrayCodeAI/eyrie/client"
	"github.com/GrayCodeAI/eyrie/storage"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("eyrie/conversation")

type Engine struct {
	store    storage.Store
	provider client.Provider
}

func New(store storage.Store, provider client.Provider) *Engine {
	return &Engine{store: store, provider: provider}
}

type PromptOpts struct {
	Model        string
	SystemPrompt string
	Tools        []client.EyrieTool
	MaxTokens    int
	Temperature  *float64
}

type Event struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
	NodeID  string `json:"node_id,omitempty"`
	Error   string `json:"error,omitempty"`
}

const (
	EventDelta     = "delta"
	EventDone      = "done"
	EventNodeSaved = "node_saved"
	EventError     = "error"
)

func (e *Engine) Prompt(ctx context.Context, message string, opts PromptOpts) (<-chan Event, error) {
	ctx, span := tracer.Start(
		ctx, "conversation.Prompt",
		trace.WithAttributes(
			attribute.String("model", opts.Model),
			attribute.Int("message_length", len(message)),
		),
	)

	rootID := uuid.New().String()
	rootNode := &storage.Node{
		ID:           rootID,
		RootID:       rootID,
		Sequence:     0,
		NodeType:     storage.NodeTypeUser,
		Content:      message,
		Status:       "completed",
		Title:        generateTitle(message),
		SystemPrompt: opts.SystemPrompt,
		CreatedAt:    time.Now(),
	}
	if err := e.store.CreateNode(ctx, rootNode); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return nil, fmt.Errorf("conversation: create root: %w", err)
	}

	messages := []client.EyrieMessage{{Role: "user", Content: message}}
	ch, err := e.streamAndSave(ctx, rootNode, messages, opts)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return nil, err
	}
	span.SetStatus(codes.Ok, "")
	span.End()
	return ch, nil
}

func (e *Engine) PromptFrom(ctx context.Context, parentNodeID, message string, opts PromptOpts) (<-chan Event, error) {
	ctx, span := tracer.Start(
		ctx, "conversation.PromptFrom",
		trace.WithAttributes(
			attribute.String("parent_node_id", parentNodeID),
			attribute.String("model", opts.Model),
			attribute.Int("message_length", len(message)),
		),
	)

	ancestors, err := e.store.GetAncestors(ctx, parentNodeID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return nil, fmt.Errorf("conversation: get ancestors: %w", err)
	}
	if len(ancestors) == 0 {
		span.SetStatus(codes.Error, "node not found")
		span.End()
		return nil, fmt.Errorf("conversation: node not found: %s", parentNodeID)
	}

	root := ancestors[0]
	lastNode := ancestors[len(ancestors)-1]

	if opts.Model == "" {
		opts.Model = root.Model
	}
	if opts.SystemPrompt == "" {
		opts.SystemPrompt = root.SystemPrompt
	}

	userNode := &storage.Node{
		ID:       uuid.New().String(),
		ParentID: parentNodeID,
		RootID:   root.ID,
		Sequence: lastNode.Sequence + 1,
		NodeType: storage.NodeTypeUser,
		Content:  message,
		Status:   "completed",
	}
	if err := e.store.CreateNode(ctx, userNode); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return nil, fmt.Errorf("conversation: create user node: %w", err)
	}

	if resultIDs := extractToolResultIDsFromContent(message); len(resultIDs) > 0 {
		_ = e.store.IndexToolIDs(ctx, userNode.ID, resultIDs, "result")
	}

	ancestorIDs := make([]string, len(ancestors)+1)
	for i, a := range ancestors {
		ancestorIDs[i] = a.ID
	}
	ancestorIDs[len(ancestors)] = userNode.ID

	orphans, err := e.store.GetOrphanedToolUses(ctx, ancestorIDs)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return nil, fmt.Errorf("conversation: check orphaned tool uses: %w", err)
	}
	if len(orphans) > 0 {
		ancestors = injectSyntheticToolResults(ancestors, orphans)
	}

	messages := buildMessages(ancestors)
	messages = append(messages, client.EyrieMessage{Role: "user", Content: message})

	ch, err := e.streamAndSave(ctx, userNode, messages, opts)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return nil, err
	}
	span.SetStatus(codes.Ok, "")
	span.End()
	return ch, nil
}

func (e *Engine) ResolveNode(ctx context.Context, ref string) (*storage.Node, error) {
	node, err := e.store.GetNode(ctx, ref)
	if err == nil {
		return node, nil
	}
	node, err = e.store.GetNodeByPrefix(ctx, ref)
	if err == nil {
		return node, nil
	}
	return e.store.GetNodeByAlias(ctx, ref)
}

func (e *Engine) ListConversations(ctx context.Context) ([]*storage.Node, error) {
	return e.store.ListRootNodes(ctx)
}

func (e *Engine) GetSubtree(ctx context.Context, id string) ([]*storage.Node, error) {
	return e.store.GetSubtree(ctx, id)
}

func (e *Engine) DeleteNode(ctx context.Context, id string) error {
	return e.store.DeleteNode(ctx, id)
}

const defaultGroupBudgetMultiplier = 4

func (e *Engine) streamAndSave(ctx context.Context, parentNode *storage.Node, messages []client.EyrieMessage, opts PromptOpts) (<-chan Event, error) {
	if e.provider == nil {
		return nil, fmt.Errorf("conversation: engine has no provider")
	}
	_, span := tracer.Start(
		ctx, "conversation.streamAndSave",
		trace.WithAttributes(
			attribute.String("model", opts.Model),
			attribute.String("provider", e.provider.Name()),
			attribute.Int("message_count", len(messages)),
		),
	)

	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	chatOpts := client.ChatOptions{
		Model:       opts.Model,
		System:      opts.SystemPrompt,
		MaxTokens:   maxTokens,
		Temperature: opts.Temperature,
		Tools:       opts.Tools,
	}

	sr, err := e.provider.StreamChat(ctx, messages, chatOpts)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return nil, fmt.Errorf("conversation: stream: %w", err)
	}

	groupBudget := maxTokens * defaultGroupBudgetMultiplier
	events := make(chan Event, 100)

	go func() {
		defer close(events)
		defer span.End()

		var (
			groupID         string
			accumulatedText string
			cumulativeOut   int
			currentParent   = parentNode
			currentSR       = sr
			currentStream   = sr.Events
		)

		// Ensure the last StreamResult is always closed on exit. The
		// closure captures currentSR by reference, so it closes whatever
		// stream is active when the goroutine returns — including the
		// initial stream on early exits and the final continuation stream
		// on normal completion. Previous streams are closed explicitly
		// during the continuation loop (currentSR.Close() before reassign).
		defer func() {
			if currentSR != nil {
				currentSR.Close()
			}
		}()

		for {
			var fullTextBuilder strings.Builder
			var usage *client.EyrieUsage
			var stopReason string
			start := time.Now()

			for evt := range currentStream {
				switch evt.Type {
				case "content":
					fullTextBuilder.WriteString(evt.Content)
					select {
					case events <- Event{Type: EventDelta, Content: evt.Content}:
					case <-ctx.Done():
						return
					}
				case "done":
					stopReason = evt.StopReason
					usage = evt.Usage
				case "error":
					select {
					case events <- Event{Type: EventError, Error: evt.Error}:
					case <-ctx.Done():
					}
					return
				}
			}

			fullText := fullTextBuilder.String()

			if fullText == "" && stopReason == "" {
				if accumulatedText != "" {
					select {
					case events <- Event{Type: EventNodeSaved, NodeID: currentParent.ID}:
					case <-ctx.Done():
					}
				}
				return
			}

			accumulatedText += fullText
			if usage != nil {
				cumulativeOut += usage.CompletionTokens
			}

			shouldContinue := stopReason == "max_tokens" && cumulativeOut < groupBudget
			if shouldContinue && groupID == "" {
				groupID = uuid.New().String()
			}

			assistantNode := &storage.Node{
				ID:            uuid.New().String(),
				ParentID:      currentParent.ID,
				RootID:        currentParent.RootID,
				Sequence:      currentParent.Sequence + 1,
				NodeType:      storage.NodeTypeAssistant,
				Content:       accumulatedText,
				OutputGroupID: groupID,
				Model:         opts.Model,
				Provider:      e.provider.Name(),
				StopReason:    stopReason,
				LatencyMs:     int(time.Since(start).Milliseconds()),
				Status:        "completed",
				CreatedAt:     time.Now(),
			}
			if usage != nil {
				assistantNode.TokensIn = usage.PromptTokens
				assistantNode.TokensOut = usage.CompletionTokens
				assistantNode.TokensCacheRead = usage.CacheReadTokens
				assistantNode.TokensCacheCreation = usage.CacheCreationTokens
			}
			if err := e.store.CreateNode(ctx, assistantNode); err != nil {
				select {
				case events <- Event{Type: EventError, Error: err.Error()}:
				case <-ctx.Done():
				}
				return
			}
			if toolUseIDs := extractToolUseIDsFromContent(assistantNode.Content); len(toolUseIDs) > 0 {
				_ = e.store.IndexToolIDs(ctx, assistantNode.ID, toolUseIDs, "use")
			}

			if !shouldContinue {
				select {
				case events <- Event{Type: EventDone, NodeID: assistantNode.ID}:
				case <-ctx.Done():
				}
				return
			}

			currentParent = assistantNode

			contMessages := make([]client.EyrieMessage, len(messages), len(messages)+1)
			copy(contMessages, messages)
			contMessages = append(contMessages, client.EyrieMessage{Role: "assistant", Content: accumulatedText})

			contSR, contErr := e.provider.StreamChat(ctx, contMessages, chatOpts)
			if contErr != nil {
				// Surface the continuation failure explicitly. The prior
				// assistant node is already persisted as "completed" with the
				// truncated text; emit an error event tagged to that node so
				// callers can distinguish a truncated/continuation-failed
				// response from a clean completion. The done event still
				// follows so the stream terminates normally.
				select {
				case events <- Event{Type: EventError, Error: contErr.Error(), NodeID: assistantNode.ID}:
				case <-ctx.Done():
				}
				select {
				case events <- Event{Type: EventDone, NodeID: assistantNode.ID}:
				case <-ctx.Done():
				}
				return
			}
			// Close the previous stream before switching to the continuation.
			currentSR.Close()
			currentSR = contSR
			currentStream = contSR.Events
		}
	}()

	return events, nil
}

func buildMessages(nodes []*storage.Node) []client.EyrieMessage {
	seen := map[string]bool{}
	var raw []struct {
		role string
		node *storage.Node
	}
	for _, n := range nodes {
		if n.OutputGroupID != "" {
			if seen[n.OutputGroupID] {
				continue
			}
			seen[n.OutputGroupID] = true
		}
		var role string
		switch n.NodeType {
		case storage.NodeTypeToolCall:
			role = "tool_call"
		case storage.NodeTypeToolResult:
			role = "tool_result"
		case storage.NodeTypeUser:
			role = "user"
		case storage.NodeTypeAssistant:
			role = "assistant"
		case storage.NodeTypeSystem:
			role = "system"
		default:
			continue
		}
		raw = append(raw, struct {
			role string
			node *storage.Node
		}{role, n})
	}

	var messages []client.EyrieMessage
	for _, r := range raw {
		switch r.role {
		case "tool_call":
			msg := client.EyrieMessage{Role: "assistant", Content: r.node.Content}
			if len(r.node.Metadata) > 0 {
				var meta struct {
					ToolID   string                 `json:"tool_id"`
					ToolName string                 `json:"tool_name"`
					Input    map[string]interface{} `json:"input"`
				}
				if err := json.Unmarshal(r.node.Metadata, &meta); err == nil {
					name := meta.ToolName
					if name == "" {
						name = meta.ToolID
					}
					if name != "" {
						msg.ToolUse = append(msg.ToolUse, client.ToolCall{
							ID:        meta.ToolID,
							Name:      name,
							Arguments: meta.Input,
						})
					}
				}
			}
			messages = append(messages, msg)
		case "tool_result":
			tr := client.ToolResult{Content: r.node.Content}
			if len(r.node.Metadata) > 0 {
				var meta struct {
					ToolUseID string `json:"tool_use_id"`
					IsError   bool   `json:"is_error"`
				}
				if err := json.Unmarshal(r.node.Metadata, &meta); err == nil && meta.ToolUseID != "" {
					tr.ToolUseID = meta.ToolUseID
					tr.IsError = meta.IsError
				}
			}
			// If the last message is a user message with tool results, append
			// this result to it rather than creating a separate message.
			if n := len(messages); n > 0 && messages[n-1].Role == "user" && len(messages[n-1].ToolResults) > 0 {
				messages[n-1].ToolResults = append(messages[n-1].ToolResults, tr)
			} else {
				messages = append(messages, client.EyrieMessage{
					Role:        "user",
					ToolResults: []client.ToolResult{tr},
				})
			}
		default:
			messages = append(messages, client.EyrieMessage{
				Role:    r.role,
				Content: r.node.Content,
			})
		}
	}
	return messages
}

func generateTitle(msg string) string {
	msg = strings.TrimSpace(msg)
	runes := []rune(msg)
	if len(runes) > 50 {
		return string(runes[:50]) + "..."
	}
	return msg
}
