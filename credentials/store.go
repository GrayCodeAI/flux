package credentials

import (
	"context"
	"strings"
	"sync"

	"github.com/GrayCodeAI/eyrie/catalog"
)

// ServiceName is the OS secret-store service under which credentials are
// filed. It defaults to "eyrie" (host-neutral). Embedders must call
// SetServiceName before any credential read or write to store secrets under
// their own service; changing it later orphans previously stored secrets.
var ServiceName = "eyrie"

// SetServiceName overrides the secret-store service name. Call it once at
// startup, before the first credential read or write; changing it later
// orphans previously stored secrets.
func SetServiceName(name string) {
	name = strings.TrimSpace(name)
	if name != "" {
		ServiceName = name
	}
}

// Store persists provider API secrets outside provider.json.
type Store interface {
	Set(ctx context.Context, account, secret string) error
	Get(ctx context.Context, account string) (string, error)
	Delete(ctx context.Context, account string) error
}

var (
	defaultStore   Store
	defaultStoreMu sync.RWMutex
)

// DefaultStore returns the process-wide credential store (OS secret service).
func DefaultStore() Store {
	defaultStoreMu.RLock()
	s := defaultStore
	defaultStoreMu.RUnlock()
	if s != nil {
		return s
	}
	defaultStoreMu.Lock()
	defer defaultStoreMu.Unlock()
	if defaultStore == nil {
		defaultStore = NewCombinedStore()
	}
	return defaultStore
}

// SetDefaultStore replaces the process-wide store (tests).
func SetDefaultStore(s Store) {
	defaultStoreMu.Lock()
	defaultStore = s
	defaultStoreMu.Unlock()
}

// AccountForEnv maps a standard env var to a stable keychain account name.
func AccountForEnv(envKey string) string {
	return strings.ToLower(strings.TrimSpace(envKey))
}

// EnvForAccount is a best-effort reverse map for loading into process env.
// Accounts are stored as lowercased env names (see AccountForEnv), so the
// uppercase transform recovers the env key for every registered provider.
func EnvForAccount(account string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(account), "-", "_"))
}

// APIKeysMap returns env-keyed secrets for catalog discovery.
func APIKeysMap(ctx context.Context, store Store) map[string]string {
	if store == nil {
		store = DefaultStore()
	}
	out := map[string]string{}
	for _, envKey := range discoveryEnvKeys(ctx) {
		secret, err := store.Get(ctx, AccountForEnv(envKey))
		if err != nil || strings.TrimSpace(secret) == "" {
			continue
		}
		out[envKey] = secret
	}
	return out
}

func discoveryEnvKeys(ctx context.Context) []string {
	if ctx == nil {
		ctx = context.Background()
	}
	if keys := catalog.DiscoveryEnvKeyNames(ctx); len(keys) > 0 {
		return keys
	}
	return []string{
		"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY",
		"GEMINI_API_KEY", "XAI_API_KEY",
		"MOONSHOT_API_KEY", "XIAOMI_MIMO_PAYG_API_KEY", "XIAOMI_MIMO_TOKEN_PLAN_API_KEY",
		"CANOPYWAVE_API_KEY", "OPENCODEGO_API_KEY",
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
		"VERTEX_ACCESS_TOKEN", "GOOGLE_OAUTH_ACCESS_TOKEN",
	}
}
