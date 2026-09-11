// Package eyrie provides LLM provider clients and configuration.
//
// The Version variable is sourced from the VERSION file at the repo root
// and propagated to sub-packages at init time.
package eyrie

import (
	_ "embed"
	"strings"

	"github.com/GrayCodeAI/eyrie/client"
)

//go:embed VERSION
var versionFile string

// Version of the eyrie library. Single source of truth: VERSION file.
var Version = strings.TrimSpace(versionFile)

func init() {
	client.SetVersion(Version)
}
