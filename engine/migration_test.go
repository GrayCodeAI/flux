package engine

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/GrayCodeAI/eyrie/config"
)

// TestMigrateConfigDirHonorsEYRIE_CONFIG_DIR verifies H1 fix: when
// EYRIE_CONFIG_DIR is set, the migration copies from
// <UserConfigDir>/hawk/ → <EYRIE_CONFIG_DIR>/, not the default path.
func TestMigrateConfigDirHonorsEYRIE_CONFIG_DIR(t *testing.T) {
	// Fresh state for this test.
	migrateProviderConfigDirOnce = sync.Once{}

	userDir := t.TempDir()
	customDir := t.TempDir()
	t.Setenv("EYRIE_CONFIG_DIR", customDir)
	t.Setenv("HAWK_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	// Compute the same old dir the migration will read: it derives from
	// os.UserConfigDir(), which on macOS/Linux appends "Library/Application
	// Support" (or XDG_CONFIG_HOME) under $HOME. Replicate that to put the
	// old file in the same path the migration will look at.
	t.Setenv("HOME", userDir)
	userConfigDir, err := os.UserConfigDir()
	if err != nil || userConfigDir == "" {
		t.Skipf("UserConfigDir unavailable on this platform: %v", err)
	}
	oldDir := filepath.Join(userConfigDir, "hawk")
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatalf("mkdir old: %v", err)
	}
	oldData := []byte(`{"version":1,"active":{"provider":"openai","model":"gpt-4o"}}`)
	oldPath := filepath.Join(oldDir, "provider.json")
	if err := os.WriteFile(oldPath, oldData, 0o600); err != nil {
		t.Fatalf("write old: %v", err)
	}

	// Sanity: the resolved dir is the custom one.
	got, err := config.GetProviderConfigDir()
	if err != nil {
		t.Fatalf("GetProviderConfigDir: %v", err)
	}
	if got != customDir {
		t.Fatalf("GetProviderConfigDir = %q, want %q", got, customDir)
	}

	migrateProviderConfigDir()

	// Should have copied to <EYRIE_CONFIG_DIR>/provider.json.
	dst := filepath.Join(customDir, "provider.json")
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read %s: %v (migration should target the custom dir)", dst, err)
	}
	if string(data) != string(oldData) {
		t.Fatalf("migration content mismatch")
	}
	// Should NOT have created a file at the default path.
	defaultPath := filepath.Join(userConfigDir, "eyrie", "provider.json")
	if _, err := os.Stat(defaultPath); err == nil {
		t.Fatalf("migration wrote to default %s — should only write to EYRIE_CONFIG_DIR", defaultPath)
	}
}

// TestMigrateConfigDirAtomicAndIdempotent verifies the .tmp+rename write
// does not leave a partial file and is safe to call twice.
func TestMigrateConfigDirAtomicAndIdempotent(t *testing.T) {
	migrateProviderConfigDirOnce = sync.Once{}
	userDir := t.TempDir()
	customDir := t.TempDir()
	t.Setenv("EYRIE_CONFIG_DIR", customDir)
	t.Setenv("HAWK_CONFIG_DIR", "")
	t.Setenv("HOME", userDir)
	userConfigDir, err := os.UserConfigDir()
	if err != nil || userConfigDir == "" {
		t.Skipf("UserConfigDir unavailable: %v", err)
	}
	oldDir := filepath.Join(userConfigDir, "hawk")
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatalf("mkdir old: %v", err)
	}
	oldData := []byte(`{"version":2}`)
	if err := os.WriteFile(filepath.Join(oldDir, "provider.json"), oldData, 0o600); err != nil {
		t.Fatalf("write old: %v", err)
	}

	migrateProviderConfigDir()
	// First call: file present, no .tmp left.
	if _, err := os.Stat(filepath.Join(customDir, "provider.json")); err != nil {
		t.Fatalf("first migration: %v", err)
	}
	if _, err := os.Stat(filepath.Join(customDir, "provider.json.tmp")); err == nil {
		t.Fatalf("atomic .tmp file leaked after migration")
	}
	// Reset the once and run again; must not overwrite (idempotent).
	migrateProviderConfigDirOnce = sync.Once{}
	migrateProviderConfigDir()
	got, _ := os.ReadFile(filepath.Join(customDir, "provider.json"))
	if string(got) != string(oldData) {
		t.Fatalf("second migration changed file content")
	}
}
