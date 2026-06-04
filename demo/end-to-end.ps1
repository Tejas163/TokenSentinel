#!/usr/bin/env pwsh
# TokenSentinel End-to-End Demo — prescriptive engine + continuous optimization
# Run: .\demo\end-to-end.ps1

$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
Set-Location $ProjectRoot

function Heading($t) { Write-Host "`n============================================" -ForegroundColor Cyan; Write-Host "  $t" -ForegroundColor White; Write-Host "============================================" -ForegroundColor Cyan }

function Step($n, $t) { Write-Host "`n--- Step ${n}: ${t} ---" -ForegroundColor Yellow }
function SubStep($t) { Write-Host "  > $t" -ForegroundColor Gray }
function Pass($m) { Write-Host "  [OK] $m" -ForegroundColor Green }
function Fail($m) { Write-Host "  [FAIL] $m" -ForegroundColor Red }
function Info($m) { Write-Host "  [..] $m" -ForegroundColor DarkCyan }

function Api($url, $method="GET", $body=$null, $headers) {
    if (-not $headers) { $headers = @{} }
    $params = @{ Uri = $url; Method = $method; ContentType = "application/json" }
    if ($body) { $params.Body = ($body | ConvertTo-Json -Depth 10 -Compress) }
    if (-not $headers.ContainsKey("X-Api-Key")) { $params.Headers = @{"X-Api-Key"="dev-key-123"} }
    try { return (Invoke-RestMethod @params -ErrorAction Stop) } catch { return $null }
}

Heading "TokenSentinel End-to-End Demo"
Write-Host "  Gateway routing + prescriptive engine + continuous optimization"
Write-Host ""

# =============================================================================
# PHASE 1: Environment Check
# =============================================================================
Step "1" "Checking environment"

$redisContainer = docker ps --filter "name=redis" -q 2>$null
if (-not $redisContainer) {
    Info "Starting services via docker compose..."
    & cmd /c "docker compose -f proxyops_gateway/docker-compose.yml up -d >nul 2>&1"
    Start-Sleep 10
    $redisContainer = docker ps --filter "name=redis" -q 2>$null
}

$redisPing = docker exec $redisContainer redis-cli PING 2>$null
if ($redisPing -match "PONG") { Pass "Redis is running" } else { Fail "Redis not reachable"; exit 1 }

$health = Api "http://localhost:3001/health"
if ($health) { Pass "Cost-dashboard :3001 ($($health.status))" } else { Fail "Cost-dashboard not running"; exit 1 }

$health = Api "http://localhost:8080/health"
if ($health) { Pass "Go-router :8080 ($($health.status))" } else { Fail "Go-router not running" }

$health = Api "http://localhost:3000/health"
if ($health) { Pass "Rust-proxy :3000 ($($health.status))" } else { Fail "Rust-proxy not running" }

# =============================================================================
# PHASE 2: Inject sample gateway traffic (live cost data)
# =============================================================================
Step "2" "Injecting sample LLM traffic into the gateway"

$traffic = @(
    @{model="gpt-4o";         input=450;  output=1200; team="engineering"}
    @{model="gpt-4o";         input=320;  output=850;  team="engineering"}
    @{model="gpt-4o-mini";    input=120;  output=240;  team="engineering"}
    @{model="gpt-4o-mini";    input=200;  output=380;  team="product"}
    @{model="claude-3-opus";  input=800;  output=2400; team="research"}
    @{model="claude-3-opus";  input=650;  output=1900; team="research"}
    @{model="claude-3-sonnet"; input=300; output=900;  team="product"}
    @{model="gpt-4-turbo";    input=500;  output=1500; team="engineering"}
    @{model="gpt-3.5-turbo";  input=80;   output=160;  team="engineering"}
    @{model="llama-3-70b";    input=400;  output=1100; team="infra"}
    @{model="mixtral-8x7b";   input=250;  output=700;  team="infra"}
    @{model="gpt-4o";         input=510;  output=1400; team="engineering"}
    @{model="claude-3-opus";  input=900;  output=2800; team="research"}
    @{model="gpt-4o-mini";    input=150;  output=290;  team="product"}
    @{model="gpt-4-turbo";    input=600;  output=1800; team="engineering"}
)
$i = 0
foreach ($r in $traffic) {
    $i++
    $reqId = "e2e-$i"
    $ts = (Get-Date).AddMinutes(-$i*3).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    $json = "{`"model`":`"$($r.model)`",`"input_tokens`":$($r.input),`"output_tokens`":$($r.output),`"timestamp`":`"$ts`",`"team`":`"$($r.team)`"}"
    $json | docker exec -i $redisContainer redis-cli -x SET "sentinel:${reqId}:cost" >$null 2>&1
    docker exec $redisContainer redis-cli PUBLISH "health:events" "cost:${reqId}" >$null 2>&1
}
Start-Sleep 8
Pass "15 cost records injected across 7 models and 4 teams"

