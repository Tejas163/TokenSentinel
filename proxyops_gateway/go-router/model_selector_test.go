package main

import (
	"testing"
)

func TestSelectModel_EmptyProviders(t *testing.T) {
	if got := selectModel([]byte("hello"), nil); got != nil {
		t.Error("expected nil for empty providers")
	}
}

func TestSelectModel_SingleProvider(t *testing.T) {
	providers := []UpstreamConfig{{URL: "http://a.com", Model: "gpt-4"}}
	got := selectModel([]byte("hello"), providers)
	if got == nil || got.URL != "http://a.com" {
		t.Error("expected single provider to be returned")
	}
}

func TestSelectModel_ShortPromptUsesCheap(t *testing.T) {
	providers := []UpstreamConfig{
		{URL: "http://cheap.com", Model: "gpt-3.5"},
		{URL: "http://capable.com", Model: "gpt-4"},
	}
	got := selectModel([]byte("short prompt"), providers)
	if got == nil || got.URL != "http://cheap.com" {
		t.Errorf("expected cheap model for short prompt, got %v", got)
	}
}

func TestSelectModel_MediumPromptUsesMedium(t *testing.T) {
	providers := []UpstreamConfig{
		{URL: "http://cheap.com", Model: "gpt-3.5"},
		{URL: "http://medium.com", Model: "gpt-4o-mini"},
		{URL: "http://capable.com", Model: "gpt-4"},
	}
	prompt := make([]byte, 1000)
	got := selectModel(prompt, providers)
	if got == nil || got.URL != "http://medium.com" {
		t.Errorf("expected medium model for medium prompt, got %v", got)
	}
}

func TestSelectModel_LongPromptUsesCapable(t *testing.T) {
	providers := []UpstreamConfig{
		{URL: "http://cheap.com", Model: "gpt-3.5"},
		{URL: "http://capable.com", Model: "gpt-4"},
	}
	prompt := make([]byte, 5000)
	got := selectModel(prompt, providers)
	if got == nil || got.URL != "http://capable.com" {
		t.Errorf("expected capable model for long prompt, got %v", got)
	}
}

func TestSelectModel_ExactTierPreferred(t *testing.T) {
	providers := []UpstreamConfig{
		{URL: "http://cheap1", Model: "gpt-3.5"},
		{URL: "http://cheap2", Model: "claude-3-haiku"},
	}
	got := selectModel([]byte("hi"), providers)
	if got == nil || got.URL != "http://cheap1" {
		t.Errorf("expected first matching cheap model, got %v", got)
	}
}

func TestSelectModel_FallsBackToClosestTier(t *testing.T) {
	providers := []UpstreamConfig{
		{URL: "http://cheap.com", Model: "gpt-3.5"},
	}
	prompt := make([]byte, 5000)
	got := selectModel(prompt, providers)
	if got == nil || got.URL != "http://cheap.com" {
		t.Errorf("expected fallback to only available provider, got %v", got)
	}
}

func TestModelTierFor_KnownModels(t *testing.T) {
	tests := []struct {
		model string
		want  modelTier
	}{
		{"gpt-3.5-turbo", tierCheap},
		{"gpt-4o-mini", tierMedium},
		{"gpt-4", tierCapable},
		{"claude-3-haiku", tierCheap},
		{"claude-3-opus", tierCapable},
		{"mistral-small", tierCheap},
		{"mistral-large", tierCapable},
		{"gemini-1.5-flash", tierCheap},
		{"gemini-2.0-pro", tierCapable},
	}
	for _, tc := range tests {
		got := modelTierFor(tc.model)
		if got != tc.want {
			t.Errorf("modelTierFor(%q) = %d, want %d", tc.model, got, tc.want)
		}
	}
}

func TestModelTierFor_Unknown(t *testing.T) {
	if got := modelTierFor("unknown-model-v1"); got != tierUnknown {
		t.Errorf("expected tierUnknown, got %d", got)
	}
}

func TestCloserTier_ExactMatch(t *testing.T) {
	if got := closerTier(tierCheap, tierCheap); got != 2 {
		t.Errorf("closerTier(cheap, cheap) = %d, want 2", got)
	}
}

func TestCloserTier_Adjacent(t *testing.T) {
	if got := closerTier(tierCheap, tierMedium); got != 1 {
		t.Errorf("closerTier(cheap, medium) = %d, want 1", got)
	}
}

func TestCloserTier_Unknown(t *testing.T) {
	if got := closerTier(tierUnknown, tierCapable); got != 1 {
		t.Errorf("closerTier(unknown, capable) = %d, want 1", got)
	}
}

func TestCheapestScore_KnownModel(t *testing.T) {
	if got := cheapestScore("gpt-3.5-turbo"); got != 5 {
		t.Errorf("cheapestScore(gpt-3.5-turbo) = %d, want 5", got)
	}
}

func TestCheapestScore_UnknownModel(t *testing.T) {
	if got := cheapestScore("o1-preview"); got != 0 {
		t.Errorf("cheapestScore(o1-preview) = %d, want 0", got)
	}
}

func TestCheapestProvider_Empty(t *testing.T) {
	if got := cheapestProvider(nil); got != nil {
		t.Error("expected nil for empty providers")
	}
}

func TestCheapestProvider_SelectsCheapest(t *testing.T) {
	providers := []UpstreamConfig{
		{URL: "http://capable.com", Model: "gpt-4"},
		{URL: "http://cheap.com", Model: "gpt-3.5"},
	}
	got := cheapestProvider(providers)
	if got == nil || got.URL != "http://cheap.com" {
		t.Errorf("expected cheapest provider, got %v", got)
	}
}
