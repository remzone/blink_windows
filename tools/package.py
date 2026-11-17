#!/usr/bin/env python3
"""Package locally built Windows executable with verified official Wintun."""
import hashlib
from pathlib import Path
import shutil
import struct
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parent.parent
DIST = ROOT / 'dist'
VERSION = '0.3.0-preview.4'
SHA256 = '07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51'
URL = 'https://www.wintun.net/builds/wintun-0.14.1.zip'


def machine(data):
    if data[:2] != b'MZ' or len(data) < 64:
        raise SystemExit('Not a PE file')
    offset = struct.unpack_from('<I', data, 60)[0]
    if data[offset:offset + 4] != b'PE\0\0':
        raise SystemExit('Invalid PE signature')
    return struct.unpack_from('<H', data, offset + 4)[0]


def main():
    for arch in ['amd64', '386']:
        if not (DIST / arch / 'Blink.exe').is_file():
            raise SystemExit(f'Build dist/{arch}/Blink.exe first')
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
    sums = []
    for arch, label, dll_arch in [('amd64', 'x64', 'amd64'), ('386', 'x86', 'x86')]:
        package = DIST / f'Blink-Windows-{VERSION}-{label}'
        if package.exists():
            shutil.rmtree(package)
        package.mkdir(parents=True)
        expected = 0x8664 if arch == 'amd64' else 0x14c
        if machine((DIST / arch / 'Blink.exe').read_bytes()) != expected:
            raise SystemExit(f'EXE architecture mismatch: {arch}')
        shutil.copy2(DIST / arch / 'Blink.exe', package / 'Blink.exe')
        with zipfile.ZipFile(archive) as source:
            for name, target in [(f'wintun/bin/{dll_arch}/wintun.dll', 'wintun.dll'),
                                 ('wintun/LICENSE.txt', 'WINTUN-LICENSE.txt')]:
                data = source.read(name)
                if target == 'wintun.dll' and machine(data) != expected:
                    raise SystemExit(f'Wintun architecture mismatch: {arch}')
                (package / target).write_bytes(data)
        for name in ['README.md', 'LICENSE', 'THIRD-PARTY-NOTICES.md']:
            shutil.copy2(ROOT / name, package / name)
        shutil.copytree(ROOT / 'docs', package / 'docs')
        shutil.copy2(ROOT / 'internal/wfp/LICENSE', package / 'WFP-LICENSE.txt')
        shutil.copy2(ROOT / 'cmd/blink/ui/vendor/CAPTCHA-LICENSE.txt', package)
        shutil.copy2(ROOT / 'cmd/blink/ui/vendor/FLAG-ICONS-LICENSE.txt', package)
        output = Path(shutil.make_archive(str(package), 'zip', DIST, package.name))
        digest = hashlib.sha256(output.read_bytes()).hexdigest()
        sums.append(f'{digest}  {output.name}')
        print(f'{output}\nSHA-256: {digest}')
    (DIST / 'SHA256SUMS').write_text('\n'.join(sums) + '\n')


if __name__ == '__main__':
    main()