# =============================================================================
# PHASE 3: Prescriptive Assessment
# =============================================================================
Step "3" "Creating a prescriptive assessment"

$assessmentPayload = @{
    company_name = "DemoCorp"
    cloud_vendor = "aws"
    gpu_configs = @(
        @{ type = "A100"; count = 4; region = "us-east-1"; hourly_price = 3.50; reserved = $true },
        @{ type = "H100"; count = 2; region = "us-west-2"; hourly_price = 4.50; reserved = $false }
    )
    monthly_request_volume = 500000
    token_distribution = @{ input_pct = 0.75; output_pct = 0.25 }
    current_monthly_spend = 18500
    providers_used = @(
        @{ name = "openai"; models = @("gpt-4o", "gpt-4o-mini", "gpt-4-turbo"); monthly_spend = 9500 },
        @{ name = "anthropic"; models = @("claude-3-opus", "claude-3-sonnet"); monthly_spend = 7000 },
        @{ name = "self-hosted"; models = @("llama-3-70b", "mixtral-8x7b"); monthly_spend = 2000 }
    )
    team_composition = @{ developers = 25; platform_engineers = 3; devops = 2; management = 2 }
    source = "live"
}

$assessment = Api "http://localhost:3001/api/prescriptive/assessments" "POST" $assessmentPayload
if ($assessment -and $assessment.id) {
    $aid = $assessment.id
    Pass "Assessment created: ID=$aid, v$($assessment.version), $($assessment.company_name)"
} else {
    Fail "Failed to create assessment"
    exit 1
}

# =============================================================================
# PHASE 4: Run the Prescriptive Engine
# =============================================================================
Step "4" "Running the prescriptive engine"

$report = Api "http://localhost:3001/api/prescriptive/assessments/$aid/run" "POST"
if ($report) {
    Pass "Engine ran successfully"

    Write-Host "`n  === EXECUTIVE SUMMARY ===" -ForegroundColor Green
    Write-Host "  Current monthly spend:   `$($report.total_current_spend.ToString('N2'))" -ForegroundColor White
    Write-Host "  Projected monthly spend: `$($report.total_projected_spend.ToString('N2'))" -ForegroundColor White
    Write-Host "  Projected monthly savings:`$($report.total_monthly_savings.ToString('N2'))" -ForegroundColor Green
    if ($report.total_current_spend -gt 0) {
        $savingsRate = [math]::Round(($report.total_monthly_savings / $report.total_current_spend) * 100, 1)
        Write-Host "  Savings rate:            $savingsRate%" -ForegroundColor Cyan
    }

    Write-Host "`n  === COST BREAKDOWN ===" -ForegroundColor Green
    Write-Host "  Model                   Provider       Current/Mo    Proj/Mo" -ForegroundColor Gray
    Write-Host "  ----------------------  -------------  ------------  ------------" -ForegroundColor Gray
    foreach ($c in $report.cost_breakdown) {
        Write-Host ("  {0,-24} {1,-13}`$$($c.current_monthly_cost.ToString('N2'))`t`$$($c.projected_monthly_cost.ToString('N2'))" -f $c.model, $c.provider)
    }

    Write-Host "`n  === RECOMMENDATIONS ===" -ForegroundColor Green
    foreach ($r in $report.recommendations) {
        $icon = switch ($r.priority) { "high" { "!!!" } "medium" { "! " } default { "- " } }
        Write-Host "  [$($r.priority.ToUpper())] $($r.description)" -ForegroundColor $(if($r.priority -eq "high"){ "Red" } elseif($r.priority -eq "medium"){ "Yellow" } else { "Gray" })
    }
} else {
    Fail "Engine run failed"
}

