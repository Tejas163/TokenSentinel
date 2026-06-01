package main

import "strings"

var modelPricing = []struct {
	prefix string
	input  float64
	output float64
}{
	{"gpt-4o", 2.50, 10.00},
	{"gpt-4-turbo", 10.00, 30.00},
	{"gpt-4", 30.00, 60.00},
	{"gpt-4o-mini", 0.15, 0.60},
	{"gpt-3.5", 0.50, 1.50},
	{"claude-3-opus", 15.00, 75.00},
	{"claude-3-sonnet", 3.00, 15.00},
	{"claude-3-haiku", 0.25, 1.25},
	{"claude-3.5-sonnet", 3.00, 15.00},
	{"claude-3.5-haiku", 0.80, 4.00},
	{"llama-3-8b", 0.05, 0.20},
	{"llama-3-70b", 0.59, 0.79},
	{"llama-3.1-405b", 2.50, 10.00},
	{"mixtral-8x7b", 0.24, 0.72},
	{"mistral-large", 2.00, 6.00},
	{"mistral-medium", 0.70, 2.10},
	{"mistral-small", 0.20, 0.60},
	{"gemini-1.5-pro", 1.25, 5.00},
	{"gemini-1.5-flash", 0.075, 0.30},
	{"gemini-2.0-pro", 2.50, 10.00},
	{"gemini-2.0-flash", 0.10, 0.40},
}

func estimateCost(inputTokens, outputTokens int, model string) float64 {
	lower := strings.ToLower(model)
	for _, p := range modelPricing {
		if strings.HasPrefix(lower, p.prefix) {
			return (float64(inputTokens)/1000*p.input + float64(outputTokens)/1000*p.output) / 100
		}
	}
	return 0
}
