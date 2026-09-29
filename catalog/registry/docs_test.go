package registry_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/GrayCodeAI/flux/catalog/registry"
)

// These tests keep the provider facts that humans and agents read first
// (README, AGENTS.md, docs, .env.example, package comments) derived from the
// registry instead of hand-maintained numbers that drift.

// repoRoot is the module root relative to this package directory.
const repoRoot = "../.."

// providerCountDocs are the files that may state how many providers Flux
// supports. Every count they state must equal len(registry.All()).
var providerCountDocs = []string{
	"README.md",
	"AGENTS.md",
	"docs/README.md",
	"docs/ARCHITECTURE.md",
	"docs/guides/CREDENTIAL-SETUP-FLOW.md",
	"docs/guides/DYNAMIC-MODEL-DISCOVERY.md",
	"runtime/runtime.go",
}

// providerCountClaim matches prose such as "28 provider gateways",
// "16 registered providers" or "75+ LLM providers".
var providerCountClaim = regexp.MustCompile(`(?i)\b(\d+)(\+?)\s+(?:registered\s+|supported\s+|LLM\s+)?provider(?:s|\s+gateways)\b`)

// readmeProviderRow matches one row of the README "Supported Providers" table:
// | **Display name** | `provider_id` | `CREDENTIAL_ENV` optional note |
var readmeProviderRow = regexp.MustCompile("^\\| \\*\\*[^|]+\\*\\* \\| `([a-z0-9_]+)` \\| `([A-Z0-9_]+)`([^|]*)\\|$")

var envAssignment = regexp.MustCompile(`^([A-Z][A-Z0-9_]*)=`)

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

func specsBySortOrder() []registry.ProviderSpec {
	specs := registry.All()
	sort.SliceStable(specs, func(i, j int) bool { return specs[i].SortOrder < specs[j].SortOrder })
	return specs
}

// markdownSection returns the text from heading up to the next level-2 heading.
func markdownSection(t *testing.T, doc, heading string) string {
	t.Helper()
	start := strings.Index(doc, "\n"+heading+"\n")
	if start < 0 {
		t.Fatalf("heading %q not found", heading)
	}
	body := doc[start+len(heading)+2:]
	if end := strings.Index(body, "\n## "); end >= 0 {
		body = body[:end]
	}
	return body
}

func TestDocumentedProviderCountsMatchRegistry(t *testing.T) {
	t.Parallel()
	want := len(registry.All())
	for _, rel := range providerCountDocs {
		for _, m := range providerCountClaim.FindAllStringSubmatch(readRepoFile(t, rel), -1) {
			n, err := strconv.Atoi(m[1])
			if err != nil || n != want || m[2] != "" {
				t.Errorf("%s claims %q; catalog/registry defines exactly %d providers", rel, m[0], want)
			}
		}
	}
}

func TestREADMEProviderTableMatchesRegistry(t *testing.T) {
	t.Parallel()
	section := markdownSection(t, readRepoFile(t, "README.md"), "## Supported Providers")
	var rows [][]string
	for _, line := range strings.Split(section, "\n") {
		if m := readmeProviderRow.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			rows = append(rows, m)
		}
	}
	specs := specsBySortOrder()
	if len(rows) != len(specs) {
		t.Fatalf("README Supported Providers table has %d rows; registry has %d providers", len(rows), len(specs))
	}
	for i, spec := range specs {
		id, env, note := rows[i][1], rows[i][2], rows[i][3]
		if id != spec.ProviderID {
			t.Errorf("README row %d is %q; registry SortOrder position %d is %q", i+1, id, i+1, spec.ProviderID)
			continue
		}
		if env != spec.CredentialEnv {
			t.Errorf("README row %q lists %s; registry CredentialEnv is %s", id, env, spec.CredentialEnv)
		}
		for _, fallback := range spec.CredentialEnvFallbacks {
			if !strings.Contains(note, "`"+fallback+"`") {
				t.Errorf("README row %q does not mention credential fallback %s", id, fallback)
			}
		}
		for _, region := range spec.RegionOptions {
			if !strings.Contains(note, "`"+region.Value+"`") {
				t.Errorf("README row %q does not mention region %q", id, region.Value)
			}
		}
	}
}

func TestEnvExampleListsRegistryCredentials(t *testing.T) {
	t.Parallel()
	credentialEnvs := map[string]bool{}
	var want []string
	for _, spec := range specsBySortOrder() {
		credentialEnvs[spec.CredentialEnv] = true
		want = append(want, spec.CredentialEnv)
	}

	// The first run of consecutive KEY= lines is the provider credential block.
	var block []string
	inBlock := false
	for _, line := range strings.Split(readRepoFile(t, ".env.example"), "\n") {
		m := envAssignment.FindStringSubmatch(line)
		if m == nil {
			if inBlock {
				break
			}
			continue
		}
		inBlock = true
		block = append(block, m[1])
	}
	if strings.Join(block, " ") != strings.Join(want, " ") {
		t.Errorf(".env.example credential block drifted from the registry (SortOrder)\n got: %v\nwant: %v", block, want)
	}

	// No other API-key variable may appear: Flux reads none besides the
	// registry credentials, so an extra one would be a key nothing uses.
	for _, line := range strings.Split(readRepoFile(t, ".env.example"), "\n") {
		if m := envAssignment.FindStringSubmatch(strings.TrimPrefix(strings.TrimSpace(line), "# ")); m != nil &&
			strings.HasSuffix(m[1], "_API_KEY") && !credentialEnvs[m[1]] {
			t.Errorf(".env.example lists %s, which is not a registry CredentialEnv", m[1])
		}
	}
}
