// SPDX-License-Identifier: GPL-3.0-only
// Package wfp installs a temporary IPv6 egress guard in a dynamic WFP session.
package wfp

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/remzone/blink_windows/internal/diagnostics"
	"golang.org/x/sys/windows"
)

var (
	dll            = windows.NewLazySystemDLL("fwpuclnt.dll")
	engineOpen     = dll.NewProc("FwpmEngineOpen0")
	engineClose    = dll.NewProc("FwpmEngineClose0")
	txnBegin       = dll.NewProc("FwpmTransactionBegin0")
	txnCommit      = dll.NewProc("FwpmTransactionCommit0")
	txnAbort       = dll.NewProc("FwpmTransactionAbort0")
	sublayerAdd    = dll.NewProc("FwpmSubLayerAdd0")
	filterAdd      = dll.NewProc("FwpmFilterAdd0")
	appIDFromName  = dll.NewProc("FwpmGetAppIdFromFileName0")
	freeMemory     = dll.NewProc("FwpmFreeMemory0")
	layerConnectV6 = windows.GUID{Data1: 0x4a72393b, Data2: 0x319f, Data3: 0x44bc, Data4: [8]byte{0x84, 0xc3, 0xba, 0x54, 0xdc, 0xb3, 0xb6, 0xb4}}
	fieldAppID     = windows.GUID{Data1: 0xd78e1e87, Data2: 0x8644, Data3: 0x4ea5, Data4: [8]byte{0x94, 0x37, 0xd8, 0x09, 0xec, 0xef, 0xc9, 0x71}}
	fieldFlags     = windows.GUID{Data1: 0x632ce23b, Data2: 0x5167, Data3: 0x435c, Data4: [8]byte{0x86, 0xd7, 0xe9, 0x03, 0x68, 0x4a, 0xa8, 0x0c}}
	fieldProtocol  = windows.GUID{
		Data1: 0x3971ef2b,
		Data2: 0x623e,
		Data3: 0x4f9a,
		Data4: [8]byte{0x8c, 0xb1, 0x6e, 0x79, 0xb8, 0x06, 0xb9, 0xa7},
	}
	fieldLocalPort = windows.GUID{
		Data1: 0x0c1ba1af,
		Data2: 0x5765,
		Data3: 0x453f,
		Data4: [8]byte{0xaf, 0x22, 0xa8, 0xf7, 0x91, 0xac, 0x77, 0x5b},
	}
	fieldRemotePort = windows.GUID{
		Data1: 0xc35a604d,
		Data2: 0xd22b,
		Data3: 0x4e1a,
		Data4: [8]byte{0x91, 0xb4, 0x68, 0xf6, 0x74, 0xee, 0x67, 0x4b},
	}
)

// Win32 APIs return their DWORD status directly, not GetLastError.
// Pointer arguments must escape before Find/Call can allocate or grow the stack.
//
//go:uintptrescapes
func call(proc *windows.LazyProc, args ...uintptr) (err error) {
	started := time.Now()
	diagnostics.Event("windows.filter.api.start", map[string]any{"operation": proc.Name})
	defer func() {
		var code syscall.Errno
		errors.As(err, &code)
		diagnostics.Event("windows.filter.api.finish", map[string]any{"operation": proc.Name, "elapsedMs": time.Since(started).Milliseconds(), "ok": err == nil, "status": uint32(code)})
	}()
	if err := proc.Find(); err != nil {
		return err
	}
	result, _, _ := proc.Call(args...)
	if result != 0 {
		return syscall.Errno(result)
	}
	return nil
}

func failure(step string, err error) error {
	var code syscall.Errno
	errors.As(err, &code)
	diagnostics.Event("windows.filter.failure", map[string]any{"backend": "wfp", "step": step, "win32": uint32(code)})
	return fmt.Errorf("Windows не включила временную защиту IPv6 (WFP: %s, код 0x%08X). Подробности — в файле журнала", step, uint32(code))
}

type Session struct {
	mu     sync.Mutex
	handle uintptr
}

// New creates an all-or-nothing, process-owned policy. No persistent Firewall
// rules, CIDRs, keywords, service settings or physical adapter bindings are used.
func New() (*Session, error) {
	diagnostics.Event("windows.filter.start", map[string]any{"backend": "wfp"})
	name, _ := windows.UTF16PtrFromString("Blink temporary IPv6 protection")
	config := session0{displayData: displayData0{name: name}, flags: 1, txnWaitTimeoutInMSec: 5000}

	s := &Session{}
	err := installDynamic(operations{
		open: func() error {
			err := call(engineOpen, 0, 10, 0, uintptr(unsafe.Pointer(&config)), uintptr(unsafe.Pointer(&s.handle)))
			runtime.KeepAlive(config)
			runtime.KeepAlive(name)
			return err
		},
		begin:   func() error { return call(txnBegin, s.handle, 0) },
		install: s.configure,
		commit:  func() error { return call(txnCommit, s.handle) },
		abort: func() error {
			err := call(txnAbort, s.handle)
			if err != nil {
				return failure("abort", err)
			}
			return nil
		},
		close: s.Close,
	})
	if err != nil {
		var op *operationError
		if errors.As(err, &op) {
			return nil, failure(op.step, op.err)
		}
		return nil, err
	}
	diagnostics.Event("windows.filter.ready", map[string]any{"backend": "wfp", "filters": 7, "dynamic": true})
	return s, nil
}

