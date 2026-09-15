#!/usr/bin/env bash
# Hard-reset flux to origin/main. Use after a history rewrite or stale SHAs.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

echo "==> Fetching origin"
git fetch origin

echo "==> Resetting flux to origin/main"
git checkout main
git reset --hard origin/main

echo "==> Done. flux: $(git rev-parse --short HEAD)"
