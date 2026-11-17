// SPDX-License-Identifier: GPL-3.0-only
package client

import (
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"github.com/remzone/blink_windows/native/mobile"
	"github.com/remzone/blink_windows/native/vkturn"
)

// VKOptions matches the Android/iOS connection settings. Credentials are never settings.
type VKOptions struct {
	ServerDefaults bool   `json:"serverDefaults"`
	Transport      string `json:"transport"`
	Preset         string `json:"preset"`
	DNS            string `json:"dns"`
	Relay          string `json:"relay"`
	Connections    int    `json:"connections"`
	MTU            int    `json:"mtu"`
	Pace           int    `json:"pace"`
	Window         int    `json:"window"`
	Interval       int    `json:"interval"`
	Congestion     bool   `json:"congestion"`
}

func DefaultVKOptions() VKOptions {
	return VKOptions{ServerDefaults: true, Transport: "auto", Preset: "compatibility", DNS: "1.1.1.1", Connections: 1, MTU: 1200, Window: 64, Interval: 20, Congestion: true}
}
func (o VKOptions) Validate() error {
	if o.Transport != "auto" && o.Transport != "tcp" && o.Transport != "udp" {
		return errors.New("Выберите Авто, TCP или UDP")
	}
	if o.Preset != "compatibility" && o.Preset != "balanced" && o.Preset != "conservative" && o.Preset != "custom" {
		return errors.New("Неизвестный профиль KCP")
	}
	ip, err := netip.ParseAddr(o.DNS)
	if err != nil || !ip.Is4() || o.Connections < 0 || o.Connections > 16 || o.MTU < 576 || o.MTU > 1200 || o.Pace != 0 && (o.Pace < 16 || o.Pace > 4096) || o.Window < 16 || o.Window > 256 || o.Interval < 10 || o.Interval > 100 {
		return errors.New("Проверьте DNS, соединения, MTU, окно, интервал и pacing TURN")
	}
	if o.Relay != "" {
		host, port, e := net.SplitHostPort(o.Relay)
		n, _ := strconv.Atoi(port)
		if e != nil || host == "" || len(host) > 253 || strings.ContainsAny(host, " /'\"\r\n") || n < 1 || n > 65535 {
			return errors.New("TURN-сервер должен иметь формат host:port")
		}
	}
	return nil
}
func (o VKOptions) Apply(v *mobile.VKProfile) error {
	if err := o.Validate(); err != nil {
		return err
	}
	v.TURNTransport = o.Transport
	v.DNSServer = o.DNS
	v.TURNServer = o.Relay
	if o.Connections != 0 {
		v.Streams = o.Connections
	}
	v.Tuning = vkturn.Tuning{Preset: o.Preset, MTU: o.MTU, PaceKiB: o.Pace}
	if o.Preset == "custom" {
		v.Tuning.Window = o.Window
		v.Tuning.IntervalMS = o.Interval
		v.Tuning.Congestion = o.Congestion
	}
	return nil
}

type RoutingOptions struct {
	Mode    string `json:"mode"`
	Include string `json:"include"`
	Exclude string `json:"exclude"`
}

func DefaultRouting() RoutingOptions { return RoutingOptions{Mode: "inherit"} }
func (o RoutingOptions) Policy(server json.RawMessage) (string, error) {
	b, _ := json.Marshal(o)
	raw, err := mobile.MergeRoutingPolicy(string(server), string(b))
	if err != nil {
		return "", err
	}
	var policy mobile.RoutingPolicy
	if err = json.Unmarshal([]byte(raw), &policy); err != nil {
		return "", err
	}
	// The shared selected router puts exclusions before inclusions. Two IPv4 halves
	// express "all except" without its intentionally rejected /0 rule.
	if policy.Mode == "all" && (len(policy.ExcludeDomains) > 0 || len(policy.ExcludeIPs) > 0) {
		policy.Mode = "selected"
		policy.Domains = nil
		policy.IPs = []string{"0.0.0.0/1", "128.0.0.0/1"}
		if err = policy.Validate(); err != nil {
			return "", err
		}
		encoded, _ := json.Marshal(policy)
		return string(encoded), nil
	}
	return raw, nil
}

// ConnectionProfile revalidates saved source and applies the user's explicit overrides.
func ConnectionProfile(p Profile, vk VKOptions, routing RoutingOptions) (Profile, error) {
	policy, err := routing.Policy(p.Routing)
	if err != nil {
		return p, err
	}
	var decoded mobile.RoutingPolicy
	if err = json.Unmarshal([]byte(policy), &decoded); err != nil {
		return p, err
	}
	p.Routing = json.RawMessage(policy)
	if p.Kind == "vk" {
		var v mobile.VKProfile
		if json.Unmarshal(p.Raw, &v) != nil {
			return p, errors.New("Неверный сохранённый профиль")
		}
		v.Routing = &decoded
		if !vk.ServerDefaults {
			if err = vk.Apply(&v); err != nil {
				return p, err
			}
		}
		p.Raw, _ = json.Marshal(v)
	} else if decoded.Mode != "all" && p.Kind != "xray" && p.Kind != "xray-json" && p.Kind != "wireguard" && p.Kind != "amneziawg" {
		return p, errors.New("Выборочная маршрутизация требует Xray или WireGuard")
	}
	return p, nil
}
