param(
    [int]$Port = 8080
)

$ErrorActionPreference = "Stop"

Push-Location $PSScriptRoot
try {
    $env:PORT = [string]$Port
    docker compose up --build --scale app=2
}
finally {
    Pop-Location
}
