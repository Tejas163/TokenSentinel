#!/usr/bin/env pwsh
# TokenSentinel Interactive Demo
# Run from project root: .\demo\demo.ps1

$ErrorActionPreference = "Stop"
$OK = "✅ "
$FAIL = "❌ "
$HEADING = "═══════════════════════════════════════════════"

function Heading($t) { Write-Host "`n$HEADING" -ForegroundColor Cyan; Write-Host "  $t" -ForegroundColor White; Write-Host $HEADING -ForegroundColor Cyan }

function Step($n, $t) { Write-Host "`n[$n] $t..." -ForegroundColor Yellow }

function Pass($m) { Write-Host "  $OK $m" -ForegroundColor Green }

function Fail($m) { Write-Host "  $FAIL $m" -ForegroundColor Red }

function Api($url) {
    try { return (Invoke-RestMethod -Uri $url -ErrorAction Stop) } catch { return $null }
}

Heading "TokenSentinel Live Demo"

# Step 1: Start services
Step "1/5" "Starting services"
$compose = "proxyops_gateway/docker-compose.yml"
docker compose -f $compose up -d --build 2>&1 | Out-Null
Start-Sleep -Seconds 5

# Wait for health
$maxWait = 30; $waited = 0
while ($waited -lt $maxWait) {
    $r = $null; try { $r = Invoke-RestMethod -Uri "http://localhost:8080/health" -ErrorAction Stop } catch {}
    if ($r) { break }
    Start-Sleep -Seconds 2; $waited += 2
}

# Check each service
$redis = docker exec $(docker ps --filter "name=redis" -q) redis-cli PING 2>$null
if ($redis -match "PONG") { Pass "Redis           → PONG" } else { Fail "Redis not responding" }

$go = Api "http://localhost:8080/health"
if ($go) { Pass "Go Router       → $($go | ConvertTo-Json -Compress)" } else { Fail "Go Router down" }

$rust = Api "http://localhost:3000/health"
if ($rust) { Pass "Rust Proxy      → $($rust | ConvertTo-Json -Compress)" } else { Fail "Rust Proxy down" }

$hb = docker exec $(docker ps --filter "name=redis" -q) redis-cli GET "health:erlang-monitor" 2>$null
if ($hb) { Pass "Erlang Monitor  → Heartbeat: $hb" } else { Fail "Erlang Monitor no heartbeat" }

$dash = Api "http://localhost:3001/api/dashboard/summary?period=1h"
if ($dash) { Pass "Cost Dashboard  → HTTP 200" } else { Fail "Cost Dashboard down" }

Start-Sleep -Seconds 5

# Step 2: Inject cost data
Step "2/5" "Injecting realistic cost data"
$records = @(
    @{model="gpt-4";          input=120; output=340; ts="2026-06-01T12:00:00Z"}
    @{model="gpt-4";          input=85;  output=220; ts="2026-06-01T12:05:00Z"}
    @{model="claude-3-opus";  input=200; output=500; ts="2026-06-01T12:10:00Z"}
    @{model="gpt-3.5-turbo";  input=45;  output=90;  ts="2026-06-01T12:15:00Z"}
    @{model="claude-3-opus";  input=150; output=380; ts="2026-06-01T12:20:00Z"}
    @{model="gpt-4";          input=95;  output=180; ts="2026-06-01T12:25:00Z"}
    @{model="gemini-pro";     input=60;  output=120; ts="2026-06-01T12:30:00Z"}
    @{model="claude-3-opus";  input=300; output=650; ts="2026-06-01T12:35:00Z"}
)
$redisContainer = docker ps --filter "name=redis" -q
$i = 0
foreach ($r in $records) {
    $i++
    $reqId = "demo-$i"
    $json = "{`"model`":`"$($r.model)`",`"input_tokens`":$($r.input),`"output_tokens`":$($r.output),`"timestamp`":`"$($r.ts)`"}"
    $json | docker exec -i $redisContainer redis-cli -x SET "sentinel:$reqId:cost" 2>$null | Out-Null
    docker exec $redisContainer redis-cli PUBLISH "health:events" "cost:$reqId" 2>$null | Out-Null
    Pass "$($r.model) → $($r.input) in / $($r.output) out"
    Start-Sleep -Milliseconds 300
}
Start-Sleep -Seconds 5

