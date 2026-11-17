#!/usr/bin/env python3
"""Read-only comparison of the vendored native source snapshot."""
from pathlib import Path
import sys

root = Path(__file__).resolve().parent.parent / 'native'
source = Path(sys.argv[1]).resolve()
paths = [Path('LICENSE'), Path('go.mod'), Path('go.sum')]
for name in ['mobile', 'share', 'vkturn', 'resources']:
    local = {p.relative_to(root) for p in (root / name).rglob('*') if p.is_file()}
    upstream = {p.relative_to(source) for p in (source / name).rglob('*') if p.is_file()}
    if local != upstream:
        raise SystemExit(f'File set differs in {name}: {local ^ upstream}')
    paths.extend(sorted(local))
for relative in paths:
    expected = (source / relative).read_bytes().replace(
        b'github.com/remzone/blink-android/native', b'github.com/remzone/blink_windows/native')
    if (root / relative).read_bytes() != expected:
        raise SystemExit(f'Snapshot differs: {relative}')
print(f'Native snapshot matches: {len(paths)} files (module prefix normalized)')
