#!/usr/bin/env bash
set -euo pipefail

echo "=== TokenSentinel Benchmark Suite ==="

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
DASHBOARD_URL="${DASHBOARD_URL:-http://localhost:3001}"

# Health checks
echo "--- Pre-flight checks ---"
for url in "$GATEWAY_URL/health" "$DASHBOARD_URL/api/dashboard/summary?period=5m"; do
  if curl -sf "$url" > /dev/null 2>&1; then
    echo "  OK: $url"
  else
    echo "  FAIL: $url"
    exit 1
  fi
done

# Run k6 load test
echo "--- Running k6 load test ---"
if command -v k6 &> /dev/null; then
  GATEWAY_URL="$GATEWAY_URL" DASHBOARD_URL="$DASHBOARD_URL" \
    k6 run k6-load-test.js
  echo "--- Results ---"
  cat benchmark/results.json
else
  echo "  k6 not installed. Install from https://k6.io/docs/getting-started/installation/"
  echo "  Skipping load test."
fi

echo "=== Benchmark complete ==="
