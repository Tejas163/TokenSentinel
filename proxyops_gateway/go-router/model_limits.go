package main

import (
	"os"
	"strconv"
)

type modelLimit struct {
	maxInputTokens  int
	maxOutputTokens int
}

var modelLimits = []struct {
	prefix string
	limit  modelLimit
}{
	{"gpt-4o",                      modelLimit{maxInputTokens: 128000, maxOutputTokens: 16384}},
	{"gpt-4-turbo",                 modelLimit{maxInputTokens: 128000, maxOutputTokens: 16384}},
	{"gpt-4-32k",                   modelLimit{maxInputTokens: 32768,  maxOutputTokens: 16384}},
	{"gpt-4",                       modelLimit{maxInputTokens: 8192,   maxOutputTokens: 8192}},
	{"gpt-4o-mini",                 modelLimit{maxInputTokens: 128000, maxOutputTokens: 16384}},
	{"gpt-3.5-turbo",              modelLimit{maxInputTokens: 16385,  maxOutputTokens: 4096}},
	{"claude-3-opus",               modelLimit{maxInputTokens: 200000, maxOutputTokens: 4096}},
	{"claude-3-sonnet",             modelLimit{maxInputTokens: 200000, maxOutputTokens: 4096}},
	{"claude-3-haiku",              modelLimit{maxInputTokens: 200000, maxOutputTokens: 4096}},
	{"claude-3.5-sonnet",           modelLimit{maxInputTokens: 200000, maxOutputTokens: 8192}},
	{"claude-3.5-haiku",            modelLimit{maxInputTokens: 200000, maxOutputTokens: 8192}},
	{"llama-3-8b",                  modelLimit{maxInputTokens: 8192,   maxOutputTokens: 4096}},
	{"llama-3-70b",                 modelLimit{maxInputTokens: 8192,   maxOutputTokens: 4096}},
	{"llama-3.1-405b",              modelLimit{maxInputTokens: 131072, maxOutputTokens: 4096}},
	{"mixtral-8x7b",                modelLimit{maxInputTokens: 32768,  maxOutputTokens: 4096}},
	{"mistral-large",               modelLimit{maxInputTokens: 128000, maxOutputTokens: 4096}},
	{"mistral-medium",              modelLimit{maxInputTokens: 32000,  maxOutputTokens: 4096}},
	{"mistral-small",               modelLimit{maxInputTokens: 32000,  maxOutputTokens: 4096}},
	{"gemini-1.5-pro",              modelLimit{maxInputTokens: 2097152, maxOutputTokens: 8192}},
	{"gemini-1.5-flash",            modelLimit{maxInputTokens: 1048576, maxOutputTokens: 8192}},
	{"gemini-2.0-pro",              modelLimit{maxInputTokens: 2097152, maxOutputTokens: 8192}},
	{"gemini-2.0-flash",            modelLimit{maxInputTokens: 1048576, maxOutputTokens: 8192}},
}

var defaultModelLimit = modelLimit{maxInputTokens: 4096, maxOutputTokens: 4096}

func getModelLimit(model string) modelLimit {
	for _, entry := range modelLimits {
		if matchModelPrefix(model, entry.prefix) {
			return entry.limit
		}
	}
	return defaultModelLimit
}

func matchModelPrefix(model, prefix string) bool {
	if len(model) < len(prefix) {
		return false
	}
	return model[:len(prefix)] == prefix
}

func applyBodyLimitOverride(limit modelLimit) modelLimit {
	if s := os.Getenv("MODEL_MAX_INPUT_TOKENS"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			limit.maxInputTokens = v
		}
	}
	if s := os.Getenv("MODEL_MAX_OUTPUT_TOKENS"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			limit.maxOutputTokens = v
		}
	}
	return limit
}
