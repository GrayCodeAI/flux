package client

import "github.com/GrayCodeAI/eyrie/client/adapters"

// FreezeRegistry prevents further provider registrations.
func FreezeRegistry() { adapters.FreezeRegistry() }

// RegisterDynamicProvider adds a user-defined OpenAI-compatible provider at runtime.
func RegisterDynamicProvider(name, baseURL, envKey string) error {
	return adapters.RegisterDynamicProvider(name, baseURL, envKey)
}
