$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
  Push-Location frontend
  try {
    npm ci
    if ($LASTEXITCODE -ne 0) { throw 'Frontend dependencies failed' }
    npm run build
    if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
  } finally { Pop-Location }
  New-Item -ItemType Directory -Force dist | Out-Null
  go run github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json --arch amd64 --out cmd/xiaotao/rsrc
  if ($LASTEXITCODE -ne 0) { throw 'Icon resource generation failed' }
  go build -trimpath -ldflags '-s -w -H=windowsgui' -o dist/xiaotao.exe ./cmd/xiaotao
  if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }
  Get-Item dist/xiaotao.exe | Select-Object FullName, Length
} finally { Pop-Location }
