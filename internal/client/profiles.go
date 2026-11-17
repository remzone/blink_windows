// SPDX-License-Identifier: GPL-3.0-only
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/remzone/blink_windows/native/mobile"
)

const MaxDocument = 1 << 20

type Profile struct {
	ExternalID string          `json:"externalId,omitempty"`
	ID         string          `json:"id,omitempty"`
	SourceID   string          `json:"sourceId,omitempty"`
	Routing    json.RawMessage `json:"routing,omitempty"`
	Name       string          `json:"name"`
	Kind       string          `json:"kind"`
	Raw        json.RawMessage `json:"raw"`
	Addresses  []string        `json:"addresses"`
	DNS        []string        `json:"dns"`
	Routes     []string        `json:"routes"`
	MTU        int             `json:"mtu"`
}
type State struct {
	Sources         []Source          `json:"sources,omitempty"`
	Selections      map[string]string `json:"selections,omitempty"`
	ExpiresAt       int64             `json:"expiresAt,omitempty"`
	VKOptions       *VKOptions        `json:"vkOptions,omitempty"`
	Routing         *RoutingOptions   `json:"routingOptions,omitempty"`
	Profiles        []Profile         `json:"profiles"`
	Selected        int               `json:"selected"`
	SubscriptionURL string            `json:"subscriptionURL,omitempty"`
	Revision        int64             `json:"revision"`
}
type ImportResult struct {
	ExpiresAt int64
	Profiles  []Profile
	Rejected  int
	Notice    string
	Revision  int64
}

func Parse(raw string, now time.Time, minimum int64) (ImportResult, error) {
	var marker struct {
		Schema string `json:"schema"`
	}
	_ = json.Unmarshal([]byte(raw), &marker)
	if strings.HasPrefix(marker.Schema, "vokop.subscription.") {
		return parseVokop(raw, now, minimum)
	}
	data, err := mobile.ImportProfiles(raw)
	if err != nil {
		return ImportResult{}, err
	}
	var doc struct {
		Profiles []json.RawMessage `json:"profiles"`
		Rejected []json.RawMessage `json:"rejected"`
		Notice   string            `json:"notice"`
	}
	if err = json.Unmarshal([]byte(data), &doc); err != nil {
		return ImportResult{}, err
	}
	result := ImportResult{Rejected: len(doc.Rejected), Notice: doc.Notice}
	for _, r := range doc.Profiles {
		var p mobile.ImportedProfile
		if err = json.Unmarshal(r, &p); err != nil {
			return ImportResult{}, err
		}
		result.Profiles = append(result.Profiles, Profile{Name: p.Name, Kind: p.Kind, Raw: r, Addresses: p.Addresses, DNS: p.DNS, Routes: p.Routes, MTU: p.MTU})
	}
	return result, nil
}

var callPattern = regexp.MustCompile(`^https://(?:www\.)?vk\.(?:com|ru)/call/join/[A-Za-z0-9_=-]+$`)
var uuidPattern = regexp.MustCompile(`^[a-fA-F0-9]{8}-(?:[a-fA-F0-9]{4}-){3}[a-fA-F0-9]{12}$`)
var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
var hostPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)