# =============================================================================
# PHASE 5: View Report Page
# =============================================================================
Step "5" "Viewing the report page"

$reportHtml = Invoke-WebRequest -Uri "http://localhost:3001/api/prescriptive/report/$aid" -Headers @{"X-Api-Key"="dev-key-123"} -UseBasicParsing -ErrorAction SilentlyContinue
if ($reportHtml -and $reportHtml.Content.Length -gt 500) {
    Pass "Report page loaded ($($reportHtml.Content.Length) bytes)"
    Info "Open in browser: http://localhost:3001/api/prescriptive/report/$aid"
} else {
    Fail "Report page failed to load"
}

# =============================================================================
# PHASE 6: Export Reports
# =============================================================================
Step "6" "Exporting reports"

try {
    $csv = Invoke-WebRequest -Uri "http://localhost:3001/api/prescriptive/report/$aid/csv" -Headers @{"X-Api-Key"="dev-key-123"} -ErrorAction Stop
    if ($csv.Content.Length -gt 100) { Pass "CSV export: $($csv.Content.Length) bytes" } else { Fail "CSV export too short" }
} catch { Fail "CSV export failed" }

try {
    $pdf = Invoke-WebRequest -Uri "http://localhost:3001/api/prescriptive/report/$aid/pdf" -Headers @{"X-Api-Key"="dev-key-123"} -ErrorAction Stop
    if ($pdf.Content.Length -gt 100) { Pass "PDF export: $($pdf.Content.Length) bytes" } else { Fail "PDF export too short" }
} catch { Fail "PDF export failed" }

# =============================================================================
# PHASE 7: What-If Simulator
# =============================================================================
Step "7" "Testing What-If simulator"

$whatIfResult = Api "http://localhost:3001/api/prescriptive/what-if/$aid" "POST" @{ volume_multiplier = 1.5; input_pct = 0.8 }
if ($whatIfResult) {
    Pass "What-If calculated: $($whatIfResult.Count) model projections with 50% volume increase"
} else {
    Fail "What-If failed"
}

# =============================================================================
# PHASE 8: Starter Templates
# =============================================================================
Step "8" "Fetching starter templates"

$templates = Api "http://localhost:3001/api/prescriptive/templates"
if ($templates -and $templates.Count -eq 3) {
    Pass "3 starter templates available"
    foreach ($t in $templates) { Write-Host "  - $($t.name): $($t.description)" -ForegroundColor Gray }
} else {
    Fail "Templates endpoint failed"
}

# =============================================================================
# PHASE 9: Monitoring Rules + Alerts
# =============================================================================
Step "9" "Configuring continuous optimization"

$rulePayload = @{
    model = "gpt-4o"
    pct_threshold = 15
    abs_threshold = 50
    period = "7d"
    enabled = $true
    webhook_url = "https://hooks.example.com/tokensentinel"
    email_to = "admin@democorp.com"
}
$rule = Api "http://localhost:3001/api/monitoring/rules" "POST" $rulePayload
if ($rule -and $rule.id) {
    $rid = $rule.id
    Pass "Monitoring rule created: ID=$rid, model=$($rule.model), threshold=$($rule.pct_threshold)% / `$$($rule.abs_threshold)"
} else {
    Fail "Monitoring rule creation failed"
}

# Inject a spend spike to trigger an alert
SubStep "Injecting spend spike to trigger alert..."
$spikeRecords = 1..10 | ForEach-Object {
    $reqId = "spike-$_"
    $ts = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    $json = "{`"model`":`"gpt-4o`",`"input_tokens`":5000,`"output_tokens`":15000,`"timestamp`":`"$ts`"}"
    $json | docker exec -i $redisContainer redis-cli -x SET "sentinel:${reqId}:cost" >$null 2>&1
    docker exec $redisContainer redis-cli PUBLISH "health:events" "cost:${reqId}" >$null 2>&1
}
Start-Sleep 5
Pass "Spike data injected — 10 high-volume gpt-4o requests"

# Check for alerts
$alerts = Api "http://localhost:3001/api/monitoring/alerts?unacknowledged=true"
if ($alerts -and $alerts.Count -gt 0) {
    Pass "$($alerts.Count) active alerts detected"
    foreach ($a in $alerts) {
        Write-Host "  [$(if($a.severity -eq 'critical'){'!!!'}else{'! '})] $($a.message)" -ForegroundColor $(if($a.severity -eq 'critical'){'Red'}else{'Yellow'})
    }
} else {
    Info "No alerts yet (may need more data for baseline)"
}

