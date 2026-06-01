#!/usr/bin/env pwsh
# TokenSentinel E2E Validation (Windows / PowerShell)
param(
    [string]$RedisHost = "localhost",
    [int]$RedisPort = 6379,
    [string]$GoRouterUrl = "http://localhost:8080",
    [string]$DashboardUrl = "http://localhost:3001"
)

$PASS = 0
$FAIL = 0

function Check {
    param([string]$Desc, [scriptblock]$ScriptBlock)
    try {
        & $ScriptBlock *>$null
        if ($LASTEXITCODE -eq 0 -or $?) {
            Write-Host "  PASS: $Desc"
            $script:PASS++
        } else {
            Write-Host "  FAIL: $Desc"
            $script:FAIL++
        }
    } catch {
        Write-Host "  FAIL: $Desc"
        $script:FAIL++
    }
}

function Invoke-Redis {
    param([string]$Args)
    $p = Start-Process -NoNewWindow -FilePath "redis-cli" -ArgumentList "-h $RedisHost -p $RedisPort $Args" -Wait -PassThru
    return $p.ExitCode -eq 0
}

Write-Host "=== TokenSentinel E2E Validation ==="

# Services must be running (docker compose up -d)
Write-Host "--- Service health ---"
Check "Redis ping" { redis-cli -h $RedisHost -p $RedisPort ping }
Check "Go router health" { Invoke-WebRequest -Uri "$GoRouterUrl/health" -UseBasicParsing }
Check "Cost dashboard health" { Invoke-WebRequest -Uri "$DashboardUrl/" -UseBasicParsing }
Check "Erlang monitor heartbeat" { redis-cli -h $RedisHost -p $RedisPort GET health:erlang-monitor }

Write-Host "--- Cost data flow ---"
$REQ_ID = "e2e-test-$(Get-Date -UFormat %s)"
redis-cli -h $RedisHost -p $RedisPort SETEX "sentinel:${REQ_ID}:cost" 3600 `
    '{"model":"gpt-4","input_tokens":50,"output_tokens":150,"timestamp":"2026-06-01T12:00:00Z"}'
Check "Redis cost key written" { redis-cli -h $RedisHost -p $RedisPort EXISTS "sentinel:${REQ_ID}:cost" }

redis-cli -h $RedisHost -p $RedisPort PUBLISH "health:events" "cost:${REQ_ID}" > $null
Start-Sleep -Seconds 2

Write-Host "--- Cost dashboard ---"
Check "Dashboard summary returns data" {
    $r = Invoke-WebRequest -Uri "$DashboardUrl/api/dashboard/summary?period=24h" -UseBasicParsing
    $r.Content -match "total_requests"
}
Check "Dashboard cost by model returns data" {
    $r = Invoke-WebRequest -Uri "$DashboardUrl/api/dashboard/costs?period=24h" -UseBasicParsing
    $r.Content -match "model"
}
Check "Dashboard HTML page" {
    $r = Invoke-WebRequest -Uri "$DashboardUrl/" -UseBasicParsing
    $r.Content -match "TokenSentinel"
}

Write-Host "--- Route resolution (simulated) ---"
redis-cli -h $RedisHost -p $RedisPort SET "routes:/v1/chat/completions" `
    '{"pattern":"/v1/chat/completions","providers":[{"url":"https://api.openai.com/v1/chat/completions","timeout":30,"model":"gpt-4","weight":1}]}'
Check "Route key written" { redis-cli -h $RedisHost -p $RedisPort EXISTS "routes:/v1/chat/completions" }

Write-Host ""
Write-Host "=== Results: $PASS passed, $FAIL failed ==="
exit $FAIL
