#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Eyrie is host-neutral: it must not depend on any Hawk package. Shared
# ecosystem vocabulary belongs in eagle, whose module path does
# not match this expression.
FORBIDDEN_HAWK='github\.com/GrayCodeAI/hawk(/|")'
FORBIDDEN_ENGINES='github\.com/GrayCodeAI/(harrier|shrike|swift|kestrel|merlin)(/|")'

exit_code=0

if command -v rg >/dev/null 2>&1; then
  violations="$(rg -n "$FORBIDDEN_HAWK" --glob '*.go' . || true)"
  engine_violations="$(rg -n "$FORBIDDEN_ENGINES" --glob '*.go' . || true)"
else
  violations="$(grep -rn --include='*.go' -E "$FORBIDDEN_HAWK" . || true)"
  engine_violations="$(grep -rn --include='*.go' -E "$FORBIDDEN_ENGINES" . || true)"
fi

if [[ -n "${violations}" ]]; then
  echo "forbidden Hawk imports found:"
  echo "${violations}"
  echo
  echo "eyrie must use eagle or local contracts, never the Hawk product module"
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
