package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// SchemaValidation holds configuration for structured output validation with retry.
type SchemaValidation struct {
	// Schema is the JSON schema to validate against.
	Schema map[string]interface{}
	// MaxRetries is the maximum number of retry attempts if validation fails.
	MaxRetries int
	// StrictMode enables strict schema validation requiring all fields.
	StrictMode bool
}

// StructuredOutputError represents a validation failure with details.
type StructuredOutputError struct {
	// Response is the raw response that failed validation.
	Response string
	// ValidationErr is the underlying validation error.
	ValidationErr error
	// Attempt is the attempt number that failed.
	Attempt int
}

func (e *StructuredOutputError) Error() string {
	return fmt.Sprintf("structured output validation failed (attempt %d): %v", e.Attempt, e.ValidationErr)
}

// ValidateStructuredOutput validates a JSON response against a schema.
// It checks that the response is valid JSON and that all required fields
// specified in the schema are present with correct types.
func ValidateStructuredOutput(response string, schema map[string]interface{}) error {
	// Parse the response as JSON
	var parsed interface{}
	if err := json.Unmarshal([]byte(response), &parsed); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	// Get the root type from schema
	schemaType, _ := schema["type"].(string)
	if schemaType == "" {
		schemaType = "object"
	}

	// Validate based on schema type
	return validateValue(parsed, schema, schemaType)
}

// validateValue recursively validates a value against a schema.
func validateValue(value interface{}, schema map[string]interface{}, schemaType string) error {
	switch schemaType {
	case "object":
		return validateObject(value, schema)
	case "array":
		return validateArray(value, schema)
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string, got %T", value)
		}
	case "number":
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("expected number, got %T", value)
		}
	case "integer":
		f, ok := value.(float64)
		if !ok {
			return fmt.Errorf("expected integer, got %T", value)
		}
		if f != float64(int(f)) {
			return fmt.Errorf("expected integer, got float %v", f)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected boolean, got %T", value)
		}
	case "null":
		if value != nil {
			return fmt.Errorf("expected null, got %T", value)
		}
	}
	return nil
}

// validateObject validates a JSON object against an object schema.
func validateObject(value interface{}, schema map[string]interface{}) error {
	obj, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("expected object, got %T", value)
	}

	// Check required fields
	required, _ := schema["required"].([]interface{})
	for _, req := range required {
		fieldName, ok := req.(string)
		if !ok {
			continue
		}
		if _, exists := obj[fieldName]; !exists {
			return fmt.Errorf("missing required field: %s", fieldName)
		}
	}

	// Validate properties
	properties, _ := schema["properties"].(map[string]interface{})
	for propName, propSchemaRaw := range properties {
		propSchema, ok := propSchemaRaw.(map[string]interface{})
		if !ok {
			continue
		}
		propValue, exists := obj[propName]
		if !exists {
			continue // Skip optional fields
		}
		propType, _ := propSchema["type"].(string)
		if err := validateValue(propValue, propSchema, propType); err != nil {
			return fmt.Errorf("field %q: %w", propName, err)
		}
	}

	return nil
}

// validateArray validates a JSON array against an array schema.
func validateArray(value interface{}, schema map[string]interface{}) error {
	arr, ok := value.([]interface{})
	if !ok {
		return fmt.Errorf("expected array, got %T", value)
	}

	// Validate items if schema specifies item type
	items, _ := schema["items"].(map[string]interface{})
	if items != nil {
		itemType, _ := items["type"].(string)
		for i, item := range arr {
			if err := validateValue(item, items, itemType); err != nil {
				return fmt.Errorf("item[%d]: %w", i, err)
			}
		}
	}

	// Check minItems
	if minItems, ok := schema["minimum"].(float64); ok {
		if float64(len(arr)) < minItems {
			return fmt.Errorf("array length %d below minimum %v", len(arr), minItems)
		}
	}

	return nil
}

// BuildStructuredPrompt adds JSON schema instructions to the message system prompt.
// It prepends schema requirements to ensure the LLM outputs valid JSON matching the schema.
func BuildStructuredPrompt(messages []EyrieMessage, schema map[string]interface{}) []EyrieMessage {
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		// Fallback: return messages unchanged if schema can't be marshaled
		return messages
	}

	schemaInstruction := fmt.Sprintf(`You must respond with valid JSON that matches the following schema:
%s

Important:
- Your response MUST be valid JSON
- Do not include any text before or after the JSON
- Do not use markdown code blocks
- All required fields must be present`, string(schemaJSON))

	// Find system message and prepend to it, or create new one
	result := make([]EyrieMessage, 0, len(messages)+1)
	systemFound := false

	for _, msg := range messages {
		if msg.Role == "system" && !systemFound {
			// Prepend schema instruction to existing system message
			newMsg := msg
			if newMsg.Content != "" {
				newMsg.Content = schemaInstruction + "\n\n" + newMsg.Content
			} else {
				newMsg.Content = schemaInstruction
			}
			result = append(result, newMsg)
			systemFound = true
		} else {
			result = append(result, msg)
		}
	}

	if !systemFound {
		// Insert system message at the beginning
		systemMsg := EyrieMessage{Role: "system", Content: schemaInstruction}
		result = append([]EyrieMessage{systemMsg}, result...)
	}

	return result
}

