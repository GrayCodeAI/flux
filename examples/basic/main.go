// Example: basic chat with flux.
//
// Run:
//
//	ANTHROPIC_API_KEY=sk-... go run ./examples/basic/
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
		{Role: "user", Content: "What is 2 + 2?"},
	}

	resp, err := c.Chat(context.Background(), messages, core.ChatOptions{
		Model: "claude-sonnet-4-6",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "chat error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(resp.Content)
	fmt.Printf("Tokens: input=%d output=%d\n", resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
}
