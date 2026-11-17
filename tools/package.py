#!/usr/bin/env python3
"""Package locally built Windows executable with verified official Wintun."""
import hashlib
from pathlib import Path
import shutil
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parent.parent
DIST = ROOT / 'dist'
PACKAGE = DIST / 'Blink-Windows-0.1.0-x64'
SHA256 = '07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51'
URL = 'https://www.wintun.net/builds/wintun-0.14.1.zip'


def main():
    exe = DIST / 'Blink.exe'
    if not exe.is_file():
        raise SystemExit('Build dist/Blink.exe first')
    archive = DIST / 'wintun-0.14.1.zip'
    if not archive.exists():
        with urllib.request.urlopen(URL, timeout=60) as response:
            data = response.read(16 * 1024 * 1024 + 1)
        if len(data) > 16 * 1024 * 1024:
            raise SystemExit('Wintun archive exceeds limit')
        if hashlib.sha256(data).hexdigest() != SHA256:
            raise SystemExit('Wintun SHA-256 mismatch')
        archive.write_bytes(data)
    if hashlib.sha256(archive.read_bytes()).hexdigest() != SHA256:
        raise SystemExit('Wintun SHA-256 mismatch')
    PACKAGE.mkdir(parents=True, exist_ok=True)
    shutil.copy2(exe, PACKAGE / 'Blink.exe')
    with zipfile.ZipFile(archive) as source:
        for name, target in [('wintun/bin/amd64/wintun.dll', 'wintun.dll'),
                             ('wintun/LICENSE.txt', 'WINTUN-LICENSE.txt')]:
            (PACKAGE / target).write_bytes(source.read(name))
    for name in ['README.md', 'LICENSE', 'THIRD-PARTY-NOTICES.md']:
        shutil.copy2(ROOT / name, PACKAGE / name)
    shutil.copytree(ROOT / 'docs', PACKAGE / 'docs', dirs_exist_ok=True)
    output = Path(shutil.make_archive(str(PACKAGE), 'zip', DIST, PACKAGE.name))
    digest = hashlib.sha256(output.read_bytes()).hexdigest()
    (DIST / 'SHA256SUMS').write_text(f'{digest}  {output.name}\n')
    print(f'{output}\nSHA-256: {digest}')


if __name__ == '__main__':
    main()
