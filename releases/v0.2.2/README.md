# Blink Windows 0.2.2

[Скачать Windows x64](https://github.com/remzone/blink_windows/raw/refs/heads/main/releases/v0.2.2/Blink-Windows-0.2.2-x64.zip) · [Скачать Windows x86, 32 бит](https://github.com/remzone/blink_windows/raw/refs/heads/main/releases/v0.2.2/Blink-Windows-0.2.2-x86.zip) · [Контрольные суммы](SHA256SUMS)

Выбор и поиск по большому списку после импорта, вкладки «Все / Обычный VPN / TURN», локальные SVG-флаги. Требуются Windows 10/11 x64 либо Windows 10 x86, WebView2 Runtime и права администратора. Распакуйте архив целиком: `Blink.exe` и `wintun.dll` должны лежать рядом.

Обе сборки получены кросс-компиляцией на Linux. Тесты frontend с 120 синтетическими серверами и переносимые Go-тесты прошли; реальное подключение на Windows этим не подтверждается. [Проверки](../../docs/validation.md) · [Скриншоты и установка](../../README.md) · [Изменения](../../docs/CHANGELOG.md).

EXE не подписан Authenticode. В пакеты включены официальный Wintun соответствующей архитектуры, документация и лицензии. Исходники: https://github.com/remzone/blink_windows.
