// Package flux provides LLM provider clients and configuration.
//
// The Version variable is sourced from the VERSION file at the repo root
// and propagated to sub-packages at init time.
package flux

import (
	_ "embed"
	"strings"

	"github.com/GrayCodeAI/flux/client"
)

//go:embed VERSION
var versionFile string

// Version of the flux library. Single source of truth: VERSION file.
var Version = strings.TrimSpace(versionFile)

func init() {
	client.SetVersion(Version)
}
