package credentials

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// envFileMigrationMarkerPath is the on-disk marker that env-file import ran
// once. The marker file name is stable so upgraded installs do not re-import.
func envFileMigrationMarkerPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hawk", ".legacy-env-migrated")
}

func envFileMigrationDone() bool {
	_, err := os.Stat(envFileMigrationMarkerPath())
	return err == nil
}

func markEnvFileMigrationDone() {
	_ = os.MkdirAll(filepath.Dir(envFileMigrationMarkerPath()), 0o700)
	_ = os.WriteFile(envFileMigrationMarkerPath(), []byte("ok\n"), 0o600)
}

// MigrateEnvFileCredentials imports API keys from plaintext credential files
// (~/.hawk/env, ~/.hawk/.env) into the OS secret store and removes them.
func MigrateEnvFileCredentials(ctx context.Context) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if envFileMigrationDone() {
		return 0, nil
	}
	total := 0
	for _, path := range []string{hawkEnvPath(), hawkDotEnvPath()} {
		n, err := migrateEnvFileAt(ctx, path)
		if err != nil && !os.IsNotExist(err) {
			return total, err
		}
		total += n
	}
	markEnvFileMigrationDone()
	return total, nil
}

func migrateEnvFileAt(ctx context.Context, path string) (int, error) {
	secrets, err := readEnvFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if len(secrets) == 0 {
		_ = os.Remove(path)
		return 0, nil
	}
	cs, ok := DefaultStore().(*CombinedStore)
	if !ok || cs.Keychain == nil {
		return 0, ErrKeychainUnavailable
	}
	migrated := 0
	for _, envKey := range discoveryEnvKeys(ctx) {
		secret := strings.TrimSpace(secrets[envKey])
		if secret == "" {
			continue
		}
		account := AccountForEnv(envKey)
		if existing, err := cs.Keychain.Get(ctx, account); err == nil && strings.TrimSpace(existing) != "" {
			migrated++
			continue
		}
		if err := cs.Keychain.Set(ctx, account, secret); err != nil {
			continue
		}
		migrated++
	}
	if migrated > 0 || len(secrets) > 0 {
		_ = os.Remove(path)
	}
	return migrated, nil
}

func hawkEnvPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hawk", "env")
}

func hawkDotEnvPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hawk", ".env")
}

func readEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is built from os.UserHomeDir() in hawkDotEnvPath, not untrusted input
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		// Strip surrounding quotes (single or double) from values.
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		if key != "" && val != "" {
			out[key] = val
		}
	}
	return out, nil
}
