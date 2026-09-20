// Example: streaming chat with auto-continuation.
//
// Run:
//
//	ANTHROPIC_API_KEY=sk-... go run ./examples/streaming/
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/GrayCodeAI/flux/provider"
	"github.com/GrayCodeAI/flux/provider/core"
)

func main() {
	c := provider.Client(&core.FluxConfig{
		Provider: provider.DetectProvider(),
	})

	messages := []core.FluxMessage{
		{Role: "user", Content: "Write a short poem about programming."},
	}

	sr, err := c.StreamChat(context.Background(), messages, core.ChatOptions{
		Model: "claude-sonnet-4-6",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "stream error: %v\n", err)
		os.Exit(1)
	}
	defer sr.Close()

	for evt := range sr.Events {
		switch evt.Type {
		case "content":
			fmt.Print(evt.Content)
		case "tool_call":
			if evt.ToolCall != nil {
				fmt.Printf("\n[Tool call: %s]\n", evt.ToolCall.Name)
			}
		case "done":
			fmt.Println()
		case "error":
			fmt.Fprintf(os.Stderr, "\nstream error: %s\n", evt.Error)
		}
	}
}
