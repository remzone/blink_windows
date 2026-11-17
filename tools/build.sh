#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p dist
export CGO_ENABLED=0
# A single wildcard manifest is valid for both PE architectures.
for arch in amd64 386; do
  rm -f cmd/blink/resource_windows_*.syso
  go run github.com/akavel/rsrc@v0.10.2 -arch "$arch" -manifest assets/blink.manifest -ico assets/blink.ico -o "cmd/blink/resource_windows_${arch}.syso"
  mkdir -p "dist/$arch"
  GOOS=windows GOARCH="$arch" go build -tags production -trimpath -ldflags='-s -w -H windowsgui -checklinkname=0' -o "dist/$arch/Blink.exe" ./cmd/blink
done
python3 tools/package.py