// WithStructuredOutput returns a ClientOption for structured JSON output.
//
// The option itself is inert: neither adapter is mutated at construction
// time. Anthropic uses the prefill technique and OpenAI sets response_format,
// both handled per-call in ChatWithStructuredOutput (which receives the
// schema through its SchemaValidation parameter — the fields this option
// previously carried were never read). Kept for API compatibility.
func WithStructuredOutput(schema map[string]interface{}, maxRetries int) ClientOption {
	return ClientOption{}
}

// ChatWithStructuredOutput sends a chat request with structured output validation.
// If the response doesn't match the schema, it retries with error feedback.
func (c *EyrieClient) ChatWithStructuredOutput(ctx context.Context, messages []EyrieMessage, opts ChatOptions, validation SchemaValidation) (*EyrieResponse, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("eyrie: messages must not be empty")
	}

	provider := opts.Provider
	if provider == "" {
		provider = c.defaultProvider
	}

	p, err := c.getOrCreateProvider(provider)
	if err != nil {
		return nil, err
	}

	if opts.Model == "" {
		opts.Model = ResolveDefaultModel(provider)
	}

	maxRetries := validation.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	// Build prompt with schema instructions
	structuredMessages := BuildStructuredPrompt(messages, validation.Schema)

	// Configure response format based on provider
	switch provider {
	case "openai", "":
		schemaJSON, _ := json.Marshal(validation.Schema)
		opts.ResponseFormat = &ResponseFormat{
			Type:   "json_schema",
			Schema: string(schemaJSON),
		}
	case "anthropic":
		// For Anthropic, we'll use prefill technique by adding assistant message
		structuredMessages = addAnthropicPrefill(structuredMessages)
	}

	var lastResp *EyrieResponse
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// If retrying, add error feedback to messages
		currentMessages := structuredMessages
		if attempt > 0 && lastResp != nil {
			currentMessages = addRetryFeedback(structuredMessages, lastResp.Content, lastErr, validation.Schema)
			// Re-add prefill for Anthropic
			if provider == "anthropic" {
				currentMessages = addAnthropicPrefill(currentMessages)
			}
		}

		resp, err := p.Chat(ctx, currentMessages, opts)
		if err != nil {
			lastErr = err
			lastResp = nil
			continue
		}

		// Extract JSON from response (handle potential markdown code blocks)
		jsonContent := extractJSON(resp.Content)

		// Validate the response
		validationErr := ValidateStructuredOutput(jsonContent, validation.Schema)
		if validationErr == nil {
			// Success - return the cleaned JSON content
			resp.Content = jsonContent
			return resp, nil
		}

		lastErr = validationErr
		lastResp = resp
	}

	// All retries exhausted
	if lastResp != nil {
		return nil, &StructuredOutputError{
			Response:      lastResp.Content,
			ValidationErr: lastErr,
			Attempt:       maxRetries + 1,
		}
	}

	return nil, fmt.Errorf("structured output validation failed after %d attempts: %w", maxRetries+1, lastErr)
}

// addAnthropicPrefill adds an assistant message prefill for Anthropic to encourage JSON output.
func addAnthropicPrefill(messages []EyrieMessage) []EyrieMessage {
	// Add assistant message with opening brace to force JSON output
	prefillMsg := EyrieMessage{
		Role:    "assistant",
		Content: "```json\n{",
	}
	return append(messages, prefillMsg)
}

// addRetryFeedback adds error feedback to messages for retry attempts.
func addRetryFeedback(messages []EyrieMessage, lastResponse string, validationErr error, schema map[string]interface{}) []EyrieMessage {
	schemaJSON, _ := json.MarshalIndent(schema, "", "  ")

	feedback := fmt.Sprintf(`Your previous response was invalid. Error: %v

Your response was:
%s

Please fix the error and respond with valid JSON matching this schema:
%s

Remember:
- Include ALL required fields
- Use correct data types
- Respond with ONLY the JSON object, no additional text`,
		validationErr,
		lastResponse,
		string(schemaJSON))

	// Add as user message
	feedbackMsg := EyrieMessage{
		Role:    "user",
		Content: feedback,
	}

	return append(messages, feedbackMsg)
}

// extractJSON extracts JSON from a response, handling markdown code blocks.
func extractJSON(content string) string {
	content = strings.TrimSpace(content)

	// Check for markdown code block
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}

	// Try to find JSON object or array
	startIdx := strings.Index(content, "{")
	if startIdx == -1 {
		startIdx = strings.Index(content, "[")
	}
	if startIdx > 0 {
		content = content[startIdx:]
	}

	// Find the last closing brace or bracket
	endIdx := strings.LastIndex(content, "}")
	if endIdx == -1 {
		endIdx = strings.LastIndex(content, "]")
	}
	if endIdx >= 0 && endIdx < len(content)-1 {
		content = content[:endIdx+1]
	}

	return content
}

// ChatOption extension for structured output - we need to add fields to ChatOption
// This is handled by the apply functions storing the schema
