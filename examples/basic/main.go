// Example: basic chat with eyrie.
//
// Run:
//
//	ANTHROPIC_API_KEY=sk-... go run ./examples/basic/
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/GrayCodeAI/eyrie/client"
)

func main() {
	c := client.Client(&client.EyrieConfig{
		Provider: client.DetectProvider(),
	})

	messages := []client.EyrieMessage{
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
