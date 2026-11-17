# Происхождение исходников

За основу взяты существующие локальные клиенты:

- [remzone/blink-android](https://github.com/remzone/blink-android), HEAD `b205d622c1247ba221d8c6b4f5d2883aef61d7d0`: скопированы исходники `native/{mobile,share,vkturn,resources}`, `go.mod`, `go.sum`, LICENSE и синтетические fixtures. Снимок сделан из рабочего дерева 17 ноября 2026; HEAD указывает происхождение checkout и не удостоверяет отсутствие локальных изменений. APK/AAR/JAR, рабочие каталоги и реальные реквизиты не копируются в публикуемые файлы.
- [remzone/blink_ios](https://github.com/remzone/blink_ios), HEAD `e198d6111e22cd36590ad026e2b1758d82e32495`: изучены модели профилей, контракт Vokop, UI и документация единого ядра. Swift UI/NetworkExtension не используются в Windows; Go-пути уже общие с Android.
- Для Windows добавлены `cmd/blink`, `internal/client`, `internal/platform`, скрипты сборки, упаковки и документация. Общий core переведён с module prefix `github.com/remzone/blink-android/native` на `github.com/remzone/blink_windows/native` без изменения wire-протоколов.

Зафиксированные upstream-версии и лицензии находятся в `native/go.mod`, `native/go.sum`, `native/vkturn/UPSTREAM.md`, `native/share/LICENSE` и `native/resources/UPSTREAM.md`. Используется Xray commit `676eaf66f8f0` (v26.11.9), версия модуля `v1.260327.1-0.20261109052709-676eaf66f8f0`.

В корневом модуле через replace дополнительно закреплён `google.golang.org/genproto` на `v0.0.0-20260817212433-ac3dfec99bb1`: это устраняет конфликт старого монолитного genproto и нового подпакета rpc в workspace. Родные go.mod/go.sum снимка сохранены, кроме module prefix.

## Обновление ядра

Не собирайте Windows через относительную replace-ссылку на соседний Android/iOS checkout. Обновление должно переносить reviewable снимок четырёх каталогов и module/sum-файлов, заменять только import prefix, сохранять лицензии, обновлять provenance и проходить тесты и Windows-сборку.

Скрипт `tools/check-native.py /path/to/blink-android/native` сверяет все файлы снимка с указанным источником, игнорируя только преобразование import prefix. Он ничего не записывает в источник. Различие рабочей копии относительно HEAD следует отдельно учитывать при привязке к upstream commit.

Для Wintun используется официальный архив 0.14.1 с SHA-256 `07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51`. ZIP не входит в исходники; packager проверяет архив и добавляет лицензию DLL в сборку.

## Desktop 0.2.0

Вместо Win32-формы используется Wails 2.17.1 с go-webview2 1.0.22. Версии закреплены root go.mod/go.sum. Архитектуры x64/x86 собираются отдельно; официальный архив Wintun содержит соответствующие DLL.

Официальный `@vkid/captcha` 2.0.0 получен из npm от автора VK ID <devsupport@corp.vk.com>; локальная UMD-сборка скопирована без изменения, вместе с MIT LICENSE и SHA-256 в `cmd/blink/ui/vendor/UPSTREAM.json`. Адаптер `captcha.js` только монтирует visible/block challenge по URL ядра и передаёт одноразовый ответ обратно; никакого решателя нет.

Дизайн основан на фактических мобильных `BlinkTheme.swift` / `BlinkStyle.java`; Windows HTML/CSS, векторы и EXE-иконка написаны в этом репозитории. `tools/make-icon.py` воспроизводит исходную иконку, не использует чужие растровые изображения. Скриншоты docs сняты с синтетическими данными на тех же frontend assets.

## Флаги 0.2.1

`flag-icons` 7.5.0: неизменённые `flags/4x3/[a-z][a-z].svg` из npm-архива. MIT-лицензия и SHA-256 архива находятся в `cmd/blink/ui/vendor/FLAG-ICONS-{LICENSE.txt,UPSTREAM.json}`. Источник: https://github.com/lipis/flag-icons. PNG, внешние CDN и генерация изображения не используются.
