package main

import (
	"testing"
)

func TestEnforceBudget_NoTeam(t *testing.T) {
	target := &UpstreamConfig{URL: "http://target.com", Model: "gpt-4"}
	got := enforceBudget(nil, "", nil, target)
	if got != target {
		t.Error("expected original target when no team")
	}
}

func TestCheapestScore_Ordering(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"gpt-3.5-turbo", 5},
		{"claude-3-haiku", 4},
		{"llama-3-8b", 3},
		{"mistral-small-latest", 2},
		{"gemini-1.5-flash-001", 1},
	}
	for _, tc := range tests {
		got := cheapestScore(tc.model)
		if got != tc.want {
			t.Errorf("cheapestScore(%q) = %d, want %d", tc.model, got, tc.want)
		}
	}
}

func TestCheapestScore_SubstringMatch(t *testing.T) {
	if got := cheapestScore("gpt-3.5"); got != 5 {
		t.Errorf("cheapestScore(gpt-3.5) = %d, want 5 (substring match)", got)
	}
}

func TestCheapestProvider_SelectsByScore(t *testing.T) {
	providers := []UpstreamConfig{
		{URL: "http://capable1", Model: "gpt-4"},
		{URL: "http://capable2", Model: "claude-3-opus"},
		{URL: "http://cheap", Model: "gpt-3.5-turbo"},
	}
	got := cheapestProvider(providers)
	if got == nil || got.URL != "http://cheap" {
		t.Errorf("cheapestProvider = %v, want http://cheap", got)
	}
}

func TestCheapestProvider_TieGoesToFirst(t *testing.T) {
	providers := []UpstreamConfig{
		{URL: "http://first", Model: "gpt-3.5"},
		{URL: "http://second", Model: "claude-3-haiku"},
	}
	got := cheapestProvider(providers)
	if got == nil || got.URL != "http://first" {
		t.Errorf("cheapestProvider tie = %v, want http://first", got)
	}
}

func TestCheapestProvider_SingleProvider(t *testing.T) {
	providers := []UpstreamConfig{{URL: "http://only", Model: "o1-preview"}}
	got := cheapestProvider(providers)
	if got == nil || got.URL != "http://only" {
		t.Errorf("cheapestProvider single = %v, want http://only", got)
	}
}
