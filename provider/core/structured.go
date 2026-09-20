package core

// SchemaValidation describes the JSON schema and retry policy for a structured
// output request. The media feature owns validation and prompting; core owns
// this small cross-feature contract so other features can request it without
// importing media.
type SchemaValidation struct {
	Schema     map[string]interface{}
	MaxRetries int
	StrictMode bool
}
