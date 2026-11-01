param(
  [ValidateSet('prod', 'uat')]
  [string]$Env = 'prod'
)

$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
  $environment = if ($Env -eq 'uat') {
    [pscustomobject]@{
      Name = 'uat'
      Exe = 'xiaotao-uat.exe'
      Shortcut = '小涛 UAT.lnk'
      Description = '小涛 UAT · 测试环境'
    }
  } else {
    [pscustomobject]@{
      Name = 'prod'
      Exe = 'xiaotao.exe'
      Shortcut = '小涛.lnk'
      Description = '小涛 · 你的 AI 工作空间'
    }
  }
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
  $ldflags = "-s -w -H=windowsgui -X xiaotao/xt-client/internal/runtimeenv.Active=$($environment.Name)"
  $buildArgs = @('-trimpath', '-ldflags', $ldflags, '-o', "dist/$($environment.Exe)", './cmd/xiaotao')
  & go build @buildArgs
  if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }
  $desktopPath = [Environment]::GetFolderPath('Desktop')
  $shell = New-Object -ComObject WScript.Shell
  $shortcut = $shell.CreateShortcut((Join-Path $desktopPath $environment.Shortcut))
  $shortcut.TargetPath = Join-Path $PSScriptRoot "dist/$($environment.Exe)"
  $shortcut.WorkingDirectory = Join-Path $PSScriptRoot 'dist'
  $shortcut.IconLocation = $shortcut.TargetPath + ',0'
  $shortcut.Description = $environment.Description
  $shortcut.Save()
  Get-Item "dist/$($environment.Exe)" | Select-Object FullName, Length
} finally { Pop-Location }
