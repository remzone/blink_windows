// SPDX-License-Identifier: GPL-3.0-only
package platform

import (
	"errors"
	"github.com/remzone/blink_windows/internal/client"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
	"os/exec"
	"strings"
	"testing"
)

func TestRejectMalformedNetworkBeforePowerShell(t *testing.T) {
	p := client.Profile{Addresses: []string{"10.77.0.2/32"}, Routes: []string{"0.0.0.0/0"}, DNS: []string{"1.1.1.1"}, MTU: 1280}
	if err := validateNetwork(p); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"1.1.1.1';exit;#", "dns.example.org", "::1"} {
		bad := p
		bad.DNS = []string{s}
		if validateNetwork(bad) == nil {
			t.Fatalf("accepted DNS %q", s)
		}
	}
	bad := p
	bad.Routes = []string{"0.0.0.0/0';exit;#"}
	if validateNetwork(bad) == nil {
		t.Fatal("unsafe route accepted")
	}
}

func TestDualStackWGRequiresExplicitAddressAndFullCoreRouting(t *testing.T) {
	p := client.Profile{Kind: "wireguard", Addresses: []string{"10.77.0.2/32", "fd00::2/128"}, Routes: []string{"0.0.0.0/0", "::/0"}, DNS: []string{"1.1.1.1", "2606:4700:4700::1111"}, MTU: 1280}
	if err := validateNetwork(p); err != nil {
		t.Fatal(err)
	}
	p.Routing = []byte(`{"mode":"selected"}`)
	if validateNetwork(p) == nil {
		t.Fatal("unsupported IPv6 selected routing accepted")
	}
	p.Routing = nil
	p.Kind = "xray"
	if validateNetwork(p) == nil {
		t.Fatal("unsupported Xray IPv6 accepted")
	}
}

// Read-only Windows regression: choosing the upstream must not parse PowerShell.
func TestDefaultInterfaceUsesWindowsRoutingTable(t *testing.T) {
	index, err := defaultInterface(windows.AF_INET)
	if err != nil {
		t.Fatal(err)
	}
	if index == 0 {
		t.Skip("No active IPv4 default route on this machine")
	}
	luid, err := winipcfg.LUIDFromIndex(index)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := luid.Interface()
	if err != nil {
		t.Fatal(err)
	}
	if strings.EqualFold(adapter.Alias(), "Blink") || adapter.OperStatus != winipcfg.IfOperStatusUp {
		t.Fatalf("invalid upstream: %s", adapter.Alias())
	}
}

func TestPowerShellStderrCannotCorruptStdout(t *testing.T) {
	out, err := powershell(`[Console]::Error.WriteLine('diagnostic');Write-Output 42`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "42" {
		t.Fatalf("unexpected stdout: %q", out)
	}
}

func TestNetworkFailureExportsOnlyKnownStepAndNumericHRESULT(t *testing.T) {
	failure := &exec.ExitError{Stderr: []byte("private-server.example/private-token BLINK_STEP=dns;BLINK_HRESULT=-2147024891 private-key")}
	message := networkConfigurationError(failure).Error()
	if !strings.Contains(message, "DNS") || !strings.Contains(message, "0x80070005") || strings.Contains(message, "private") {
		t.Fatal(message)
	}
	failure.Stderr = []byte("BLINK_STEP=private-token;BLINK_HRESULT=5")
	if strings.Contains(networkConfigurationError(failure).Error(), "private") {
		t.Fatal("unknown step leaked")
	}
}

// Real PowerShell parser with mocked cmdlets: these tests never change the host network.
func TestCleanupWithoutRulesOrAddressesIsSuccessful(t *testing.T) {
	mock := `
 function Get-DnsClientNrptRule {}
 function Clear-DnsClientCache {throw 'cache must not be touched without rules'}
 function Get-NetFirewallRule {[pscustomobject]@{Name='OtherVPN'}}
 function Get-NetAdapter {[pscustomobject]@{Name='Blink';ifIndex=1234};[pscustomobject]@{Name='Ethernet';ifIndex=9}}
 function Set-DnsClientServerAddress {param($InterfaceIndex,[switch]$ResetServerAddresses,$ErrorAction) if($InterfaceIndex -ne 1234){throw 'wrong adapter'}}
 function Get-NetIPAddress {}
 function Get-NetRoute {}
 function Remove-DnsClientNrptRule {throw 'nothing to delete'}
 function Remove-NetFirewallRule {throw 'wrong firewall rule'}
 function Remove-NetIPAddress {throw 'nothing to delete'}
 function Remove-NetRoute {throw 'nothing to delete'}
 `
	if _, err := powershell(mock + cleanupScript); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupKeepsFailingStepAndAttemptsOtherSteps(t *testing.T) {
	mock := `
 function Get-DnsClientNrptRule {throw [System.UnauthorizedAccessException]::new('sensitive error text')}
 function Get-NetFirewallRule {}
 function Get-NetAdapter {}
 `
	_, err := powershell(mock + cleanupScript)
	var failure *exec.ExitError
	if !errors.As(err, &failure) {
		t.Fatalf("expected failure: %v", err)
	}
	events := cleanupEvents(failure.Stderr)
	if len(events) != 4 || events[0].OK || events[0].Step != "nrpt" || !events[2].OK || !events[3].OK {
		t.Fatalf("%+v", events)
	}
	if strings.Contains(cleanupError(err).Error(), "sensitive") {
		t.Fatal("raw stderr escaped")
	}
}

// Production network script with inert mocks: Firewall creation must never call
// NetSecurity again, and failures on remaining steps retain structured metadata.
func TestNetworkScriptUsesNativeGuardAndRetainsDNSFailure(t *testing.T) {
	mock := `
 function Get-NetAdapter {[pscustomobject]@{ifIndex=1234}}
 function Get-NetRoute {}
 function Get-NetIPAddress {}
 function Disable-NetAdapterBinding {}
 function Set-NetIPInterface {}
 function New-NetIPAddress {}
 function Set-DnsClientServerAddress {if($script:blinkTestDeny){throw [System.UnauthorizedAccessException]::new('private error text')}}
 function New-NetRoute {}
 function Add-DnsClientNrptRule {}
 function Clear-DnsClientCache {}
 function New-NetFirewallRule {throw 'NetSecurity rule creation must not be used'}
 `
	p := client.Profile{Addresses: []string{"10.77.0.2/32"}, Routes: []string{"0.0.0.0/0"}, DNS: []string{"1.1.1.1"}, MTU: 1280}
	script := buildNetworkScript(p)
	if _, err := powershell(mock + script); err != nil {
		t.Fatal(err)
	}
	_, err := powershell(mock + `$script:blinkTestDeny=$true;` + script)
	var failure *exec.ExitError
	if !errors.As(err, &failure) {
		t.Fatalf("expected mocked DNS error: %v", err)
	}
	events := networkEvents(failure.Stderr)
	if len(events) != 1 || events[0].Step != "dns" || events[0].OK || events[0].HRESULT == 0 || events[0].Exception == "" {
		t.Fatalf("lost failure details: %+v", events)
	}
	if strings.Contains(networkConfigurationError(err).Error(), "private") {
		t.Fatal("raw error text escaped")
	}
}
