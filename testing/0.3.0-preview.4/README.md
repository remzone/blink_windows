# Диагностический preview 0.3.0-preview.4

Preview.3 отозван: пользовательский журнал подтвердил отказ New-NetFirewallRule с Windows 0x80070057. Preview.4 использует dynamic Windows Filtering Platform session вместо этой команды. Это **не подтверждённый рабочий релиз**: новое реальное подключение на Windows ещё нужно проверить.

[Скачать x64](https://github.com/remzone/blink_windows/raw/refs/heads/main/testing/0.3.0-preview.4/Blink-Windows-0.3.0-preview.4-x64.zip) · [Скачать x86, 32 бит](https://github.com/remzone/blink_windows/raw/refs/heads/main/testing/0.3.0-preview.4/Blink-Windows-0.3.0-preview.4-x86.zip) · [SHA256SUMS](SHA256SUMS)

1. Полностью закройте старый Blink и распакуйте новый ZIP целиком: EXE и Wintun DLL рядом. Нужен WebView2 Runtime и UAC для сетевых операций.
2. Сохранённые профили и независимые VPN/TURN-подписки остаются в DPAPI-хранилище текущего пользователя Windows; заново добавлять сохранившиеся данные не нужно.
3. Подключитесь. После готовности транспорта появится «Включаем временную защиту IPv6», затем настройка сети. Новый механизм удаляет свои фильтры при отключении/завершении процесса; в профиль с явным IPv6 и частичными OS routes глобальный фильтр не добавляется.
4. При ошибке: **Диагностика → Открыть папку журнала**, пришлите `blink.log` и `blink.log.1`, если он есть. Путь `%APPDATA%\Blink\logs`. Для WFP там будут события `windows.filter.*`, имя операции и прямой код Windows API; ключи и ссылки подписок не пишутся. Отправки по сети нет.

Переносимые race tests, browser tests и компиляция x64/x86 с ABI assertions выполнены локально. Реальные Windows WFP/driver/VPN в этой среде не запускались. EXE не подписан Authenticode.

[Документация и screenshots](../../README.md) · [Проверки](../../docs/validation.md) · [Изменения](../../docs/CHANGELOG.md).
