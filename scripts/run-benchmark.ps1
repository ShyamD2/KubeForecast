# PowerShell runner for benchmark suite on Windows
$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$rootDir = Split-Path -Parent $scriptDir

Write-Host "=== Running Kubernetes Predictive Scheduling Benchmark Suite ===" -ForegroundColor Cyan
Set-Location $rootDir

if (!(Test-Path "./experiments/results")) {
    New-Item -ItemType Directory -Path "./experiments/results" | Out-Null
}

$goBin = "C:\Program Files\Go\bin\go.exe"
if (!(Test-Path $goBin)) {
    $goBin = "go"
}

& $goBin run ./cmd/simulator/main.go --scenario all --output-dir ./experiments/results

Write-Host ""
Write-Host "Benchmark completed successfully." -ForegroundColor Green
Write-Host "Results exported to:"
Write-Host "  - JSON: ./experiments/results/benchmark_summary.json"
Write-Host "  - CSV:  ./experiments/results/benchmark_summary.csv"
