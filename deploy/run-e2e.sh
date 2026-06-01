#!/usr/bin/env bash
set -euo pipefail

echo "=== TokenSentinel E2E Validation ==="
PASS=0
FAIL=0

check() {
  local desc="$1"
  shift
  if "$@" > /dev/null 2>&1; then
    echo "  PASS: $desc"
    PASS=$((PASS + 1))
  else
    echo "  FAIL: $desc"
    FAIL=$((FAIL + 1))
  fi
}

# Services must be running (docker compose up -d)
echo "--- Service health ---"
check "Redis ping" redis-cli -h localhost -p 6379 ping
check "Go router health" curl -sf http://localhost:8080/health
check "Cost dashboard health" curl -sf http://localhost:3001/
check "Erlang monitor heartbeat" redis-cli -h localhost -p 6379 GET health:erlang-monitor

echo "--- Cost data flow ---"
# Simulate a cost entry being written to Redis (as Go router would)
REQ_ID="e2e-test-$(date +%s)"
redis-cli -h localhost -p 6379 SETEX "sentinel:${REQ_ID}:cost" 3600 \
  '{"model":"gpt-4","input_tokens":50,"output_tokens":150,"timestamp":"2026-06-01T12:00:00Z"}'
check "Redis cost key written" \
  redis-cli -h localhost -p 6379 EXISTS "sentinel:${REQ_ID}:cost"

# Publish cost event (as Go router would)
redis-cli -h localhost -p 6379 PUBLISH "health:events" "cost:${REQ_ID}" > /dev/null
sleep 2

echo "--- Cost dashboard ---"
check "Dashboard summary returns data" \
  curl -sf "http://localhost:3001/api/dashboard/summary?period=24h" | grep -q "total_requests"
check "Dashboard cost by model returns data" \
  curl -sf "http://localhost:3001/api/dashboard/costs?period=24h" | grep -q "model"
check "Dashboard HTML page" \
  curl -sf http://localhost:3001/ | grep -q "TokenSentinel"

echo "--- Route resolution (simulated) ---"
# Simulate a route config in Redis
redis-cli -h localhost -p 6379 SET "routes:/v1/chat/completions" \
  '{"pattern":"/v1/chat/completions","providers":[{"url":"https://api.openai.com/v1/chat/completions","timeout":30,"model":"gpt-4","weight":1}]}'
check "Route key written" \
  redis-cli -h localhost -p 6379 EXISTS "routes:/v1/chat/completions"

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
exit $FAIL
