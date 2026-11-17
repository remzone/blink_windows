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
| Desktop host | github.com/wailsapp/wails/v2 v2.17.1; MIT |
| WebView2 bindings | github.com/wailsapp/go-webview2 v1.0.22; MIT and embedded loader notices |
| VK ID CAPTCHA Web SDK | @vkid/captcha 2.0.0, V Kontakte LLC; MIT; cmd/blink/ui/vendor/VKID-CAPTCHA-LICENSE.txt and UPSTREAM.json |
| Country flags | flag-icons 7.5.0, Panayiotis Lipiridis; MIT; cmd/blink/ui/vendor/FLAG-ICONS-LICENSE.txt and FLAG-ICONS-UPSTREAM.json |
| Windows IP Helper bindings | golang.zx2c4.com/wireguard/windows/tunnel/winipcfg v1.1.1; MIT |
| Manifest resource generator (build only) | github.com/akavel/rsrc v0.10.2; MIT |

Other transitive dependencies include Pion networking libraries, KCP/smux, QUIC/TLS libraries and their own license files in their source modules. These notices do not relicense those components. Consult the pinned source modules when redistributing binaries. Sources for this client are available at https://github.com/remzone/blink_windows and upstream module sources remain identifiable through go.mod/go.sum.
