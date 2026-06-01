package main

import (
	"strings"
)

type modelTier int

const (
	tierUnknown modelTier = iota
	tierCheap
	tierMedium
	tierCapable
)

var modelTiers = []struct {
	prefix string
	tier   modelTier
}{
	{"gpt-3.5", tierCheap},
	{"gpt-4o-mini", tierMedium},
	{"gpt-4", tierCapable},
	{"claude-3-haiku", tierCheap},
	{"claude-3-sonnet", tierMedium},
	{"claude-3-opus", tierCapable},
	{"claude-3.5-sonnet", tierCapable},
	{"llama-3-8b", tierCheap},
	{"llama-3-70b", tierCapable},
	{"llama-3.1-8b", tierCheap},
	{"llama-3.1-70b", tierCapable},
	{"llama-3.1-405b", tierCapable},
	{"mixtral-8x7b", tierMedium},
	{"mistral-small", tierCheap},
	{"mistral-medium", tierMedium},
	{"mistral-large", tierCapable},
	{"gemini-1.5-flash", tierCheap},
	{"gemini-1.5-pro", tierCapable},
	{"gemini-2.0-flash", tierMedium},
	{"gemini-2.0-pro", tierCapable},
}

func modelTierFor(name string) modelTier {
	lower := strings.ToLower(name)
	for _, m := range modelTiers {
		if strings.HasPrefix(lower, m.prefix) {
			return m.tier
		}
	}
	return tierUnknown
}

func selectModel(prompt []byte, providers []UpstreamConfig) *UpstreamConfig {
	if len(providers) == 0 {
		return nil
	}
	if len(providers) == 1 {
		return &providers[0]
	}

	promptLen := len(prompt)
	var neededTier modelTier
	switch {
	case promptLen < 500:
		neededTier = tierCheap
	case promptLen < 2000:
		neededTier = tierMedium
	default:
		neededTier = tierCapable
	}

	var best *UpstreamConfig
	for i := range providers {
		tier := modelTierFor(providers[i].Model)
		if tier == neededTier {
			return &providers[i]
		}
		if best == nil || closerTier(tier, neededTier) > closerTier(modelTierFor(best.Model), neededTier) {
			best = &providers[i]
		}
	}
	return best
}

func closerTier(a, b modelTier) int {
	if a == b {
		return 2
	}
	if a == tierUnknown || b == tierUnknown {
		return 1
	}
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return 2 - int(diff)
}
