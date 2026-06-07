package main

import (
	"testing"
)

func TestFindEquivalents_KnownModel(t *testing.T) {
	providers := []UpstreamConfig{
		{URL: "http://gpt4", Model: "gpt-4"},
		{URL: "http://sonnet", Model: "claude-3-sonnet"},
		{URL: "http://turbo", Model: "gpt-3.5-turbo"},
		{URL: "http://mini", Model: "gpt-4o-mini"},
	}
	equiv := findEquivalents("gpt-4", providers)
	if len(equiv) == 0 {
		t.Fatal("expected equivalents for gpt-4")
	}
	found := false
	for _, p := range equiv {
		if p.Model == "gpt-4o-mini" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected gpt-4o-mini in equivalents, got %v", modelsOf(equiv))
	}
}

func TestFindEquivalents_UnknownModel(t *testing.T) {
	providers := []UpstreamConfig{{URL: "http://x", Model: "gpt-4"}}
	equiv := findEquivalents("nonexistent-model", providers)
	if equiv != nil {
		t.Errorf("expected nil for unknown model, got %v", equiv)
	}
}

func TestFindEquivalents_EmptyProviders(t *testing.T) {
	equiv := findEquivalents("gpt-4", nil)
	if equiv != nil {
		t.Errorf("expected nil for nil providers")
	}
}

func TestFindEquivalents_NoMatchInProviders(t *testing.T) {
	providers := []UpstreamConfig{
		{URL: "http://custom", Model: "custom-model"},
	}
	equiv := findEquivalents("gpt-4", providers)
	if len(equiv) != 0 {
		t.Errorf("expected no equivalents when none available, got %d", len(equiv))
	}
}

func TestBuildFallbackChain_StartsWithTarget(t *testing.T) {
	providers := []UpstreamConfig{
		{URL: "http://primary", Model: "gpt-4"},
		{URL: "http://fallback", Model: "claude-3-sonnet"},
	}
	target := &providers[0]
	chain := buildFallbackChain(target, providers)
	if len(chain) == 0 || chain[0].URL != "http://primary" {
		t.Errorf("expected chain[0] to be primary, got %v", chain)
	}
}

func TestBuildFallbackChain_Deduplicates(t *testing.T) {
	providers := []UpstreamConfig{
		{URL: "http://primary", Model: "gpt-4"},
		{URL: "http://fallback", Model: "claude-3-sonnet"},
		{URL: "http://same-as-primary", Model: "gpt-4"},
	}
	target := &providers[0]
	chain := buildFallbackChain(target, providers)
	seen := map[string]int{}
	for _, p := range chain {
		seen[p.URL]++
	}
	for url, count := range seen {
		if count > 1 {
			t.Errorf("duplicate URL in chain: %s appears %d times", url, count)
		}
	}
}

func TestBuildFallbackChain_IncludesEquivalentsFirst(t *testing.T) {
	providers := []UpstreamConfig{
		{URL: "http://gpt4", Model: "gpt-4"},
		{URL: "http://cheap", Model: "llama-3-8b"},
		{URL: "http://mini", Model: "gpt-4o-mini"},
	}
	target := &providers[0]
	chain := buildFallbackChain(target, providers)
	if len(chain) < 3 {
		t.Fatalf("expected at least 3 in chain, got %d", len(chain))
	}
	miniIdx := -1
	for i, p := range chain {
		if p.URL == "http://mini" {
			miniIdx = i
		}
	}
	cheapIdx := -1
	for i, p := range chain {
		if p.URL == "http://cheap" {
			cheapIdx = i
		}
	}
	if miniIdx < 0 {
		t.Error("expected gpt-4o-mini in chain")
	}
	if cheapIdx < 0 {
		t.Error("expected llama-3-8b in chain")
	}
}

func TestCheapestProvider_Single(t *testing.T) {
	providers := []UpstreamConfig{{URL: "http://a", Model: "gpt-4"}}
	p := cheapestProvider(providers)
	if p == nil || p.URL != "http://a" {
		t.Error("expected single provider to be cheapest")
	}
}

func TestCheapestScore(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"gpt-3.5-turbo", 5},
		{"claude-3-haiku", 4},
		{"llama-3-8b", 3},
		{"mistral-small", 2},
		{"gemini-1.5-flash", 1},
		{"unknown-model", 0},
	}
	for _, tt := range tests {
		got := cheapestScore(tt.model)
		if got != tt.want {
			t.Errorf("cheapestScore(%q) = %d, want %d", tt.model, got, tt.want)
		}
	}
}

func TestCheapestProvider_Orders(t *testing.T) {
	providers := []UpstreamConfig{
		{URL: "http://opus", Model: "claude-3-opus"},
		{URL: "http://haiku", Model: "claude-3-haiku"},
		{URL: "http://turbo", Model: "gpt-3.5-turbo"},
	}
	p := cheapestProvider(providers)
	if p == nil || p.URL != "http://turbo" {
		t.Errorf("expected turbo (cheapest), got %v", p)
	}
}

func TestRecordProviderSuccess_NoPanic(t *testing.T) {
	providerHealthData = map[string]*providerHealthEntry{}
	recordProviderSuccess("http://test")
	recordProviderSuccess("http://test")
	h, ok := providerHealthData["http://test"]
	if !ok || h.successes != 2 {
		t.Errorf("expected 2 successes, got %d", h.successes)
	}
}

func TestProviderErrorRate(t *testing.T) {
	providerHealthData = map[string]*providerHealthEntry{}
	recordProviderSuccess("http://healthy")
	recordProviderFailure("http::/flaky")
	recordProviderFailure("http::/flaky")
	rate := providerErrorRate("http::/flaky")
	if rate != 1.0 {
		t.Errorf("expected 1.0 error rate, got %.2f", rate)
	}
	rate = providerErrorRate("http://unknown")
	if rate != 0 {
		t.Errorf("expected 0 for unknown, got %.2f", rate)
	}
}

func modelsOf(providers []*UpstreamConfig) []string {
	var models []string
	for _, p := range providers {
		models = append(models, p.Model)
	}
	return models
}
