# Архитектура 0.2.0

`cmd/blink/main_windows.go` — desktop host Wails 2.17.1/WebView2 и lifecycle/controller. `cmd/blink/ui` — встроенные HTML/CSS/JS, SVG-иконки и закреплённый официальный VK CAPTCHA SDK. Node не нужен для сборки готовых assets. Привилегированные Go bindings разрешены только локальному origin приложения; удалённый CAPTCHA iframe их не получает.

`internal/client` — переносимый импорт, модель и настройки. `internal/platform` — Windows DPAPI, Wintun, PowerShell и защита внешних сокетов. `native` — неизменённый снимок общего ядра iOS/Android с Windows module prefix; исходные native-тесты и лицензии сохранены.

## Мост UI

Frontend получает безопасный Snapshot: имена/типы/индексы профилей, статус, счётчики, настройки без ключей, срок/revision. UUID, конфигурации, приватные ключи и ссылка подписки не возвращаются. Временный challenge URL и CAPTCHA-ответ находятся в отдельном короткоживущем потоке UI. Введённая конфигурация очищается при закрытии импорт-окна.

Все lifecycle-данные защищены mutex; Wails bindings могут исполняться конкурентно. UI получает `blink:state` и первоначальный Snapshot. Подключение можно отменить ещё при подготовке: результат незавершённого старта проверяется до публикации tunnel handle. Закрытие ждёт завершения сетевой операции и очистки.

Browser profile создаётся во временном подкаталоге AppData Blink, не разделяется с Edge/system browser, удаляется при штатном завершении. Оставшиеся после аварии каталоги очищаются на следующем запуске. Браузер всё равно может временно кешировать свой runtime-контент; это не обещание отсутствия всех записей WebView2 на диск.

## Путь пакетов

```mermaid
flowchart LR
    UI[Embedded desktop UI] --> Bindings[Origin-checked Go bindings]
    Bindings --> Controller[Windows lifecycle / DPAPI]
    Apps[Windows applications] --> Routes[IPv4 / explicit WG IPv6 routes]
    Routes --> Wintun
    Wintun --> Core[Shared Xray / WG / AWG / QUIC core]
    Core --> Physical[Interface-bound sockets]
    Physical --> Server[VPN server / VK TURN]
```

Физические интерфейсы определяются до маршрутов VPN. `IP_UNICAST_IF` использует сетевой порядок байтов, `IPV6_UNICAST_IF` — порядок хоста; для IPv6 учитывается собственный physical default route. Пакеты Wintun передаются ядру в порядке чтения, обратный путь — через PacketWriter.

Для VK транспорт должен стать ready до настройки адресов/маршрутов. Обычный WG running означает запуск устройства, а не свежий handshake. UI и отдельная HTTPS-диагностика различают состояние туннеля и результат запроса. После отправки CAPTCHA тот же ID подавляется до нового challenge; post-answer ожидание — 60 секунд.

## Маршрутизация

Vokop v3 и `routing` преобразуются в `mobile.RoutingPolicy`, строго валидируются и передаются StartVK/StartProfileRouted. Личные включения/исключения объединяются с серверными. Для all+exclusions формируется selected IPv4-политика с двумя `/1`, чтобы исключения не игнорировались native all-mode.

Полные OS routes также задаются двумя `/1` каждого семейства с сохранением физического default route. Более специфичные локальные маршруты продолжают действовать. Full IPv4 без явного IPv6-адреса добавляет правило Firewall для IPv6; полный профиль WG с явным IPv6 его не блокирует. NRPT задаёт DNS профиля для системного резолвера. Выборочный маршрутизатор ядра использует IPv4; IPv6+selected отклоняется.

## Очистка и хранение

Конфигурация и Close сериализованы отдельным mutex. Очистка пытается выполнить каждую независимую операцию, даже если другая заблокирована политикой Windows. Она затрагивает только собственное NRPT, Firewall `Blink-IPv6` и интерфейс `Blink`. Ошибка очистки оставляет приложение доступным для Recover. Авария требует повторного запуска/`--recover`; постоянного kill switch и службы нет.

Состояние зашифровано DPAPI CurrentUser, записывается через временный файл, sync и MoveFileEx replacement. Формат 0.1.0 совместим: новые поля необязательны, старые настройки получают defaults. Перенос ciphertext в другую учётную запись не поддерживается.

## x86

Отдельная CGO-free сборка `GOARCH=386`, собственный manifest resource и x86 DLL Wintun. Структуры Windows API/DPAPI используются из x/sys, WebView2 — через закреплённую версию go-webview2 с x86 bindings. На x64 Windows тест x86 исполняется в WOW64; он не заменяет проверку отдельной 32-битной ОС.

Официальные API: [Wintun](https://www.wintun.net/), [Wails options](https://v2.wails.io/docs/reference/options/), [VK CAPTCHA SDK](https://www.npmjs.com/package/@vkid/captcha), [NRPT](https://learn.microsoft.com/en-us/powershell/module/dnsclient/add-dnsclientnrptrule), [Firewall](https://learn.microsoft.com/en-us/powershell/module/netsecurity/new-netfirewallrule), [Playwright WebView2](https://playwright.dev/docs/webview2).
