Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

Set-Location (Join-Path $PSScriptRoot "..")
swag init -g main.go -o docs
Write-Output "Swagger generado en backend/docs"
