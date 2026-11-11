# Builds a separate plugin package without modifying the client version or installation.
$ErrorActionPreference = 'Stop'
Push-Location (Join-Path $PSScriptRoot '../..')
try {
  $package = Join-Path $PWD 'dist/plugins/trae'
  New-Item -ItemType Directory -Force $package | Out-Null
  go build -trimpath -o (Join-Path $package 'provider.exe') ./cmd/trae-source-plugin
  if ($LASTEXITCODE -ne 0) { throw 'Trae plugin build failed' }
  Copy-Item (Join-Path $PSScriptRoot 'manifest.json') (Join-Path $package 'manifest.json') -Force
  Write-Output "Plugin package: $package"
} finally { Pop-Location }
