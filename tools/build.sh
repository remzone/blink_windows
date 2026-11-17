#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p dist
export CGO_ENABLED=0
# Embed Windows UAC manifest. Generated object is excluded from Git.
go run github.com/akavel/rsrc@v0.10.2 -arch amd64 -manifest assets/blink.manifest -o cmd/blink/resource_windows_amd64.syso
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w -H windowsgui -checklinkname=0' -o dist/Blink.exe ./cmd/blink
python3 tools/package.py
