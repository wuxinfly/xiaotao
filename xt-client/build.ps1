param(
  [ValidateSet('prod', 'uat')]
  [string]$Env = 'prod',
  [switch]$Verify
)

$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
$versionFile = $null
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
  $versionFile = [System.IO.File]::Open((Join-Path $PSScriptRoot 'VERSION'), [System.IO.FileMode]::Open, [System.IO.FileAccess]::ReadWrite, [System.IO.FileShare]::None)
  $reader = [System.IO.StreamReader]::new($versionFile, [System.Text.Encoding]::UTF8, $true, 1024, $true)
  try { $previousVersion = $reader.ReadToEnd().Trim() } finally { $reader.Dispose() }
  if ($previousVersion -notmatch '^(0|[1-9][0-9]*)[.](0|[1-9][0-9]*)[.](0|[1-9][0-9]*)$') {
    throw 'VERSION must contain a numeric version in major.minor.patch format'
  }
  $version = $previousVersion
  if (-not $Verify) {
    $patch = [System.Numerics.BigInteger]::Parse($Matches[3]) + [System.Numerics.BigInteger]::One
    $version = "$($Matches[1]).$($Matches[2]).$patch"
  }
  $versionedExe = [System.IO.Path]::GetFileNameWithoutExtension($environment.Exe) + '-v' + $version + '.exe'
  Push-Location frontend
  try {
    # Reinstalling deletes esbuild.exe, which a running Vite dev server keeps
    # open on Windows. Keep a valid tree in place and reserve npm ci for a
    # missing or incomplete installation.
    $dependenciesReady = $false
    if (Test-Path 'node_modules/.package-lock.json') {
      npm ls --depth=0 --silent 2>$null | Out-Null
      $dependenciesReady = $LASTEXITCODE -eq 0
    }
    if (-not $dependenciesReady) {
      $runningEsbuild = Get-CimInstance Win32_Process -Filter "Name='esbuild.exe'" |
        Where-Object { $_.ExecutablePath -like ((Join-Path $PWD 'node_modules/*') + '*') }
      if ($runningEsbuild) {
        throw 'Frontend dependencies are incomplete and esbuild.exe is in use. Stop the Vite dev server, then run build.ps1 again.'
      }
      npm ci
      if ($LASTEXITCODE -ne 0) { throw 'Frontend dependencies failed' }
    }
    npm run lint
    if ($LASTEXITCODE -ne 0) { throw 'Frontend lint failed' }
    npm run check:size
    if ($LASTEXITCODE -ne 0) { throw 'Frontend size check failed' }
    npm run build
    if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
  } finally { Pop-Location }
  node tools/bundle-core.mjs
  if ($LASTEXITCODE -ne 0) { throw 'Core bundling failed' }
  New-Item -ItemType Directory -Force dist | Out-Null
  go run ./tools/icon
  if ($LASTEXITCODE -ne 0) { throw 'Icon rendering failed' }
  go run github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json --arch amd64 --out cmd/xiaotao/rsrc
  if ($LASTEXITCODE -ne 0) { throw 'Icon resource generation failed' }
  $ldflags = "-s -w -H=windowsgui -X xiaotao/xt-client/internal/runtimeenv.Active=$($environment.Name) -X xiaotao/xt-client/internal/app.Version=$version"
  $outputDir = if ($Verify) { 'dist/verify' } else { 'dist' }
  New-Item -ItemType Directory -Force $outputDir | Out-Null
  $buildArgs = @('-trimpath', '-ldflags', $ldflags, '-o', "$outputDir/$($environment.Exe)", './cmd/xiaotao')
  & go build @buildArgs
  if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }
  if ($Verify) {
    Get-Item "$outputDir/$($environment.Exe)" | Select-Object FullName, Length
    return
  }
  Copy-Item -LiteralPath "dist/$($environment.Exe)" -Destination "dist/$versionedExe" -Force
  $desktopPath = [Environment]::GetFolderPath('Desktop')
  $shell = New-Object -ComObject WScript.Shell
  $shortcut = $shell.CreateShortcut((Join-Path $desktopPath $environment.Shortcut))
  $shortcut.TargetPath = Join-Path $PSScriptRoot "dist/$($environment.Exe)"
  $shortcut.WorkingDirectory = Join-Path $PSScriptRoot 'dist'
  $shortcut.IconLocation = $shortcut.TargetPath + ',0'
  $shortcut.Description = $environment.Description
  $shortcut.Save()
  $versionBytes = [System.Text.Encoding]::UTF8.GetBytes($version + [Environment]::NewLine)
  $versionFile.Position = 0
  $versionFile.Write($versionBytes, 0, $versionBytes.Length)
  $versionFile.SetLength($versionBytes.Length)
  $versionFile.Flush()
  Get-Item "dist/$($environment.Exe)", "dist/$versionedExe" | Select-Object FullName, Length
} finally {
  if ($null -ne $versionFile) { $versionFile.Dispose() }
  Pop-Location
}
