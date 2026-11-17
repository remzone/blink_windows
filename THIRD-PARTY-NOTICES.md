# Third-party notices

Blink Windows integration is GPL-3.0-only; see LICENSE. Dependencies retain their own licenses; the module graph and checksums are pinned by the root/native go.mod and go.sum files.

| Component | Origin / license records |
| --- | --- |
| Shared Blink native wrappers | remzone/blink-android and remzone/blink_ios; GPL-3.0-only; native/LICENSE |
| VK TURN client | native/vkturn/LICENSE and native/vkturn/UPSTREAM.md, including pinned vk-turn-proxy provenance |
| URI import parser | native/share/LICENSE, adapted from XTLS/libXray (MIT) |
| Xray-core | github.com/xtls/xray-core; MPL-2.0 |
| WireGuard Go | golang.zx2c4.com/wireguard; MIT |
| AmneziaWG Go | github.com/amnezia-vpn/amneziawg-go/v3; MIT |
| TUIC / legacy Hysteria | github.com/sagernet/sing-quic; GPL-3.0-or-later |
| Wintun Go bindings | golang.zx2c4.com/wintun; MIT |
| Wintun signed DLL | Official prebuilt 0.14.1 distribution; its separate permissive license is copied to WINTUN-LICENSE.txt in the package. Driver source has different terms; consult https://www.wintun.net/ |
| Geodata | native/resources/LICENSE and native/resources/UPSTREAM.md |
| Go Windows APIs | golang.org/x/sys/windows; BSD-3-Clause |
| YAML parser | gopkg.in/yaml.v3; MIT / Apache-2.0 |
| Manifest resource generator (build only) | github.com/akavel/rsrc v0.10.2; MIT |

Other transitive dependencies include Pion networking libraries, KCP/smux, QUIC/TLS libraries and their own license files in their source modules. These notices do not relicense those components. Consult the pinned source modules when redistributing binaries. Sources for this client are available at https://github.com/remzone/blink_windows and upstream module sources remain identifiable through go.mod/go.sum.