func (s *Session) configure() error {
	handle := s.handle
	name, _ := windows.UTF16PtrFromString("Blink temporary IPv6 protection")
	fail := func(step string, err error) error { return &operationError{step, err} }

	key, err := windows.GenerateGUID()
	if err != nil {
		return fail("guid", err)
	}
	layer := sublayer0{subLayerKey: key, displayData: displayData0{name: name}, weight: 0xffff}
	if err = call(sublayerAdd, handle, uintptr(unsafe.Pointer(&layer)), 0); err != nil {
		return fail("sublayer", err)
	}
	runtime.KeepAlive(layer)
	runtime.KeepAlive(name)
	exe, err := os.Executable()
	if err != nil {
		return fail("executable", err)
	}
	path, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return fail("executable", err)
	}
	var appID *byteBlob
	if err = call(appIDFromName, uintptr(unsafe.Pointer(path)), uintptr(unsafe.Pointer(&appID))); err != nil {
		return fail("application", err)
	}
	runtime.KeepAlive(path)
	defer func() { freeMemory.Call(uintptr(unsafe.Pointer(&appID))) }()
	// Blink's outer VPN/TURN transport may use IPv6 even for an IPv4 tunnel.
	// The higher-weight permit applies only in our own sublayer, so it cannot
	// override blocks imposed by another firewall provider.
	app := condition0{fieldKey: fieldAppID, conditionValue: value0{_type: 12, value: uintptr(unsafe.Pointer(appID))}}
	loopback := condition0{fieldKey: fieldFlags, matchType: 6, conditionValue: value0{_type: 3, value: 1}}
	if err = s.add(key, "Blink IPv6 transport", 15, 0x1002, []condition0{app}); err != nil {
		return fail("permit_transport", err)
	}
	runtime.KeepAlive(appID)
	if err = s.add(key, "Blink IPv6 loopback", 14, 0x1002, []condition0{loopback}); err != nil {
		return fail("permit_loopback", err)
	}
	// Neighbour/router discovery and DHCPv6 remain available to the OS.
	for _, icmpType := range []uintptr{133, 135, 136} {
		c := []condition0{
			{fieldKey: fieldProtocol, conditionValue: value0{_type: 1, value: 58}},
			{fieldKey: fieldLocalPort, conditionValue: value0{_type: 2, value: icmpType}},
			{fieldKey: fieldRemotePort, conditionValue: value0{_type: 2, value: 0}},
		}
		if err = s.add(key, "Blink IPv6 neighbour discovery", 13, 0x1002, c); err != nil {
			return fail("permit_ndp", err)
		}
	}
	dhcp := []condition0{
		{fieldKey: fieldProtocol, conditionValue: value0{_type: 1, value: 17}},
		{fieldKey: fieldLocalPort, conditionValue: value0{_type: 2, value: 546}},
		{fieldKey: fieldRemotePort, conditionValue: value0{_type: 2, value: 547}},
	}
	if err = s.add(key, "Blink IPv6 DHCP", 13, 0x1002, dhcp); err != nil {
		return fail("permit_dhcp", err)
	}
	if err = s.add(key, "Blink block IPv6 egress", 0, 0x1001, nil); err != nil {
		return fail("block", err)
	}
	return nil
}

func (s *Session) add(sublayer windows.GUID, name string, weight uintptr, action uint32, conditions []condition0) error {
	label, _ := windows.UTF16PtrFromString(name)
	f := filter0{displayData: displayData0{name: label}, layerKey: layerConnectV6, subLayerKey: sublayer, weight: value0{_type: 1, value: weight}, action: action0{_type: action}, numFilterConditions: uint32(len(conditions))}
	if len(conditions) > 0 {
		f.filterCondition = &conditions[0]
	}
	var id uint64
	err := call(filterAdd, s.handle, uintptr(unsafe.Pointer(&f)), 0, uintptr(unsafe.Pointer(&id)))
	runtime.KeepAlive(f)
	runtime.KeepAlive(conditions)
	runtime.KeepAlive(label)
	return err
}

func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle == 0 {
		return nil
	}
	if err := call(engineClose, s.handle); err != nil {
		return failure("close", err)
	}
	s.handle = 0
	diagnostics.Event("windows.filter.closed", map[string]any{"backend": "wfp"})
	return nil
}
