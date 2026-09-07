package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoLegacyHostNameInUserFacingStrings guards the three call sites that
// print a command for the user to run. They named the old product.
func TestNoLegacyHostNameInUserFacingStrings(t *testing.T) {
	files := []string{
		filepath.Join("..", "catalog", "v1.go"),
		filepath.Join("..", "setup", "status.go"),
		filepath.Join("..", "runtime", "preflight.go"),
	}
	for _, f := range files {
		data, err := os.ReadFile(f) // #nosec G304 -- fixed test fixture paths
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if !strings.Contains(line, `"`) {
				continue
			}
			if strings.Contains(line, "hawk models refresh") || strings.Contains(line, "hawk will discover") || strings.Contains(line, "hawk refreshes") {
				t.Errorf("%s:%d prints the legacy host name to the user: %s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
}
