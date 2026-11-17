//go:build windows && (386 || arm)

// SPDX-License-Identifier: MIT
// Copyright (C) 2019-2026 WireGuard LLC. All Rights Reserved.
// FWPM_FILTER0 layout adapted from wireguard/windows v1.1.1.
package wfp

import "golang.org/x/sys/windows"

const (
	filterSize    = 152
	sessionSize   = 48
	sublayerSize  = 44
	conditionSize = 28
	valueSize     = 8
	weightOffset  = 72
	actionOffset  = 88
	contextOffset = 112
	idOffset      = 136
)

type filter0 struct {
	filterKey           windows.GUID // Windows type: GUID
	displayData         displayData0
	flags               uint32
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
	offset2             [4]byte       // Layout correction field
	filterID            uint64
	effectiveWeight     value0
}
