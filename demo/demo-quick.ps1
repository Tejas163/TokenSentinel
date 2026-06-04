param(
  [string]$BaseUrl = "http://localhost:3001",
  [string]$ApiKey = "dev-key-123"
)

$ErrorActionPreference = "Stop"

function Api($method, $path, $body) {
  $wc = New-Object System.Net.WebClient
  $wc.Headers.Add("X-Api-Key", $ApiKey)
  if ($body) {
    $wc.Headers.Add("Content-Type", "application/json")
    return $wc.UploadString("$BaseUrl$path", $method, ($body | ConvertTo-Json -Depth 10 -Compress))
  }
  return $wc.DownloadString("$BaseUrl$path")
}

Write-Host "=== TokenSentinel Quick Demo ===" -ForegroundColor Cyan
Write-Host ""

# Step 1: Seed demo data
Write-Host "[1/4] Seeding demo data..." -NoNewline
$result = Api "POST" "/api/admin/seed-demo" @{}
$data = $result | ConvertFrom-Json
Write-Host " done (assessment #$($data.assessment_id), $($data.entries_added) cost entries)" -ForegroundColor Green

# Step 2: View the prescriptive report
Write-Host "[2/4] Prescriptive report generated..." -NoNewline
$report = Api "GET" "/api/prescriptive/report/$($data.assessment_id)"
$r = $report | ConvertFrom-Json
$rate = if ($r.total_current_spend -gt 0) { [math]::Round($r.total_monthly_savings / $r.total_current_spend * 100, 0) } else { 0 }
Write-Host " done" -ForegroundColor Green
Write-Host "       Current spend: `$$($r.total_current_spend)"
Write-Host "       Monthly savings: `$$($r.total_monthly_savings) ($rate% reduction)"
foreach ($rec in $r.recommendations) {
  Write-Host "       - [$($rec.priority)] $($rec.description) -- save `$$([math]::Round($rec.monthly_savings))/mo"
}

# Step 3: Verify live monitoring
Write-Host "[3/4] Live monitoring status..." -NoNewline
$summary = Api "GET" "/api/dashboard/summary"
$s = $summary | ConvertFrom-Json
Write-Host " done" -ForegroundColor Green
Write-Host "       Models tracked: $($s.unique_models)"
Write-Host "       Total requests: $($s.total_requests)"
Write-Host "       Anomaly rules: active"

# Step 4: Open the pages
Write-Host ""
Write-Host "=== Demo Ready ===" -ForegroundColor Cyan
Write-Host "Dashboard:   $BaseUrl/dashboard" -ForegroundColor Yellow
Write-Host "Report:      $BaseUrl/api/prescriptive/report/$($data.assessment_id)" -ForegroundColor Yellow
Write-Host "Assessments: $BaseUrl/assessments" -ForegroundColor Yellow
Write-Host ""

# Open in browser
Start-Process "$BaseUrl/dashboard"
Start-Process "$BaseUrl/api/prescriptive/report/$($data.assessment_id)"