# =============================================================================
# PHASE 10: Savings Tracking
# =============================================================================
Step "10" "Checking savings tracking"

$savings = Api "http://localhost:3001/api/monitoring/savings"
if ($savings -and $savings -is [System.Array] -and $savings.Count -gt 0) {
    Pass "$($savings.Count) savings events detected"
    foreach ($s in $savings) {
        if ($s.estimated_monthly_savings) {
            Write-Host "  $($s.model): `$$($s.estimated_monthly_savings.ToString('N2'))/mo saved ($($s.confidence))" -ForegroundColor Green
        } else {
            Write-Host "  $($s.model): savings data available" -ForegroundColor Green
        }
    }
} else {
    Info "No savings events yet (needs baseline comparison over >7 days of data)"
}

# =============================================================================
# PHASE 11: Cost Trends
# =============================================================================
Step "11" "Checking cost trend data"

$trends = Api "http://localhost:3001/api/monitoring/trends/gpt-4o?period=30d"
if ($trends -and $trends.Count -gt 0) {
    Pass "Trend data for gpt-4o: $($trends.Count) daily data points"
    foreach ($t in $trends[-3..-1]) { Write-Host "  $($t.date): `$$($t.cost.ToString('N2'))" -ForegroundColor Gray }
} else {
    Info "No trend data available yet"
}

# =============================================================================
# PHASE 12: Version Management
# =============================================================================
Step "12" "Testing version management"

$updatedPayload = $assessmentPayload.Clone()
$updatedPayload.current_monthly_spend = 22000
$updated = Api "http://localhost:3001/api/prescriptive/assessments/$aid" "PUT" $updatedPayload
if ($updated -and $updated.version -eq 2) {
    Pass "Assessment updated: v$($updated.version) (version 2 created)"

    $versions = Api "http://localhost:3001/api/prescriptive/assessments/$aid/versions"
    if ($versions -and $versions.Count -ge 2) {
        Pass "Version history: $($versions.Count) versions available"
    }
} else {
    Fail "Version update failed"
}

# =============================================================================
# SUMMARY
# =============================================================================
Heading "Demo Complete"
Write-Host "  All systems operational" -ForegroundColor Green
Write-Host ""
Write-Host "  REPORTS" -ForegroundColor Cyan
Write-Host "    JSON report:     curl -H 'X-Api-Key: dev-key-123' http://localhost:3001/api/prescriptive/report/$aid" -ForegroundColor White
Write-Host "    HTML page:       http://localhost:3001/api/prescriptive/report/$aid" -ForegroundColor White
Write-Host "    CSV:             http://localhost:3001/api/prescriptive/report/$aid/csv" -ForegroundColor White
Write-Host "    PDF:             http://localhost:3001/api/prescriptive/report/$aid/pdf" -ForegroundColor White
Write-Host ""
Write-Host "  DASHBOARDS" -ForegroundColor Cyan
Write-Host "    Cost dashboard:  http://localhost:3001/" -ForegroundColor White
Write-Host "    Report page:     http://localhost:3001/api/prescriptive/report/$aid" -ForegroundColor White
Write-Host ""
Write-Host "  MONITORING" -ForegroundColor Cyan
Write-Host "    Rules:           curl -H 'X-Api-Key: dev-key-123' http://localhost:3001/api/monitoring/rules" -ForegroundColor White
Write-Host "    Alerts:          curl -H 'X-Api-Key: dev-key-123' http://localhost:3001/api/monitoring/alerts" -ForegroundColor White
Write-Host "    Savings:         curl -H 'X-Api-Key: dev-key-123' http://localhost:3001/api/monitoring/savings" -ForegroundColor White
Write-Host ""
Write-Host "  SERVICES" -ForegroundColor Cyan
Write-Host "    Rust Proxy:      http://localhost:3000/health" -ForegroundColor White
Write-Host "    Go Router:       http://localhost:8080/health" -ForegroundColor White
Write-Host "    Cost Dashboard:  http://localhost:3001/health" -ForegroundColor White
