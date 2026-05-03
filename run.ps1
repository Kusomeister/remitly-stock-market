param(
    [int]$Port = 8080,
    [int]$GrafanaPort = 3000
)

$ErrorActionPreference = "Stop"

Push-Location $PSScriptRoot
try {
    $env:PORT = [string]$Port
    $env:GRAFANA_PORT = [string]$GrafanaPort

    docker compose --profile observability up --build --scale app=2
}
finally {
    Pop-Location
}