func parseVokop(raw string, now time.Time, minimum int64) (ImportResult, error) {
	var d struct {
		Schema   string          `json:"schema"`
		Revision int64           `json:"revision"`
		Issued   int64           `json:"issued_at"`
		Expires  int64           `json:"expires_at"`
		Routing  json.RawMessage `json:"routing"`
		Profiles []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Backend   string `json:"backend"`
			WireGuard string `json:"wireguard"`
			Vless     struct {
				UUID string `json:"uuid"`
			} `json:"vless"`
			Turn struct {
				Host     string          `json:"peer_host"`
				Port     int             `json:"peer_port"`
				Call     string          `json:"vk_call_url"`
				Streams  int             `json:"streams"`
				Protocol string          `json:"protocol"`
				WrapKey  string          `json:"wrap_key"`
				Options  json.RawMessage `json:"options"`
			} `json:"turn"`
		} `json:"profiles"`
		VPNProfiles []string `json:"vpn_profiles"`
	}
	if len(raw) > MaxDocument || !uniqueJSON(raw) || json.Unmarshal([]byte(raw), &d) != nil {
		return ImportResult{}, errors.New("Некорректный документ Vokop")
	}
	if d.Schema != "vokop.subscription.v1" && d.Schema != "vokop.subscription.v2" && d.Schema != "vokop.subscription.v3" {
		return ImportResult{}, errors.New("Неизвестная версия подписки Vokop")
	}
	if d.Revision < 1 || d.Revision > 9007199254740991 || d.Revision < minimum || d.Issued < 0 || d.Issued > now.Unix()+300 || d.Expires <= now.Unix() || d.Expires <= d.Issued || d.Expires > 253402300799 {
		return ImportResult{}, errors.New("Подписка истекла, датирована будущим или имеет устаревшую версию")
	}
	if len(d.Profiles) < 1 || len(d.Profiles) > 16 || len(d.VPNProfiles) > 128 {
		return ImportResult{}, errors.New("Превышен лимит профилей")
	}
	var routing *mobile.RoutingPolicy
	if len(d.Routing) > 0 && string(d.Routing) != "null" {
		routing = &mobile.RoutingPolicy{}
		dec := json.NewDecoder(strings.NewReader(string(d.Routing)))
		dec.DisallowUnknownFields()
		if dec.Decode(routing) != nil || routing.Validate() != nil {
			return ImportResult{}, errors.New("Некорректные правила маршрутизации панели")
		}
	}
	if d.Schema == "vokop.subscription.v3" && (routing == nil || routing.Mode != "selected") {
		return ImportResult{}, errors.New("Vokop v3 требует выборочных правил маршрутизации")
	}
	result := ImportResult{Revision: d.Revision, ExpiresAt: d.Expires}
	ids := map[string]bool{}
	for _, v := range d.Profiles {
		if !idPattern.MatchString(v.ID) || ids[v.ID] || !uuidPattern.MatchString(v.Vless.UUID) || !hostPattern.MatchString(v.Turn.Host) || !strings.Contains(v.Turn.Host, ".") || !callPattern.MatchString(v.Turn.Call) || v.Turn.Port < 1 || v.Turn.Port > 65535 || v.Turn.Streams < 1 || v.Turn.Streams > 16 || len([]rune(v.Name)) < 1 || len([]rune(v.Name)) > 100 || strings.ContainsAny(v.Name, "\r\n\x00") {
			return ImportResult{}, errors.New("Некорректный профиль Vokop")
		}
		ids[v.ID] = true
		backend, protocol := v.Backend, v.Turn.Protocol
		if backend == "" {
			backend = "vless"
		}
		if protocol == "" {
			protocol = "kcp"
		}
		if backend != "vless" && backend != "wireguard" || backend == "vless" && (protocol != "kcp" || v.WireGuard != "") || backend == "wireguard" && (d.Schema == "vokop.subscription.v1" || v.WireGuard == "" || (protocol != "dtls" && protocol != "srtp" && protocol != "wrap")) {
			return ImportResult{}, errors.New("Несовместимые backend и транспорт Vokop")
		}
		vk := mobile.VKProfile{Routing: routing, Backend: backend, Protocol: protocol, WireGuard: v.WireGuard, WrapKey: v.Turn.WrapKey, TURNTransport: "auto", ManualCaptcha: true, Mode: "vk", UUID: v.Vless.UUID, PeerHost: v.Turn.Host, PeerPort: v.Turn.Port, VKCallURL: v.Turn.Call, Streams: v.Turn.Streams, DNSServer: "1.1.1.1", ExpiresAt: d.Expires}
		if len(v.Turn.Options) > 0 && string(v.Turn.Options) != "null" {
			options := DefaultVKOptions()
			dec := json.NewDecoder(strings.NewReader(string(v.Turn.Options)))
			dec.DisallowUnknownFields()
			if dec.Decode(&options) != nil {
				return ImportResult{}, errors.New("Неверные параметры TURN панели")
			}
			// As on Android, allocation count is inherited from turn.streams.
			options.Connections = 0
			if err := options.Apply(&vk); err != nil {
				return ImportResult{}, err
			}
		}
		p := Profile{ExternalID: v.ID, Routing: d.Routing, Name: v.Name, Kind: "vk", Addresses: []string{"10.77.0.2/32"}, DNS: []string{"1.1.1.1"}, Routes: []string{"0.0.0.0/0"}, MTU: 1280}
		if backend == "wireguard" {
			doc, err := mobile.ImportProfile(v.WireGuard)
			if err != nil {
				return ImportResult{}, err
			}
			var wg mobile.ImportedProfile
			_ = json.Unmarshal([]byte(doc), &wg)
			p.Addresses, p.DNS, p.Routes, p.MTU = wg.Addresses, wg.DNS, wg.Routes, wg.MTU
		}
		p.Raw, _ = json.Marshal(vk)
		result.Profiles = append(result.Profiles, p)
	}
	for _, source := range d.VPNProfiles {
		if strings.Contains(source, "vokop.subscription.") {
			return ImportResult{}, errors.New("Вложенные подписки не поддерживаются")
		}
		extra, err := Parse(source, now, 0)
		if err != nil {
			return ImportResult{}, err
		}
		for i := range extra.Profiles {
			extra.Profiles[i].Routing = d.Routing
		}
		if len(result.Profiles)+len(extra.Profiles) > 1024 {
			return ImportResult{}, errors.New("Слишком много профилей в подписке")
		}
		result.Profiles = append(result.Profiles, extra.Profiles...)
		result.Rejected += extra.Rejected
	}
	return result, nil
}

func Fetch(ctx context.Context, rawURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return "", errors.New("Нужна полная HTTPS-ссылка подписки")
	}
	tr := &http.Transport{Proxy: nil, TLSHandshakeTimeout: 10 * time.Second, DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext}
	defer tr.CloseIdleConnections()
	c := &http.Client{Transport: tr, Timeout: 25 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 || !strings.EqualFold(r.URL.Host, u.Host) || r.URL.Scheme != "https" || r.URL.User != nil {
			return errors.New("Перенаправление за пределы HTTPS-источника запрещено")
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", errors.New("Некорректная ссылка")
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "Blink-Windows/0.3.0-preview.2")
	res, err := c.Do(req)
	if err != nil {
		return "", errors.New("Не удалось загрузить подписку по HTTPS")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", errors.New("Сервер подписки вернул ошибку HTTP")
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, MaxDocument+1))
	if err != nil || len(b) > MaxDocument {
		return "", errors.New("Подписка превышает 1 МиБ или не прочитана")
	}
	return string(bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf})), nil
}
