package main

import (
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

type ModelTier int

const (
	TierFrontier ModelTier = iota + 1
	TierCapable
	TierFast
	TierCheap
)

type ModelInfo struct {
	Name        string
	Provider    string
	Tier        ModelTier
	InputPrice  float64
	OutputPrice float64
}

var modelCatalog = []ModelInfo{
	{Name: "gpt-4", Provider: "openai", Tier: TierFrontier, InputPrice: 30.00, OutputPrice: 60.00},
	{Name: "gpt-4-turbo", Provider: "openai", Tier: TierFrontier, InputPrice: 10.00, OutputPrice: 30.00},
	{Name: "gpt-4o", Provider: "openai", Tier: TierCapable, InputPrice: 2.50, OutputPrice: 10.00},
	{Name: "gpt-4o-mini", Provider: "openai", Tier: TierFast, InputPrice: 0.15, OutputPrice: 0.60},
	{Name: "gpt-3.5-turbo", Provider: "openai", Tier: TierCheap, InputPrice: 0.50, OutputPrice: 1.50},
	{Name: "claude-3-opus", Provider: "anthropic", Tier: TierFrontier, InputPrice: 15.00, OutputPrice: 75.00},
	{Name: "claude-3-sonnet", Provider: "anthropic", Tier: TierCapable, InputPrice: 3.00, OutputPrice: 15.00},
	{Name: "claude-3-haiku", Provider: "anthropic", Tier: TierFast, InputPrice: 0.25, OutputPrice: 1.25},
	{Name: "gemini-1.5-pro", Provider: "google", Tier: TierFrontier, InputPrice: 1.25, OutputPrice: 5.00},
	{Name: "gemini-1.5-flash", Provider: "google", Tier: TierCapable, InputPrice: 0.075, OutputPrice: 0.30},
	{Name: "mistral-large", Provider: "mistral", Tier: TierFrontier, InputPrice: 2.00, OutputPrice: 6.00},
	{Name: "mistral-small", Provider: "mistral", Tier: TierFast, InputPrice: 0.60, OutputPrice: 1.80},
	{Name: "llama-3-70b", Provider: "self-hosted", Tier: TierCapable, InputPrice: 0.59, OutputPrice: 0.79},
	{Name: "llama-3-8b", Provider: "self-hosted", Tier: TierCheap, InputPrice: 0.05, OutputPrice: 0.20},
	{Name: "mixtral-8x7b", Provider: "self-hosted", Tier: TierCapable, InputPrice: 0.24, OutputPrice: 0.72},
}

func findModel(name string) *ModelInfo {
	name = strings.ToLower(name)
	for _, m := range modelCatalog {
		if strings.EqualFold(m.Name, name) {
			return &m
		}
	}
	return nil
}

func findModelWithRedis(rdb *redis.Client, name string) *ModelInfo {
	if rdb == nil {
		return findModel(name)
	}
	key := "pricing:" + strings.ToLower(name)
	data, err := rdb.HGetAll(nil, key).Result()
	if err != nil || len(data) == 0 {
		return findModel(name)
	}
	mi := &ModelInfo{Name: name}
	if v, ok := data["provider"]; ok {
		mi.Provider = v
	}
	fmt.Sscanf(data["input_price"], "%f", &mi.InputPrice)
	fmt.Sscanf(data["output_price"], "%f", &mi.OutputPrice)
	return mi
}
