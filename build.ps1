$ErrorActionPreference = "Stop"
$repoRoot = $PSScriptRoot
$frontendDir = Join-Path $repoRoot "web"
$buildDir = Join-Path $repoRoot "build"

Write-Host "Building Frontend..."
if (-not (Test-Path $frontendDir)) {
  throw "Frontend directory not found: $frontendDir"
}

Push-Location $frontendDir
try {
  npm install
  if ($LASTEXITCODE -ne 0) { throw "npm install failed" }

  npm run build
  if ($LASTEXITCODE -ne 0) { throw "npm run build failed" }
}
finally {
  Pop-Location
}

Write-Host "Building Backend..."
if (-not (Test-Path $buildDir)) {
  New-Item -ItemType Directory -Path $buildDir | Out-Null
}
$oldGoWork = $env:GOWORK
try {
  # Ignore outer go.work so this module can build independently.
  $env:GOWORK = "off"
  Push-Location $repoRoot
  try {
    go build -o (Join-Path $buildDir "v2board.exe") ./cmd/server
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }
  }
  finally {
    Pop-Location
  }
}
finally {
  if ($null -ne $oldGoWork) {
    $env:GOWORK = $oldGoWork
  }
  else {
    Remove-Item Env:GOWORK -ErrorAction SilentlyContinue
  }
}

Write-Host "Build Complete!"
