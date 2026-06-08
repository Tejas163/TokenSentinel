#!/usr/bin/env bash
set -euo pipefail

echo "=== TokenSentinel Benchmark Suite ==="

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
DASHBOARD_URL="${DASHBOARD_URL:-http://localhost:3001}"
RESULTS_DIR="${RESULTS_DIR:-./results}"
mkdir -p "$RESULTS_DIR"

# Pre-flight checks
echo "--- Pre-flight checks ---"
for url in "$GATEWAY_URL/health" "$DASHBOARD_URL/health"; do
  if curl -sf "$url" > /dev/null 2>&1; then
    echo "  OK: $url"
  else
    echo "  FAIL: $url"
    exit 1
  fi
done

# Run k6 load test
echo "--- Running k6 load test (3 scenarios: ramp-up, soak, spike) ---"
if command -v k6 &> /dev/null; then
  GATEWAY_URL="$GATEWAY_URL" DASHBOARD_URL="$DASHBOARD_URL" \
    k6 run k6-load-test.js --out json="$RESULTS_DIR/raw.json"
  echo "--- Results ---"
  if [ -f "$RESULTS_DIR/results.json" ]; then
    cat "$RESULTS_DIR/results.json" | python3 -m json.tool 2>/dev/null || cat "$RESULTS_DIR/results.json"
  fi
else
  echo "  k6 not installed. Install from https://k6.io/docs/getting-started/installation/"
  echo "  Or run via Docker: docker run --rm -i grafana/k6 run --env GATEWAY_URL=... - <k6-load-test.js"
  exit 1
fi

echo "=== Benchmark complete ==="
echo "Raw data: $RESULTS_DIR/raw.json"
echo "Summary:  $RESULTS_DIR/results.json"
