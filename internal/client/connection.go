// SPDX-License-Identifier: GPL-3.0-only
package client

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/remzone/blink_windows/internal/diagnostics"
)

type ConnectionEvent struct {
	At     int64            `json:"at"`
	Code   string           `json:"code"`
	Fields map[string]int64 `json:"fields,omitempty"`
}
type ConnectionTrace struct {
	Stage          string            `json:"stage"`
	Detail         string            `json:"detail"`
	Error          string            `json:"error,omitempty"`
	Code           string            `json:"code,omitempty"`
	StartedAt      int64             `json:"startedAt"`
	ElapsedSeconds int64             `json:"elapsedSeconds"`
	Events         []ConnectionEvent `json:"events"`
	Problem        string            `json:"problem,omitempty"`
}

func NewConnectionTrace(now time.Time) ConnectionTrace {
	return ConnectionTrace{StartedAt: now.UnixMilli(), Events: []ConnectionEvent{}}
}
func (t *ConnectionTrace) add(code string, fields map[string]int64, now time.Time) {
	if len(t.Events) >= 100 {
		t.Events = append([]ConnectionEvent(nil), t.Events[len(t.Events)-99:]...)
	}
	t.Events = append(t.Events, ConnectionEvent{At: now.UnixMilli() - t.StartedAt, Code: code, Fields: fields})
	diagnostics.Event(code, map[string]any{"attemptStartedAt": t.StartedAt, "elapsedMs": now.UnixMilli() - t.StartedAt, "values": fields})
}
func (t *ConnectionTrace) SetStage(stage string, now time.Time) {
	if stage == "network" || stage == "connected" {
		t.Problem = ""
	}
	if t.Stage == stage {
		return
	}
	descriptions := map[string]string{"recovery": "Очищаем оставшиеся правила сети Blink", "adapter": "Создаём сетевой адаптер Windows", "engine": "Запускаем VPN-ядро", "transport": "Устанавливаем транспорт TURN", "network": "Применяем адреса, DNS и маршруты Windows", "captcha": "Ожидаем ручную проверку", "connected": "Сетевой интерфейс настроен", "stopping": "Останавливаем соединение и очищаем сеть", "disconnected": "Соединение остановлено"}
	t.Stage = stage
	t.Detail = descriptions[stage]
	t.add("windows."+stage, nil, now)
}
func (t *ConnectionTrace) Fail(code, message string, now time.Time) {
	t.ElapsedSeconds = (now.UnixMilli() - t.StartedAt) / 1000
	message = regexp.MustCompile(`(?i)vk[ _-]*turn|vk`).ReplaceAllString(message, "TURN")
	t.Error = message
	t.Code = code
	t.Stage = "failed"
	t.Detail = message
	t.add("windows.failed", nil, now)
	diagnostics.Event("connection.failure", map[string]any{"code": code, "attemptStartedAt": t.StartedAt})
}

