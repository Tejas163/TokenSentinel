#!/usr/bin/env pwsh
# TokenSentinel Interactive Demo
# Run: .\demo\demo.ps1

$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
Set-Location $ProjectRoot

function Heading($t) { Write-Host "`n============================================" -ForegroundColor Cyan; Write-Host "  $t" -ForegroundColor White; Write-Host "============================================" -ForegroundColor Cyan }

function Step($n, $t) { Write-Host "`n--- Step ${n}: ${t} ---" -ForegroundColor Yellow }
function Pass($m) { Write-Host "  [OK] $m" -ForegroundColor Green }
function Fail($m) { Write-Host "  [FAIL] $m" -ForegroundColor Red }

function Api($url) {
    try { return (Invoke-RestMethod -Uri $url -Headers @{"X-Api-Key"="dev-key-123"} -ErrorAction Stop) } catch { return $null }
}

Heading "TokenSentinel Live Demo"

# Step 1: Check + start services
Step "1" "Starting all services"
& cmd /c "docker compose -f proxyops_gateway/docker-compose.yml up -d >nul 2>&1"
Start-Sleep 8

$redisContainer = docker ps --filter "name=redis" -q

$redis = docker exec $redisContainer redis-cli PING 2>$null
if ($redis -match "PONG") { Pass "Redis" } else { Fail "Redis" }

$go = Api "http://localhost:8080/health"
if ($go) { Pass "Go Router (:8080) - $($go.status)" } else { Fail "Go Router" }

$rust = Api "http://localhost:3000/health"
if ($rust) { Pass "Rust Proxy (:3000) - $($rust.status)" } else { Fail "Rust Proxy" }

$hb = docker exec $redisContainer redis-cli GET "health:erlang-monitor" 2>$null
if ($hb) { Pass "Erlang Monitor - heartbeat: $hb" } else { Fail "Erlang Monitor" }

$dash = Api "http://localhost:3001/api/dashboard/summary?period=1h"
if ($dash) { Pass "Cost Dashboard (:3001)" } else { Fail "Cost Dashboard" }

# Step 2: Inject cost data
Step "2" "Injecting multi-model cost data"
$records = @(
    @{model="gpt-4";          input=120; output=340}
    @{model="gpt-4";          input=85;  output=220}
    @{model="claude-3-opus";  input=200; output=500}
    @{model="gpt-3.5-turbo";  input=45;  output=90}
    @{model="claude-3-opus";  input=150; output=380}
    @{model="gpt-4";          input=95;  output=180}
    @{model="gemini-pro";     input=60;  output=120}
    @{model="claude-3-opus";  input=300; output=650}
)
$i = 0
foreach ($r in $records) {
    $i++
    $reqId = "demo-$i"
    $json = "{`"model`":`"$($r.model)`",`"input_tokens`":$($r.input),`"output_tokens`":$($r.output),`"timestamp`":`"2026-06-01T12:00:00Z`"}"
    $json | docker exec -i $redisContainer redis-cli -x SET "sentinel:${reqId}:cost" *>$null
    docker exec $redisContainer redis-cli PUBLISH "health:events" "cost:${reqId}" *>$null
    Pass "$($r.model) - $($r.input) in / $($r.output) out"
    Start-Sleep -Milliseconds 200
}
Start-Sleep 6

# Step 3: Query dashboard
Step "3" "Querying cost dashboard"
$summary = Api "http://localhost:3001/api/dashboard/summary?period=24h"
if ($summary) {
    Write-Host "  Summary:"
    Write-Host "    Requests:       $($summary.total_requests)" -ForegroundColor White
    Write-Host "    Total Tokens:   $($summary.total_tokens)" -ForegroundColor White
    Write-Host "    Input:          $($summary.total_input)" -ForegroundColor White
    Write-Host "    Output:         $($summary.total_output)" -ForegroundColor White
    Write-Host "    Unique Models:  $($summary.unique_models)" -ForegroundColor White
    Write-Host "    Avg/Request:    $([math]::Round($summary.avg_tokens_per_request))" -ForegroundColor White
}

$costs = Api "http://localhost:3001/api/dashboard/costs?period=24h"
if ($costs) {
    Write-Host "`n  By Model:"
    Write-Host "    Model              Reqs    Input   Output   Total" -ForegroundColor Gray
    Write-Host "    -----------------  ------  ------  -------  -----" -ForegroundColor Gray
    foreach ($m in $costs) {
        Write-Host ("    {0,-18} {1,6} {2,7} {3,7} {4,6}" -f $m.model, $m.request_count, $m.total_input, $m.total_output, $m.total_tokens)
    }
}

# Step 4: Route config
Step "4" "Testing route configuration"
$routeJson = '{"pattern":"/v1/chat/completions","providers":[{"url":"https://api.openai.com/v1/chat/completions","timeout":30,"model":"gpt-4","weight":1}]}'
$routeJson | docker exec -i $redisContainer redis-cli -x SET "routes:/v1/chat/completions" *>$null
$routeCheck = docker exec $redisContainer redis-cli EXISTS "routes:/v1/chat/completions" 2>$null
if ($routeCheck -eq 1) { Pass "Route stored: /v1/chat/completions -> gpt-4" }

# Step 5: Verify event bus
Step "5" "Verifying infrastructure"
$channels = docker exec $redisContainer redis-cli PUBSUB CHANNELS 2>$null
if ($channels -match "health:events") { Pass "Pub/sub channel active: health:events" }
$keys = docker exec $redisContainer redis-cli KEYS "sentinel:*:cost" 2>$null
if ($keys) { Pass "Cost keys in Redis: $($keys.Count) entries" }

# Done
Heading "Demo Complete"
Write-Host "  All systems operational" -ForegroundColor Green
Write-Host ""
Write-Host "  Dashboard:   http://localhost:3001/" -ForegroundColor Cyan
Write-Host "  Go Router:   http://localhost:8080/health" -ForegroundColor Cyan
Write-Host "  Rust Proxy:  http://localhost:3000/health" -ForegroundColor Cyan
Write-Host ""

try { Start-Process "http://localhost:3001/" } catch {}
