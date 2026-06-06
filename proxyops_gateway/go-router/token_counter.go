package main

import (
	"strings"

	"github.com/pkoukk/tiktoken-go"
)

var knownPrefixes = []struct {
	prefix   string
	encoding string
}{
	{"gpt-4", "cl100k_base"},
	{"gpt-3.5", "cl100k_base"},
	{"gpt-3", "r50k_base"},
	{"text-davinci", "p50k_base"},
	{"text-curie", "r50k_base"},
	{"text-babbage", "r50k_base"},
	{"text-ada", "r50k_base"},
	{"code-davinci", "p50k_base"},
	{"code-cushman", "p50k_base"},
	{"j1-", "gpt2"},
	{"j2-", "gpt2"},
}

func estimateTokens(text, model string) int {
	tke, err := tiktoken.EncodingForModel(model)
	if err == nil {
		return len(tke.Encode(text, nil, nil))
	}
	modelLower := strings.ToLower(model)
	for _, p := range knownPrefixes {
		if strings.HasPrefix(modelLower, p.prefix) {
			tke, err := tiktoken.GetEncoding(p.encoding)
			if err == nil {
				return len(tke.Encode(text, nil, nil))
			}
		}
	}
	if approx, ok := modelFamilyApprox(modelLower); ok {
		return approx(text)
	}
	return len(text) / 4
}

var modelApproximators = []struct {
	prefix   string
	estimate func(text string) int
}{
	{"claude-3-5", func(t string) int { return len([]rune(t)) * 4 / 13 }},
	{"claude-3", func(t string) int { return len([]rune(t)) * 4 / 13 }},
	{"claude-4", func(t string) int { return len([]rune(t)) * 4 / 13 }},
	{"claude-2", func(t string) int { return len([]rune(t)) * 4 / 13 }},
	{"claude-", func(t string) int { return len([]rune(t)) * 4 / 13 }},
	{"gemini-2", func(t string) int { return len([]rune(t)) / 4 }},
	{"gemini-1", func(t string) int { return len([]rune(t)) / 4 }},
	{"gemini-", func(t string) int { return len([]rune(t)) / 4 }},
}

func modelFamilyApprox(model string) (func(string) int, bool) {
	for _, a := range modelApproximators {
		if strings.HasPrefix(model, a.prefix) {
			return a.estimate, true
		}
	}
	return nil, false
}
