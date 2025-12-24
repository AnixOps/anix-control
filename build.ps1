Write-Host "Building Frontend..."
Push-Location web
npm install
npm run build
Pop-Location

Write-Host "Building Backend..."
go build -o v2board.exe ./cmd/server

Write-Host "Build Complete!"
