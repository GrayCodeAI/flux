package flux_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// These tests keep the file paths that agents and contributors read first
// pointing at files that exist: every path-like token in AGENTS.md and every
// entry of the directory trees in README.md and docs/README.md.

// agentsPathToken matches a backticked token that contains a slash, such as
// `provider/core/stream.go` or `provider/core.Provider`.
var agentsPathToken = regexp.MustCompile("`([A-Za-z0-9_.<>-]+(?:/[A-Za-z0-9_.<>-]*)+)`")

// symbolRef matches the last path element of a package-qualified symbol such
// as core.Provider or adapters.OpenAICompat.
var symbolRef = regexp.MustCompile(`^([a-z0-9_]+)\.[A-Z][A-Za-z0-9_]*$`)

// treeEntry matches one entry of a box-drawing directory tree, capturing the
// indentation and the names before an optional "# comment".
var treeEntry = regexp.MustCompile(`^((?:│   |    )*)(?:├── |└── )([^#]+?)\s*(?:#.*)?$`)

func TestAgentsMDPathsExist(t *testing.T) {
	data, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range agentsPathToken.FindAllStringSubmatch(string(data), -1) {
		token := m[1]
		switch {
		case strings.ContainsAny(token, "<>"), // placeholder such as provider/<name>.go
			strings.HasPrefix(token, "../"), // sibling checkout
			strings.HasPrefix(token, "github.com/"),
			strings.HasPrefix(token, "rho/"):
			continue
		}
		path := strings.TrimSuffix(token, "/")
		dir, last := filepath.Split(path)
		if sm := symbolRef.FindStringSubmatch(last); sm != nil {
			path = dir + sm[1] // package directory of a qualified symbol
		}
		if _, err := os.Stat(filepath.FromSlash(path)); err != nil {
			t.Errorf("AGENTS.md cites `%s`, but %s does not exist", token, path)
		}
	}
}

func TestDocTreesListExistingPaths(t *testing.T) {
	for _, tc := range []struct{ doc, root string }{
		{"README.md", "flux/"},
		{"docs/README.md", "docs/"},
	} {
		data, err := os.ReadFile(filepath.FromSlash(tc.doc))
		if err != nil {
			t.Fatal(err)
		}
		checkTree(t, tc.doc, tc.root, string(data))
	}
}

// checkTree finds the fenced tree whose first line is root and verifies that
// each entry exists. root "flux/" is the repository root.
func checkTree(t *testing.T, doc, root, text string) {
	t.Helper()
	lines := strings.Split(text, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == root && i > 0 && strings.HasPrefix(lines[i-1], "```") {
			start = i + 1
			break
		}
	}
	if start < 0 {
		t.Fatalf("%s: no directory tree rooted at %q", doc, root)
	}
	base := strings.TrimSuffix(root, "/")
	if root == "flux/" {
		base = "."
	}
	stack := []string{base}
	entries := 0
	for _, line := range lines[start:] {
		if strings.HasPrefix(line, "```") {
			break
		}
		m := treeEntry.FindStringSubmatch(line)
		if m == nil {
			t.Errorf("%s: unparseable tree line %q", doc, line)
			continue
		}
		depth := utf8.RuneCountInString(m[1]) / 4
		if depth+1 > len(stack) {
			t.Errorf("%s: tree line %q is nested under nothing", doc, line)
			continue
		}
		stack = stack[:depth+1]
		names := strings.Fields(m[2])
		for _, name := range names {
			entries++
			path := filepath.Join(append(append([]string{}, stack...), filepath.FromSlash(name))...)
			if _, err := os.Stat(path); err != nil {
				t.Errorf("%s lists %s, which does not exist", doc, filepath.ToSlash(path))
			}
		}
		if len(names) == 1 && strings.HasSuffix(names[0], "/") {
			stack = append(stack, strings.TrimSuffix(names[0], "/"))
		}
	}
	if entries == 0 {
		t.Errorf("%s: tree rooted at %q has no entries", doc, root)
	}
}
