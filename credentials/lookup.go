package credentials

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// LookupSecret returns an API key from the OS secret store.
// It does not read the process environment — use for strict isolation from agents and shell dumps.
//
// On "not found" the function returns ("", nil) and logs at debug level — callers
// commonly probe for keys that may legitimately be absent. Real backend errors
// (keychain locked, permission denied, transport failure) are logged at warn.
func LookupSecret(ctx context.Context, envKey string) string {
	if ctx == nil {
		ctx = context.Background()
	}
	envKey = strings.TrimSpace(envKey)
	if envKey == "" {
		return ""
	}
	secret, err := lookupSecretAccount(ctx, AccountForEnv(envKey))
	if err != nil {
		if isNotFound(err) {
			slog.Debug("no secret stored", "env_key", envKey)
			return ""
		}
		slog.Warn("secret lookup failed", "env_key", envKey, "error", err)
		return ""
	}
	return strings.TrimSpace(secret)
}

func lookupSecretAccount(ctx context.Context, account string) (string, error) {
	secret, err := DefaultStore().Get(ctx, account)
	return secret, err
}

// HasSecret reports whether the store has a non-empty secret for an env key name.
// It is intentionally silent — it is a boolean predicate used in capability
// checks that iterate the full provider catalog at startup, and any logging
// here produced dozens of WARN lines on first run with no credentials stored.
func HasSecret(ctx context.Context, envKey string) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	envKey = strings.TrimSpace(envKey)
	if envKey == "" {
		return false
	}
	secret, err := lookupSecretAccount(ctx, AccountForEnv(envKey))
	if err != nil {
		return false
	}
	return strings.TrimSpace(secret) != ""
}

// isNotFound reports whether the store error represents a missing key
// (as opposed to a real failure like a locked keychain or transport error).
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNotFound) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "no such") ||
		strings.Contains(msg, "does not exist")
}

// DeleteSecret removes a stored secret for an env key name.
func DeleteSecret(ctx context.Context, envKey string) error {
	envKey = strings.TrimSpace(envKey)
	if envKey == "" {
		return fmt.Errorf("credentials: env key required")
	}
	return DefaultStore().Delete(ctx, AccountForEnv(envKey))
}

// ScrubProcessEnv removes API key env vars from the process (shell inheritance / legacy Setenv).
func ScrubProcessEnv(envKeys []string) {
	for _, k := range envKeys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		_ = os.Unsetenv(k)
	}
}
