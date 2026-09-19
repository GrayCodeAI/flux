#!/usr/bin/env bash
# Enforce provider feature-package layering.
# provider/core is the leaf contract and wire layer. Feature packages may
# depend on core, but never on the provider facade or on each other.
set -euo pipefail
cd "$(dirname "$0")/.."

fail=0

# core must not import any flux/provider package.
if grep -rn --include='*.go' '"github.com/GrayCodeAI/flux/provider' provider/core/ | grep -v '/provider/core"'; then
  echo "FAIL: provider/core must not import other provider packages" >&2
  fail=1
fi

# Subpackages (all dirs under provider/ except core) may import only provider/core.
for dir in provider/*/; do
  name=$(basename "$dir")
  [ "$name" = "core" ] && continue
  if grep -rn --include='*.go' '"github.com/GrayCodeAI/flux/provider' "$dir" | grep -v "/provider/core\""; then
    echo "FAIL: provider/$name may import provider/core only (no facade, no siblings)" >&2
    fail=1
  fi
done

if [ "$fail" -ne 0 ]; then exit 1; fi
echo "provider layering guard passed"
