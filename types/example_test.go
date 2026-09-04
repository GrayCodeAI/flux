package types_test

import (
	"fmt"

	"github.com/GrayCodeAI/graycode-router/types"
)

func ExampleCreateUserMessage() {
	msg := types.CreateUserMessage("Hello, how can I help you?")
	fmt.Printf("Role: %s\n", msg.Role)
	fmt.Printf("Content: %s\n", msg.Content)
	// Output:
	// Role: user
	// Content: Hello, how can I help you?
}

func ExampleCreateAssistantMessage() {
	msg := types.CreateAssistantMessage("I can help with that!")
	fmt.Printf("Role: %s\n", msg.Role)
	// Output:
	// Role: assistant
}

func ExampleIsTransient() {
	// Transient errors are worth retrying
	err := &types.TransientError{StatusCode: 429, Message: "rate limited"}
	fmt.Printf("Is transient: %v\n", types.IsTransient(err))
	// Output:
	// Is transient: true
}

func ExampleClassifyError() {
	// Retriable status codes get TransientError
	err := types.ClassifyError(429, "rate limited")
	if te, ok := err.(*types.TransientError); ok {
		fmt.Printf("Transient: HTTP %d\n", te.StatusCode)
	}

	// Non-retriable codes get APIError
	err = types.ClassifyError(400, "bad request")
	if apiErr, ok := err.(*types.APIError); ok {
		fmt.Printf("API Error: HTTP %d\n", apiErr.Status)
	}
	// Output:
	// Transient: HTTP 429
	// API Error: HTTP 400
}

func ExampleExtractHTTPStatus() {
	err := &types.TransientError{StatusCode: 529, Message: "overloaded"}
	code, ok := types.ExtractHTTPStatus(err)
	fmt.Printf("Status: %d, Found: %v\n", code, ok)
	// Output:
	// Status: 529, Found: true
}
