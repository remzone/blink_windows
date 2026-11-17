$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)
New-Item -ItemType Directory -Force dist | Out-Null
$env:CGO_ENABLED = '0'
go run github.com/akavel/rsrc@v0.10.2 -arch amd64 -manifest assets/blink.manifest -o cmd/blink/resource_windows_amd64.syso
if ($LASTEXITCODE -ne 0) { throw 'Manifest generation failed' }
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
go build -trimpath '-ldflags=-s -w -H windowsgui -checklinkname=0' -o dist/Blink.exe ./cmd/blink
if ($LASTEXITCODE -ne 0) { throw 'Build failed' }
python tools/package.py
if ($LASTEXITCODE -ne 0) { throw 'Packaging failed' }
