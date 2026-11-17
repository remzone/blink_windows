// SPDX-License-Identifier: GPL-3.0-only
package client

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestConnectionProgressExplainsDNSAndRedactsUnknownPayload(t *testing.T) {
	now := time.Now()
	trace := NewConnectionTrace(now)
	trace.SetStage("transport", now)
	trace.Consume(`[{"event":"native.vk.request","fields":{"stage":1}},{"event":"private-secret-url","fields":{"token":1}},{"event":"native.vk.retry","fields":{"stage":8,"url":2}}]`, now)
	if !strings.Contains(trace.TimeoutMessage(), "DNS") {
		t.Fatal(trace.TimeoutMessage())
	}
	report := trace.Report("test", "386")
	if strings.Contains(report, "vk") || strings.Contains(report, "private-secret") || strings.Contains(report, "token") || strings.Contains(report, "url") {
		t.Fatal("unexpected report fields")
	}
	var decoded any
	if json.Unmarshal([]byte(report), &decoded) != nil {
		t.Fatal("invalid report")
	}
}
func TestConnectionTraceIsBoundedAndExplainsWindowsStartup(t *testing.T) {
	now := time.Now()
	trace := NewConnectionTrace(now)
	trace.SetStage("adapter", now)
	if !strings.Contains(trace.TimeoutMessage(), "45 секунд") {
		t.Fatal(trace.TimeoutMessage())
	}
	for i := 0; i < 250; i++ {
		trace.Consume(`[{"event":"native.vk.retry","fields":{"stage":5}}]`, now)
	}
	if len(trace.Events) != 100 {
		t.Fatal(len(trace.Events))
	}
	trace.Fail("START_TIMEOUT", trace.TimeoutMessage(), now.Add(time.Minute))
	if trace.Stage != "failed" || trace.ElapsedSeconds != 60 {
		t.Fatal(trace)
	}
}

func TestXrayLogSeverityDoesNotMislabelStartupWarningAsFailure(t *testing.T) {
	now := time.Now()
	trace := NewConnectionTrace(now)
	trace.SetStage("network", now)
	trace.Consume(`[{"event":"native.xray.error","fields":{"stage":0,"errorCode":2,"count":1}},{"event":"native.xray.error","fields":{"stage":1,"errorCode":1,"count":1}},{"event":"native.xray.error","fields":{"stage":1,"errorCode":3,"count":1}}]`, now)
	if trace.Events[1].Code != "vpn.core_warning" || trace.Events[2].Code != "vpn.core_error" || trace.Events[3].Code != "vpn.core_info" || trace.Stage != "network" || trace.Error != "" {
		t.Fatalf("incorrect severity: %+v", trace)
	}
}
