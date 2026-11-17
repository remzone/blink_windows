$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)
$env:CGO_ENABLED = '0'
# Resource generator runs on the host architecture, compiler targets each output.
Remove-Item Env:GOOS -ErrorAction SilentlyContinue
Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
foreach ($arch in @('amd64','386')) {
    Remove-Item cmd/blink/resource_windows_*.syso -ErrorAction SilentlyContinue
    go run github.com/akavel/rsrc@v0.10.2 -arch $arch -manifest assets/blink.manifest -ico assets/blink.ico -o "cmd/blink/resource_windows_$arch.syso"
    if ($LASTEXITCODE -ne 0) { throw 'Manifest generation failed' }
    New-Item -ItemType Directory -Force "dist/$arch" | Out-Null
    $env:GOOS = 'windows'
    $env:GOARCH = $arch
    go build -tags production -trimpath '-ldflags=-s -w -H windowsgui -checklinkname=0' -o "dist/$arch/Blink.exe" ./cmd/blink
    if ($LASTEXITCODE -ne 0) { throw "Build failed: $arch" }
    Remove-Item Env:GOOS
    Remove-Item Env:GOARCH
}
python tools/package.py
if ($LASTEXITCODE -ne 0) { throw 'Packaging failed' }
