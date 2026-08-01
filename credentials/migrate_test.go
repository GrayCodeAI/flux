package credentials

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReadEnvFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    map[string]string
		wantErr bool
	}{
		{
			name:    "simple_key_value",
			content: "ANTHROPIC_API_KEY=sk-ant-test\n",
			want:    map[string]string{"ANTHROPIC_API_KEY": "sk-ant-test"},
		},
		{
			name:    "export_prefix",
			content: "export OPENAI_API_KEY=sk-oai-test\n",
			want:    map[string]string{"OPENAI_API_KEY": "sk-oai-test"},
		},
		{
			name:    "double_quoted_value",
			content: `ANTHROPIC_API_KEY="sk-ant-quoted"` + "\n",
			want:    map[string]string{"ANTHROPIC_API_KEY": "sk-ant-quoted"},
		},
		{
			name:    "single_quoted_value",
			content: "OPENAI_API_KEY='sk-oai-single'\n",
			want:    map[string]string{"OPENAI_API_KEY": "sk-oai-single"},
		},
		{
			name:    "skip_comments",
			content: "# This is a comment\nANTHROPIC_API_KEY=sk-ant\n",
			want:    map[string]string{"ANTHROPIC_API_KEY": "sk-ant"},
		},
		{
			name:    "skip_empty_lines",
			content: "\n\nANTHROPIC_API_KEY=sk-ant\n\n\nOPENAI_API_KEY=sk-oai\n",
			want:    map[string]string{"ANTHROPIC_API_KEY": "sk-ant", "OPENAI_API_KEY": "sk-oai"},
		},
		{
			name:    "skip_no_equals",
			content: "INVALID_LINE\nANTHROPIC_API_KEY=sk-ant\n",
			want:    map[string]string{"ANTHROPIC_API_KEY": "sk-ant"},
		},
		{
			name:    "skip_empty_value",
			content: "EMPTY_KEY=\nANTHROPIC_API_KEY=sk-ant\n",
			want:    map[string]string{"ANTHROPIC_API_KEY": "sk-ant"},
		},
		{
			name:    "skip_empty_key",
			content: "=orphan_value\nANTHROPIC_API_KEY=sk-ant\n",
			want:    map[string]string{"ANTHROPIC_API_KEY": "sk-ant"},
		},
		{
			name:    "value_with_equals_sign",
			content: "API_KEY=key=with=equals\n",
			want:    map[string]string{"API_KEY": "key=with=equals"},
		},
		{
			name:    "multiple_keys",
			content: "ANTHROPIC_API_KEY=sk-ant\nOPENAI_API_KEY=sk-oai\nGEMINI_API_KEY=sk-gem\n",
			want: map[string]string{
				"ANTHROPIC_API_KEY": "sk-ant",
				"OPENAI_API_KEY":    "sk-oai",
				"GEMINI_API_KEY":    "sk-gem",
			},
		},
		{
			name:    "mixed_export_and_plain",
			content: "export ANTHROPIC_API_KEY=sk-ant\nOPENAI_API_KEY=sk-oai\n",
			want: map[string]string{
				"ANTHROPIC_API_KEY": "sk-ant",
				"OPENAI_API_KEY":    "sk-oai",
			},
		},
		{
			name:    "whitespace_around_key_and_value",
			content: "  ANTHROPIC_API_KEY  =  sk-ant  \n",
			want:    map[string]string{"ANTHROPIC_API_KEY": "sk-ant"},
		},
		{
			name:    "empty_file",
			content: "",
			want:    map[string]string{},
		},
		{
			name:    "only_comments",
			content: "# comment1\n# comment2\n",
			want:    map[string]string{},
		},
		{
			name:    "unquoted_value_with_single_quotes_not_stripped",
			content: "KEY=it's_complex\n",
			want:    map[string]string{"KEY": "it's_complex"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "env")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}

			got, err := readEnvFile(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("readEnvFile error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d entries, want %d: %v", len(got), len(tt.want), got)
			}
			for k, want := range tt.want {
				if got[k] != want {
					t.Errorf("got[%q] = %q, want %q", k, got[k], want)
				}
			}
		})
	}
}

func TestReadEnvFile_FileNotFound(t *testing.T) {
	_, err := readEnvFile("/nonexistent/path/env")
	if !os.IsNotExist(err) {
		t.Fatalf("expected os.IsNotExist, got: %v", err)
	}
}

func TestMigrateEnvFileCredentialsAt_NoFile(t *testing.T) {
	ctx := context.Background()
	n, err := migrateEnvFileAt(ctx, "/nonexistent/path/env")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 migrated, got %d", n)
	}
}

