package ui

import (
	"math"
	"strconv"
	"strings"
)

// rgba packs a color the way Dear ImGui expects (IM_COL32).
func rgba(r, g, b, a uint8) uint32 {
	return uint32(a)<<24 | uint32(b)<<16 | uint32(g)<<8 | uint32(r)
}

func parseF(s string, def float64) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f <= 0 || math.IsInf(f, 0) {
		return def
	}
	return f
}

func num(v float64) string { return strconv.FormatFloat(v, 'g', 4, 64) }

// si formats a value with k/M/m/u prefixes: 16000 Hz -> "16 kHz".
func si(v float64, unit string) string {
	av := math.Abs(v)
	switch {
	case av >= 1e6:
		return num(v/1e6) + " M" + unit
	case av >= 1e3:
		return num(v/1e3) + " k" + unit
	case av >= 1 || av == 0:
		return num(v) + " " + unit
	case av >= 1e-3:
		return num(v*1e3) + " m" + unit
	default:
		return num(v*1e6) + " u" + unit
	}
}
