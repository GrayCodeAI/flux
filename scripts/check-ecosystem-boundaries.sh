#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Flux is host-neutral: it must not depend on any Rho package.
# Shared ecosystem vocabulary lives in rho/internal/contracts, which
# hosts vendor rather than import from here.
FORBIDDEN_RHO='github\.com/GrayCodeAI/rho(/|")'
FORBIDDEN_ENGINES='github\.com/GrayCodeAI/swift(/|")'

exit_code=0

if command -v rg >/dev/null 2>&1; then
  violations="$(rg -n "$FORBIDDEN_RHO" --glob '*.go' . || true)"
  engine_violations="$(rg -n "$FORBIDDEN_ENGINES" --glob '*.go' . || true)"
else
  violations="$(grep -rn --include='*.go' -E "$FORBIDDEN_RHO" . || true)"
  engine_violations="$(grep -rn --include='*.go' -E "$FORBIDDEN_ENGINES" . || true)"
fi

if [[ -n "${violations}" ]]; then
  echo "forbidden Rho host imports found:"
  echo "${violations}"
  echo
  echo "flux must use local contracts, never the Rho product module"
  exit_code=1
fi

if [[ -n "${engine_violations}" ]]; then
  echo "forbidden cross-engine imports found:"
  echo "${engine_violations}"
  echo
  echo "support engines must not import other engines directly — they are peers, not dependencies"
  exit_code=1
fi

if [[ $exit_code -ne 0 ]]; then
  exit $exit_code
fi

echo "ecosystem boundary guard passed"
