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

	"github.com/GrayCodeAI/flux/client"
)

func main() {
	c := client.Client(&client.FluxConfig{
		Provider: client.DetectProvider(),
	})

	messages := []client.FluxMessage{
		{Role: "user", Content: "What is 2 + 2?"},
	}

	resp, err := c.Chat(context.Background(), messages, client.ChatOptions{
		Model: "claude-sonnet-4-6",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "chat error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(resp.Content)
	fmt.Printf("Tokens: input=%d output=%d\n", resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
}
