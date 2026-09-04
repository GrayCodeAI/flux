// Package graycode-router provides LLM provider clients and configuration.
//
// The Version variable is sourced from the VERSION file at the repo root
// and propagated to sub-packages at init time.
package graycoderouter

import (
	_ "embed"
	"strings"

	"github.com/GrayCodeAI/graycode-router/client"
)

//go:embed VERSION
var versionFile string

// Version of the graycode-router library. Single source of truth: VERSION file.
var Version = strings.TrimSpace(versionFile)

func init() {
	client.SetVersion(Version)
}
