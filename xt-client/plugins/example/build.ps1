# Builds the example plugin package without touching the client version.
$ErrorActionPreference = 'Stop'
Push-Location (Join-Path $PSScriptRoot '../..')
try {
  $package = Join-Path $PWD 'dist/plugins/example'
  New-Item -ItemType Directory -Force $package | Out-Null
  go build -trimpath -o (Join-Path $package 'provider.exe') ./plugins/example
  if ($LASTEXITCODE -ne 0) { throw 'Example plugin build failed' }
  Copy-Item (Join-Path $PSScriptRoot 'manifest.json') (Join-Path $package 'manifest.json') -Force
  Write-Output "Plugin package: $package"
} finally { Pop-Location }
