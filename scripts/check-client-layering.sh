#!/usr/bin/env bash
# Enforce the client package decomposition layering
# (plans/client-package-decomposition.md):
#   - client/core is a leaf: it must not import any eyrie/client package.
#   - client subpackages (embeddings, ...) may import client/core only —
#     never the client facade or a sibling subpackage.
set -euo pipefail
cd "$(dirname "$0")/.."

fail=0

# core must not import any eyrie/client package.
if grep -rn --include='*.go' '"github.com/GrayCodeAI/eyrie/client' client/core/ | grep -v '/client/core"'; then
  echo "FAIL: client/core must not import other eyrie/client packages" >&2
  fail=1
fi

# Subpackages (all dirs under client/ except core) may import only client/core.
for dir in client/*/; do
  name=$(basename "$dir")
  [ "$name" = "core" ] && continue
  if grep -rn --include='*.go' '"github.com/GrayCodeAI/eyrie/client' "$dir" | grep -v "/client/core\"" ; then
    echo "FAIL: client/$name may import client/core only (no facade, no siblings)" >&2
    fail=1
  fi
done

if [ "$fail" -ne 0 ]; then exit 1; fi
echo "client layering guard passed"
