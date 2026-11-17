// SPDX-License-Identifier: GPL-3.0-only
package platform

import (
	"strings"
	"testing"
)

func TestCleanupReportSelectsStructuredCodesOnly(t *testing.T) {
	raw := []byte("private-key https://secret.invalid/token\n" +
		`BLINK_CLEANUP {"step":"nrpt","ok":false,"hresult":-2147024891,"errorId":"AccessDenied,Get-DnsClientNrptRule","category":"PermissionDenied","exception":"System.UnauthorizedAccessException","message":"private-key"}` + "\n" +
		`BLINK_CLEANUP {"step":"firewall","ok":true,"count":0,"hresult":0}` + "\n" +
		`BLINK_CLEANUP {"step":"private-key","ok":false,"hresult":1}` + "\n" +
		`BLINK_CLEANUP {"step":"routes","ok":false,"hresult":5,"errorId":"https://secret.invalid/token"}`)
	e := cleanupEvents(raw)
	if len(e) != 3 || e[0].Category != "PermissionDenied" || e[2].ErrorID != "" {
		t.Fatalf("unexpected events: %+v", e)
	}
	message := cleanupFailure(raw).Error()
	if !strings.Contains(message, "0x80070005") || !strings.Contains(message, "NRPT") || strings.Contains(message, "private") || strings.Contains(message, "secret") {
		t.Fatal(message)
	}
}
