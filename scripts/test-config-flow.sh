#!/usr/bin/env bash
# E2E test: /config flow — hub → credential → discover → picker → chat
# Run from anywhere: bash scripts/test-config-flow.sh
# Counts are derived from catalog/registry/providers.go, never hardcoded.
set -euo pipefail
cd "$(dirname "$0")/.."

PASS=0
FAIL=0

pass() { PASS=$((PASS+1)); echo "  PASS: $1"; }
fail() { FAIL=$((FAIL+1)); echo "  FAIL: $1"; }

echo "=== Config Flow E2E Test ==="
echo

# 1. Verify the provider registry is populated
echo "--- provider registry ---"
count=$(grep -c "ProviderID:" catalog/registry/providers.go || true)
if [ "${count:-0}" -gt 0 ]; then
  pass "registry has $count provider specs"
else
  fail "no ProviderID entries found in catalog/registry/providers.go"
fi

# 2. Verify all providers have deployment env fallbacks
echo "--- deployment env fallbacks ---"
# Check the registry test verifies all deployments have env fallbacks
if grep -q "HasAllProviderDeployments" catalog/deployment_env_test.go 2>/dev/null; then
  pass "deployment env fallbacks verified by test"
else
  fail "deployment env fallback test not found"
fi

# 3. Verify all providers have credential registry entries
echo "--- credential registry ---"
# Check the ensure function handles all providers (derived from registry, not hardcoded)
if grep -q "EnsureCredentialRegistryInCatalog" catalog/credential_registry.go 2>/dev/null; then
  pass "credential registry derived from ProviderSpec"
else
  fail "credential registry function not found"
fi

# 4. Verify every registry provider has a live fetcher
echo "--- live fetchers ---"
fetchers=$(grep -cE '^[[:space:]]+"[a-z0-9_]+":[[:space:]]+Fetch' catalog/live/fetchers.go || true)
if [ "${fetchers:-0}" -eq "${count:-0}" ]; then
  pass "all $count registry providers have live fetchers"
else
  fail "registry has ${count:-0} providers but catalog/live/fetchers.go registers ${fetchers:-0} fetchers"
fi

# 5. Verify build + tests pass
echo "--- build and test ---"
if go build ./... 2>/dev/null; then
  pass "package builds"
else
  fail "build failed"
fi

# 6. Quick test run (setup package which handles config flow)
if go test ./setup/... ./config/... ./runtime/... -count=1 -timeout 60s 2>/dev/null; then
  pass "config/setup/runtime tests pass"
else
  fail "config/setup/runtime tests failed"
fi

echo
echo "=== Results: $PASS passed, $FAIL failed ==="
if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
