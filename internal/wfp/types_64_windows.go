//go:build windows && (amd64 || arm64)

// SPDX-License-Identifier: MIT
// Copyright (C) 2019-2026 WireGuard LLC. All Rights Reserved.
// FWPM_FILTER0 layout adapted from wireguard/windows v1.1.1.
package wfp

import "golang.org/x/sys/windows"

const (
	filterSize    = 200
	sessionSize   = 72
	sublayerSize  = 72
	conditionSize = 40
	valueSize     = 16
	weightOffset  = 96
	actionOffset  = 128
	contextOffset = 152
	idOffset      = 176
)

type filter0 struct {
	filterKey           windows.GUID // Windows type: GUID
	displayData         displayData0
	flags               uint32        // Windows type: UINT32
	providerKey         *windows.GUID // Windows type: *GUID
	providerData        byteBlob
	layerKey            windows.GUID // Windows type: GUID
	subLayerKey         windows.GUID // Windows type: GUID
	weight              value0
	numFilterConditions uint32
	filterCondition     *condition0
	action              action0
	offset1             [4]byte       // Layout correction field
	providerContextKey  windows.GUID  // Windows type: GUID
	reserved            *windows.GUID // Windows type: *GUID
	filterID            uint64
	effectiveWeight     value0
}
