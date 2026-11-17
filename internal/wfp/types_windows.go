// SPDX-License-Identifier: MIT
// Copyright (C) 2019-2026 WireGuard LLC. All Rights Reserved.
// Windows SDK layouts adapted from wireguard/windows v1.1.1 (see LICENSE).
package wfp

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

type displayData0 struct{ name, description *uint16 }
type byteBlob struct {
	size uint32
	data *byte
}
type value0 struct {
	_type uint32
	value uintptr
}
type action0 struct {
	_type      uint32
	filterType windows.GUID
}
type condition0 struct {
	fieldKey       windows.GUID
	matchType      uint32
	conditionValue value0
}
type session0 struct {
	sessionKey                             windows.GUID
	displayData                            displayData0
	flags, txnWaitTimeoutInMSec, processID uint32
	sid                                    *windows.SID
	username                               *uint16
	kernelMode                             uint32
}
type sublayer0 struct {
	subLayerKey  windows.GUID
	displayData  displayData0
	flags        uint32
	providerKey  *windows.GUID
	providerData byteBlob
	weight       uint16
}

// Compile-time ABI checks run during cross compilation too, including x86's
// explicit padding around the C union and the 64-bit filter ID.
var (
	_ [filterSize - unsafe.Sizeof(filter0{})]byte
	_ [unsafe.Sizeof(filter0{}) - filterSize]byte
	_ [sessionSize - unsafe.Sizeof(session0{})]byte
	_ [unsafe.Sizeof(session0{}) - sessionSize]byte
	_ [sublayerSize - unsafe.Sizeof(sublayer0{})]byte
	_ [unsafe.Sizeof(sublayer0{}) - sublayerSize]byte
	_ [conditionSize - unsafe.Sizeof(condition0{})]byte
	_ [unsafe.Sizeof(condition0{}) - conditionSize]byte
	_ [valueSize - unsafe.Sizeof(value0{})]byte
	_ [unsafe.Sizeof(value0{}) - valueSize]byte
	_ [weightOffset - unsafe.Offsetof(filter0{}.weight)]byte
	_ [unsafe.Offsetof(filter0{}.weight) - weightOffset]byte
	_ [actionOffset - unsafe.Offsetof(filter0{}.action)]byte
	_ [unsafe.Offsetof(filter0{}.action) - actionOffset]byte
	_ [contextOffset - unsafe.Offsetof(filter0{}.providerContextKey)]byte
	_ [unsafe.Offsetof(filter0{}.providerContextKey) - contextOffset]byte
	_ [idOffset - unsafe.Offsetof(filter0{}.filterID)]byte
	_ [unsafe.Offsetof(filter0{}.filterID) - idOffset]byte
)