// Only known event IDs and numeric fields cross into the UI/report. The native
// ring contains no raw error strings, URLs, credentials, server names or packets.
func (t *ConnectionTrace) Consume(raw string, now time.Time) {
	var rows []struct {
		Event  string           `json:"event"`
		Fields map[string]int64 `json:"fields"`
	}
	if json.Unmarshal([]byte(raw), &rows) != nil {
		return
	}
	known := map[string]string{
		"native.vk.request": "turn.authorization_request", "native.vk.response": "turn.authorization_response", "native.vk.credentials_failed": "turn.authorization_rejected",
		"native.vk.retry": "turn.retry", "native.vk.turn_resolve": "turn.resolve", "native.vk.turn_connected": "turn.connected", "native.vk.turn_allocated": "turn.allocated", "native.vk.smux_ready": "turn.ready", "native.vk.smux_closed": "turn.session_closed",
		"native.xray.error": "vpn.core_error", "native.wireguard.error": "vpn.packet_error",
	}
	allowed := map[string]bool{"stage": true, "httpStatus": true, "errorCode": true, "stream": true, "count": true, "mode": true, "elapsedMs": true}
	for _, row := range rows {
		code, ok := known[row.Event]
		if !ok {
			continue
		}
		fields := map[string]int64{}
		for k, v := range row.Fields {
			if allowed[k] {
				fields[k] = v
			}
		}
		t.add(code, fields, now)
		switch code {
		case "turn.authorization_request":
			if t.Stage == "transport" {
				t.Detail = fmt.Sprintf("Авторизация TURN: шаг %d из 4", fields["stage"])
			}
		case "turn.authorization_response":
			if fields["httpStatus"] >= 400 {
				t.Problem = fmt.Sprintf("Служба авторизации TURN вернула HTTP %d", fields["httpStatus"])
			}
		case "turn.authorization_rejected":
			t.Problem = fmt.Sprintf("Авторизация TURN отклонена (код %d)", fields["errorCode"])
		case "turn.resolve":
			if t.Stage == "transport" {
				t.Detail = "Определяем адрес реле TURN"
			}
		case "turn.connected":
			if t.Stage == "transport" {
				t.Detail = "Реле TURN доступно; создаём транспортную сессию"
			}
		case "turn.allocated":
			if t.Stage == "transport" {
				t.Detail = "TURN выделил канал; согласуем защищённый транспорт"
			}
		case "turn.retry":
			stages := map[int64]string{1: "Нет соединения с реле TURN", 2: "Не удалось запустить TURN-клиент", 3: "Реле TURN не выделило канал", 4: "Не удалось подготовить защищённый транспорт", 5: "Сервер не подтвердил DTLS-сессию", 6: "Не удалось создать канал KCP", 7: "Не удалось создать мультиплексор", 8: "Не удалось определить адрес сервера через DNS"}
			if reason := stages[fields["stage"]]; reason != "" {
				t.Problem = reason
			}
			if t.Stage == "transport" && t.Problem != "" {
				t.Detail = t.Problem + ". Повторяем попытку…"
			}
		}
	}
}
func (t ConnectionTrace) TimeoutMessage() string {
	if t.Problem != "" {
		return t.Problem + ". Подключение остановлено по тайм-ауту. Подробности — в отчёте подключения."
	}
	switch t.Stage {
	case "recovery":
		return "Windows не завершила очистку прежних правил Blink за 30 секунд. Выполните восстановление сети в настройках и сохраните отчёт."
	case "adapter":
		return "Windows не завершила создание адаптера за 45 секунд. Проверьте драйвер Wintun, права администратора и конфликт с другим VPN."
	case "engine":
		return "VPN-ядро не запустилось за 45 секунд. Подключение отменено; сохраните отчёт подключения."
	case "network":
		return "Windows не применила настройки сети за 30 секунд. Подключение отменено; проверьте права и сетевые политики."
	case "captcha":
		return "Истекло время ручной проверки. Подключитесь заново."
	default:
		return "Транспорт TURN не стал готов за 90 секунд. Проверьте доступность сервера и параметры транспорта; сохраните отчёт подключения."
	}
}
func (t ConnectionTrace) Report(version, arch string) string {
	// Keep this export independent of State/Profile: no future credential field
	// can accidentally enter it via a marshalled application snapshot.
	value := struct {
		Version    string          `json:"version"`
		Arch       string          `json:"arch"`
		Connection ConnectionTrace `json:"connection"`
	}{version, arch, t}
	b, _ := json.MarshalIndent(value, "", "  ")
	return string(b)
}
func (t ConnectionTrace) SafeProblemCode() string {
	if strings.Contains(t.Problem, "DNS") {
		return "DNS_RESOLUTION"
	}
	if strings.Contains(t.Problem, "авторизации") || strings.Contains(t.Problem, "Авторизация") {
		return "TURN_AUTHORIZATION"
	}
	return "TRANSPORT_TIMEOUT"
}
