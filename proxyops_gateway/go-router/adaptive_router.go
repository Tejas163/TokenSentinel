package main

import (
	"fmt"
	"log/slog"
	"math"
	"strings"
)

func adaptiveWeight(p UpstreamConfig) int {
	base := p.Weight
	if base <= 0 {
		base = 1
	}

	errRate := providerErrorRate(p.URL)

	decay := 1.0 - errRate
	if decay < 0.05 {
		decay = 0.05
	}

	adjusted := int(math.Round(float64(base) * decay))
	if adjusted < 1 {
		adjusted = 1
	}

	return adjusted
}

func logAdaptiveWeights(providers []UpstreamConfig) {
	if len(providers) < 2 {
		return
	}
	var parts []string
	for _, p := range providers {
		staticW := p.Weight
		if staticW <= 0 {
			staticW = 1
		}
		dynamicW := adaptiveWeight(p)
		errRate := providerErrorRate(p.URL)
		if dynamicW != staticW {
			parts = append(parts, fmt.Sprintf("%s@%s: %d→%d (err %.1f%%)", p.Model, p.URL, staticW, dynamicW, errRate*100))
		}
	}
	if len(parts) > 0 {
		slog.Debug("adaptive weights", "changes", strings.Join(parts, "; "))
	}
}
