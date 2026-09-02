#!/usr/bin/env bash
# CI guard: fail if go.mod has a local replace directive.
# Local replace directives are for development only and must be removed before tagging a release.
set -euo pipefail

if grep -qE '^replace .+ => \.\./' go.mod; then
  echo "ERROR: go.mod has a local replace directive:"
  grep -nE '^replace .+ => \.\./' go.mod
  echo ""
  echo "Local replace directives must be removed before tagging a release."
  echo "For a release, the published module version of eagle must be used."
  exit 1
fi
echo "OK: no local replace directives in go.mod."