func TestMigrateEnvFileCredentialsAt_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env")
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	n, err := migrateEnvFileAt(ctx, path)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 migrated from empty file, got %d", n)
	}
	// Empty file should be removed.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("empty env file should be removed")
	}
}

func TestMigrateEnvFileCredentialsAt_OnlyComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env")
	if err := os.WriteFile(path, []byte("# just a comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	n, err := migrateEnvFileAt(ctx, path)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 migrated, got %d", n)
	}
}

func TestMigrateEnvFileCredentials_NilContext(t *testing.T) {
	// Should not panic with nil context.
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	n, err := MigrateEnvFileCredentials(context.Background())
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 migrated with no files, got %d", n)
	}
}

func TestMigrateEnvFileCredentialsAt_KeychainSkipsExisting(t *testing.T) {
	// Use mocked keyring via the global default store.
	ms := &MapStore{}
	cs := &CombinedStore{Keychain: ms}
	SetDefaultStore(cs)
	t.Cleanup(func() { SetDefaultStore(nil) })

	ctx := context.Background()
	// Pre-populate the store with an existing key.
	_ = ms.Set(ctx, AccountForEnv("ANTHROPIC_API_KEY"), "sk-already-there")

	dir := t.TempDir()
	path := filepath.Join(dir, "env")
	if err := os.WriteFile(path, []byte("ANTHROPIC_API_KEY=sk-from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	n, err := migrateEnvFileAt(ctx, path)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 (skipped but counted), got %d", n)
	}
	// The existing value should be preserved.
	got, _ := ms.Get(ctx, AccountForEnv("ANTHROPIC_API_KEY"))
	if got != "sk-already-there" {
		t.Errorf("existing key should be preserved: got %q", got)
	}
	// File should be removed.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("env file should be removed after migration")
	}
}

func TestMigrateEnvFileCredentialsAt_MultipleKeys(t *testing.T) {
	ms := &MapStore{}
	cs := &CombinedStore{Keychain: ms}
	SetDefaultStore(cs)
	t.Cleanup(func() { SetDefaultStore(nil) })

	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "env")
	content := "ANTHROPIC_API_KEY=sk-ant\nOPENAI_API_KEY=sk-oai\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	n, err := migrateEnvFileAt(ctx, path)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 migrated, got %d", n)
	}

	gotAnt, _ := ms.Get(ctx, AccountForEnv("ANTHROPIC_API_KEY"))
	if gotAnt != "sk-ant" {
		t.Errorf("ANTHROPIC_API_KEY = %q, want sk-ant", gotAnt)
	}
	gotOai, _ := ms.Get(ctx, AccountForEnv("OPENAI_API_KEY"))
	if gotOai != "sk-oai" {
		t.Errorf("OPENAI_API_KEY = %q, want sk-oai", gotOai)
	}
}

func TestMigrateEnvFileCredentials_BothPaths(t *testing.T) {
	ms := &MapStore{}
	cs := &CombinedStore{Keychain: ms}
	SetDefaultStore(cs)
	t.Cleanup(func() { SetDefaultStore(nil) })

	dir := t.TempDir()
	t.Setenv("HOME", dir)

	hawkDir := filepath.Join(dir, ".hawk")
	if err := os.MkdirAll(hawkDir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Write both env files.
	envPath := filepath.Join(hawkDir, "env")
	dotEnvPath := filepath.Join(hawkDir, ".env")
	if err := os.WriteFile(envPath, []byte("ANTHROPIC_API_KEY=sk-from-env\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dotEnvPath, []byte("OPENAI_API_KEY=sk-from-dotenv\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	n, err := MigrateEnvFileCredentials(ctx)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 total migrated, got %d", n)
	}

	// Both files should be removed.
	if _, err := os.Stat(envPath); !os.IsNotExist(err) {
		t.Error("~/.hawk/env should be removed")
	}
	if _, err := os.Stat(dotEnvPath); !os.IsNotExist(err) {
		t.Error("~/.hawk/.env should be removed")
	}
}

func TestMigrateEnvFileCredentialsAt_NilKeychain(t *testing.T) {
	// When DefaultStore is a CombinedStore with nil Keychain, migration should fail.
	cs := &CombinedStore{Keychain: nil}
	SetDefaultStore(cs)
	t.Cleanup(func() { SetDefaultStore(nil) })

	dir := t.TempDir()
	path := filepath.Join(dir, "env")
	if err := os.WriteFile(path, []byte("ANTHROPIC_API_KEY=sk-ant\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	_, err := migrateEnvFileAt(ctx, path)
	if err == nil {
		t.Fatal("expected error when keychain is nil")
	}
	if err != ErrKeychainUnavailable {
		t.Fatalf("expected ErrKeychainUnavailable, got: %v", err)
	}
}
