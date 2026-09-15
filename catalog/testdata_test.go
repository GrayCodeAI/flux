package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Run with EXPORT_RHO_FIXTURE=1 to refresh rho/internal/catalogtest/testdata/minimal_v1.json
func TestExportRhoCatalogFixture(t *testing.T) {
	t.Parallel()
	if os.Getenv("EXPORT_RHO_FIXTURE") != "1" {
		t.Skip("set EXPORT_RHO_FIXTURE=1 to export") // TODO: https://github.com/GrayCodeAI/flux/issues/30
	}
	c := SeedCatalog()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join("..", "..", "rho", "internal", "catalogtest", "testdata", "minimal_v1.json")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
