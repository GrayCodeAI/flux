#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Flux is host-neutral: it must not depend on any Rho package.
# Shared ecosystem vocabulary lives in rho/internal/contracts, which
# hosts vendor rather than import from here.
FORBIDDEN_RHO='github\.com/GrayCodeAI/rho(/|")'

if command -v rg >/dev/null 2>&1; then
  violations="$(rg -n "$FORBIDDEN_RHO" --glob '*.go' . || true)"
else
  violations="$(grep -rn --include='*.go' -E "$FORBIDDEN_RHO" . || true)"
fi

if [[ -n "${violations}" ]]; then
  echo "forbidden Rho host imports found:"
  echo "${violations}"
  echo
  echo "flux must use local contracts, never the Rho product module"
  exit 1
fi

echo "ecosystem boundary guard passed"