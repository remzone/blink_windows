// SPDX-License-Identifier: GPL-3.0-only
package platform

import "strings"

type routeCandidate struct {
	Index                        uint32
	Alias                        string
	Up                           bool
	RouteMetric, InterfaceMetric uint32
}

func chooseDefaultInterface(routes []routeCandidate) uint32 {
	var index uint32
	var metric uint64
	for _, route := range routes {
		if route.Index == 0 || !route.Up || strings.EqualFold(route.Alias, "Blink") {
			continue
		}
		// Windows uses route + interface metric; widen before adding to avoid overflow.
		total := uint64(route.RouteMetric) + uint64(route.InterfaceMetric)
		if index == 0 || total < metric || total == metric && route.Index < index {
			index, metric = route.Index, total
		}
	}
	return index
}
