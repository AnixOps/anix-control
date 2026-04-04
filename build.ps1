$ErrorActionPreference = "Stop"

Write-Host "Building Frontend..."
Push-Location web
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
$buildDir = Join-Path $PSScriptRoot "build"
if (-not (Test-Path $buildDir)) {
  New-Item -ItemType Directory -Path $buildDir | Out-Null
}
$oldGoWork = $env:GOWORK
try {
  # Ignore outer go.work so this module can build independently.
  $env:GOWORK = "off"
  go build -o (Join-Path $buildDir "v2board.exe") ./cmd/server
  if ($LASTEXITCODE -ne 0) { throw "go build failed" }
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
