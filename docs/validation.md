# Проверка Windows 0.2.2

Работа выполняется на Linux. Сборка, тестирование frontend и реальный Windows runtime — разные уровни проверки.

## Выполнено локально

- Общий core и Windows-импортёр: `go test -race ./internal/... ./native/...` — успешно. Native tests используют синтетические локальные peers; live-провайдеры не включались.
- `go vet` Windows-пакетов для amd64 и 386 — успешно.
- Кросс-компиляция EXE для обеих архитектур: x64 PE32+ AMD64 и x86 PE32 Intel 80386. Это отдельные сборки с подходящими resource objects, не переименованные файлы.
- Windows-тесты DPAPI, сетевой валидации, прямого чтения route table и изоляции PowerShell stderr скомпилированы отдельно для amd64/386. На Linux они не запускаются.
- 89 native-файлов совпадают с Android-снимком после нормализации module prefix.
- Официальный Wintun ZIP проверен по закреплённому SHA-256. Packager выбирает AMD64/x86 DLL соответственно и создаёт два ZIP с лицензиями и SHA256SUMS.
- Переносимые regression tests выбора интерфейса: метрика route+interface, исключение Blink/offline/нулевого индекса, отсутствие default route, детерминированные равные метрики и защита суммы от переполнения.
- Frontend Playwright/Chromium: вкладки VPN/TURN выбирают настоящий профиль, запоминают выбор в сеансе, пустой режим не подключает профиль другого типа, локальный флаг Казахстана загружается, нейтральные подписи интерфейса; навигация, connect/disconnect, профиль/поиск, ошибки импорта и очистка поля, настройки, диагностика, компактное окно, reduced motion и вывод недоверенного имени через textContent.
- Официальный CAPTCHA SDK: mock frame с тем же origin/source-протоколом, отклонение поддельного postMessage от другого source, приём результата из ожидаемого mock iframe, одноразовая передача и отмена. Реальные ресурсы поставщика при этом перехвачены и не запрашиваются.

Проверены Vokop v3, наследование routing/turn.options и явные локальные overrides, all+exclusions, bounds настроек, просрочка, будущая дата, rollback revision, дубликаты JSON, четыре официальных домена звонка, безопасные HTTPS-ссылки. Старый формат состояния 0.1.0 сохраняется.

## Проверка в GitHub Actions

Workflow запускается при push в main, pull request или вручную. Он настроен на:

1. Windows unit tests импортёра/DPAPI/сетевой валидации в x64.
2. Те же тесты в x86 под WOW64.
3. Общие native tests.
4. Сборку и упаковку обоих EXE.
5. Запуск настоящих desktop EXE с WebView2 и Playwright через CDP: получение Go Snapshot, импорт синтетического VLESS без подключения, DPAPI-файл без открытого UUID, сохранение настроек и screenshot.

Smoke-сценарий **не стартует VPN и не меняет маршруты/Firewall**. Порт remote debugging задаётся только окружением теста; в обычном запуске приложение его не включает. [Первый запуск CI](https://github.com/remzone/blink_windows/actions/runs/44128014115) завершился **до старта runner**: GitHub сообщил `The job was not started because your account is locked due to a billing issue.` Ни одного Windows test/smoke step в этом запуске не выполнялось, CI artifacts не созданы. Это ограничение аккаунта, не результат компиляции или теста. После устранения billing-блокировки workflow можно запустить вручную; успешного Windows-результата пока нет.

## Как воспроизвести frontend-тест

Нужны Node.js и Playwright; для Windows-сборки они не нужны.

```bash
npm install --prefix work/ui-test --ignore-scripts --no-audit --no-fund playwright@1.58.2
NODE_PATH="$PWD/work/ui-test/node_modules" BLINK_CHROMIUM=/path/to/chrome node tools/test-ui.cjs
```

Если BLINK_CHROMIUM не задан, используется браузер, установленный Playwright. Снимки сохраняются в `work/ui`. Mock backend не подтверждает работу Win32/COM или VPN.

На Windows с собранными EXE и установленным WebView2:

```powershell
$env:NODE_PATH = "$PWD/work/ui-test/node_modules"
node tools/test-windows.cjs amd64
node tools/test-windows.cjs 386
```

## Что остаётся проверить на целевых устройствах

- Физическую Windows 10 x86, а не только 32-битный процесс в WOW64.
- Wintun, адреса, маршруты `/1`, NRPT, Firewall, cleanup/Recover и отсутствие DNS/IPv6-утечек на Windows.
- Реальные IPv4 VLESS/REALITY, WG/AWG, TUIC, Hysteria, XDRIVE; dual-stack WG с явным IPv6 и сервером, который его обслуживает.
- Live TURN и ручную CAPTCHA с действующей подпиской; mock SDK bridge не гарантирует совместимость настоящего задания поставщика.
- Корпоративные политики, параллельные VPN, сторонние антивирусы, сон/пробуждение, смену Wi-Fi/Ethernet, аварию процесса и восстановление.
- Масштабирование 100/150/200%, многомониторность, Narrator и крупные списки на реальной Windows.

EXE не имеет Authenticode-подписи; установщика, постоянного kill switch, службы и автообновления нет. Эти возможности не заявляются как реализованные.


## Регрессии 0.2.2

Frontend проверяет подписку из 120 синтетических серверов: автоматический выбор после импорта, чистый поиск, внутреннюю прокрутку, фильтры 120/60/60, поиск страны на английском по русскому имени, выбор последнего сервера клавишей Enter, пустой результат и сброс. Отдельно проверяются быстрый поиск на главной и перенос вставленной ссылки в импорт без вызова backend до отправки формы. Screenshots создаются на фиктивных реквизитах; данные пользовательских подписок не используются.

ZIP x64/x86 находятся в `releases/v0.2.2`, хеши — в `SHA256SUMS`. Они собраны локально и не обозначены как успешно проверенные реальным Windows runner.
