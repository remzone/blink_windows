# Протоколы и форматы 0.2.0

| Формат / backend | Импорт и подключение |
| --- | --- |
| VLESS, VMess, Trojan, Shadowsocks URI | Общий парсер, Xray; ОС-интерфейс IPv4 |
| Hysteria2 URI | Xray |
| Xray JSON / массив | Поддерживаемые routing/DNS/streamSettings сохраняются |
| XDRIVE | JSON, закреплённый transport общего Xray |
| Clash YAML | Поддерживаемые узлы; группы/DNS/правила Clash не применяются, выдаётся уведомление |
| TUIC v5 / Hysteria v1 | sing-quic, без выборочного маршрутизатора |
| WG / AWG `.conf` и поддерживаемые URI | Пакетный backend, явные Address/AllowedIPs/DNS; IPv4/IPv6 полного профиля |
| Обычная подписка plain/base64 | До 1 МиБ / 1024 строк, отклонённые узлы считаются отдельно |
| Vokop v1 | VLESS → KCP/smux → DTLS → VK TURN, Auto/UDP/TCP |
| Vokop v2 | Также WG через DTLS/SRTP/WRAP |
| Vokop v3 | Требуется валидная `blink.routing.v1` в режиме selected; политика передаётся native |
| `turn.options` | Строго проверенные настройки транспорта, KCP, DNS/relay и pacing; streams наследуется из turn.streams, как в Android |
| Личные routing rules | Наследовать / all / selected; включения и исключения, исключения приоритетны |
| OpenVPN / AnyConnect / Forti | Не реализованы в общем ядре |

Таблица перечисляет реализованные пути кода, а не результаты live-проверки каждого протокола на Windows. Поддержка x86 включает отдельный PE32 EXE и x86 Wintun, а не переименование x64-файла.

Домены звонка VK: HTTPS `vk.com`, `vk.ru` и варианты с `www`; исходная ссылка сохраняется, ID сравнивается с учётом регистра. Подменные домены/query/fragment/кодированные пути не допускаются. Modern CAPTCHA — официальный Web SDK `@vkid/captcha` 2.0.0, visible/block flow, результат только для текущего неистёкшего challenge; legacy — ручной код изображения. Автоматического решения нет.

Выборочная маршрутизация общего ядра использует IPv4 и поддерживает Xray/стандартный WG. AWG extensions и TUIC/Hysteria v1 отвергают этот путь. IPv6 WG требует явного адреса/маршрутов/DNS, MTU ≥1280 и полного режима ядра. Xray/VLESS VK остаются IPv4, как мобильное ядро. Настройки могут быть приняты парсером, но отвергнуты при подключении, если комбинация backend/routing несовместима.

Частичные AllowedIPs, local exclusions и direct-правила Xray — явный прямой трафик. Диагностика следует этим правилам; успешный HTTPS-запрос не доказывает полного туннелирования всех приложений.