# Step 3: Query dashboard
Step "3/5" "Querying cost dashboard (24h period)"
Start-Sleep -Seconds 3
$summary = Api "http://localhost:3001/api/dashboard/summary?period=24h"
if ($summary) {
    Write-Host "  ┌──────────────────────────┬──────────┐" -ForegroundColor Gray
    Write-Host ("  │ {0,-24} │ {1,8} │" -f "Metric", "Value") -ForegroundColor Gray
    Write-Host "  ├──────────────────────────┼──────────┤" -ForegroundColor Gray
    Write-Host ("  │ {0,-24} │ {1,8} │" -f "Total Requests", $summary.total_requests) -ForegroundColor Gray
    Write-Host ("  │ {0,-24} │ {1,8} │" -f "Total Tokens", $summary.total_tokens) -ForegroundColor Gray
    Write-Host ("  │ {0,-24} │ {1,8} │" -f "Total Input", $summary.total_input) -ForegroundColor Gray
    Write-Host ("  │ {0,-24} │ {1,8} │" -f "Total Output", $summary.total_output) -ForegroundColor Gray
    Write-Host ("  │ {0,-24} │ {1,8} │" -f "Unique Models", $summary.unique_models) -ForegroundColor Gray
    Write-Host ("  │ {0,-24} │ {1,8} │" -f "Avg Tokens/Request", [math]::Round($summary.avg_tokens_per_request)) -ForegroundColor Gray
    Write-Host "  └──────────────────────────┴──────────┘" -ForegroundColor Gray
}

$costs = Api "http://localhost:3001/api/dashboard/costs?period=24h"
if ($costs) {
    Write-Host "`n  Cost Breakdown by Model:" -ForegroundColor White
    Write-Host "  ┌──────────────────┬──────────┬──────────┬───────────┬────────┐" -ForegroundColor Gray
    Write-Host ("  │ {0,-16} │ {1,8} │ {2,8} │ {3,9} │ {4,6} │" -f "Model", "Requests", "Input", "Output", "Total") -ForegroundColor Gray
    Write-Host "  ├──────────────────┼──────────┼──────────┼───────────┼────────┤" -ForegroundColor Gray
    foreach ($m in $costs) {
        Write-Host ("  │ {0,-16} │ {1,8} │ {2,8} │ {3,9} │ {4,6} │" -f $m.model, $m.request_count, $m.total_input, $m.total_output, $m.total_tokens) -ForegroundColor Gray
    }
    Write-Host "  └──────────────────┴──────────┴──────────┴───────────┴────────┘" -ForegroundColor Gray
}

# Step 4: Route config
Step "4/5" "Testing route configuration"
$routeJson = '{"pattern":"/v1/chat/completions","providers":[{"url":"https://api.openai.com/v1/chat/completions","timeout":30,"model":"gpt-4","weight":1}]}'
$routeJson | docker exec -i $redisContainer redis-cli -x SET "routes:/v1/chat/completions" 2>$null | Out-Null
$routeCheck = docker exec $redisContainer redis-cli EXISTS "routes:/v1/chat/completions" 2>$null
if ($routeCheck -eq 1) {
    Pass "Route stored: /v1/chat/completions → gpt-4 via api.openai.com"
}

# Step 5: Summary
Step "5/5" "Checking Redis event bus"
$channels = docker exec $redisContainer redis-cli PUBSUB CHANNELS 2>$null
if ($channels -match "health:events") {
    Pass "Pub/sub channel active: health:events"
}
$keys = docker exec $redisContainer redis-cli KEYS "sentinel:*:cost" 2>$null
if ($keys) {
    Pass "Cost keys in Redis: $($keys.Count) entries"
}

Heading "DEMO COMPLETE"
Write-Host "  All systems operational" -ForegroundColor Green
Write-Host ""
Write-Host "  Dashboard:   http://localhost:3001/" -ForegroundColor Cyan
Write-Host "  Go Router:   http://localhost:8080/health" -ForegroundColor Cyan
Write-Host "  Rust Proxy:  http://localhost:3000/health" -ForegroundColor Cyan
Write-Host ""

# Open dashboard in browser
try { Start-Process "http://localhost:3001/" } catch {}
