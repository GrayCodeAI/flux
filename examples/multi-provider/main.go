// Example: multi-provider fallback chain.
//
// Tries Anthropic first, falls back to OpenAI, then Gemini.
//
// Run:
//
//	ANTHROPIC_API_KEY=sk-ant-... OPENAI_API_KEY=sk-... go run ./examples/multi-provider/
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/GrayCodeAI/eyrie/client"
)

func main() {
	primary := client.Client(&client.EyrieConfig{
		Provider: "anthropic",
	})
	secondary := client.Client(&client.EyrieConfig{
		Provider: "openai",
	})

	messages := []client.EyrieMessage{
		{Role: "user", Content: "Explain what a fallback chain is in one sentence."},
	}

	// Try primary first, fall back to secondary on failure.
	resp, err := primary.Chat(context.Background(), messages, client.ChatOptions{
		Model: "claude-sonnet-4-6",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "primary failed, trying secondary: %v\n", err)
		resp, err = secondary.Chat(context.Background(), messages, client.ChatOptions{
			Model: "gpt-4o",
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "all providers failed: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println(resp.Content)
}
