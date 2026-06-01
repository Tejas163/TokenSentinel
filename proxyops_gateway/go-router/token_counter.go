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
	if tke, err := tiktoken.EncodingForModel(model); err == nil {
		return len(tke.Encode(text, nil, nil))
	}
	modelLower := strings.ToLower(model)
	for _, p := range knownPrefixes {
		if strings.HasPrefix(modelLower, p.prefix) {
			if tke, err := tiktoken.GetEncoding(p.encoding); err == nil {
				return len(tke.Encode(text, nil, nil))
			}
		}
	}
	return len(text) / 4
}
