package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Run with EXPORT_HAWK_FIXTURE=1 to refresh hawk/internal/catalogtest/testdata/minimal_v1.json
func TestExportHawkCatalogFixture(t *testing.T) {
	t.Parallel()
	if os.Getenv("EXPORT_HAWK_FIXTURE") != "1" {
		t.Skip("set EXPORT_HAWK_FIXTURE=1 to export") // TODO: https://github.com/GrayCodeAI/eyrie/issues/30
	}
	c := SeedCatalog()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join("..", "..", "hawk", "internal", "catalogtest", "testdata", "minimal_v1.json")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
