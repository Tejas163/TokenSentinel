package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

func syncModelCatalogToRedis(ctx context.Context) {
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("pricing: redis unavailable, skipping catalog sync: %v", err)
		return
	}
	for _, m := range modelCatalog {
		key := fmt.Sprintf("pricing:%s", m.Name)
		data := map[string]interface{}{
			"name":         m.Name,
			"provider":     m.Provider,
			"tier":         int(m.Tier),
			"input_price":  m.InputPrice,
			"output_price": m.OutputPrice,
		}
		if err := rdb.HSet(ctx, key, data).Err(); err != nil {
			log.Printf("pricing: failed to sync %s: %v", m.Name, err)
		}
	}
	log.Printf("pricing: synced %d models to redis", len(modelCatalog))
}

func findModelFromRedis(ctx context.Context, name string) *ModelInfo {
	if rdb == nil {
		return nil
	}
	key := fmt.Sprintf("pricing:%s", strings.ToLower(name))
	data, err := rdb.HGetAll(ctx, key).Result()
	if err != nil || len(data) == 0 {
		return nil
	}
	mi := &ModelInfo{Name: name}
	if v, ok := data["provider"]; ok {
		mi.Provider = v
	}
	if v, ok := data["input_price"]; ok {
		fmt.Sscanf(v, "%f", &mi.InputPrice)
	}
	if v, ok := data["output_price"]; ok {
		fmt.Sscanf(v, "%f", &mi.OutputPrice)
	}
	if v, ok := data["tier"]; ok {
		var t int
		fmt.Sscanf(v, "%d", &t)
		mi.Tier = ModelTier(t)
	}
	return mi
}

func findModel(name string) *ModelInfo {
	name = strings.ToLower(name)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if mi := findModelFromRedis(ctx, name); mi != nil {
		return mi
	}
	for _, m := range modelCatalog {
		if strings.EqualFold(m.Name, name) {
			return &m
		}
	}
	return nil
}

func handleAdminPricing(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		listPricing(w, r)
	case http.MethodPost:
		upsertPricing(w, r)
	case http.MethodDelete:
		deletePricing(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type pricingEntry struct {
	Name        string  `json:"name"`
	Provider    string  `json:"provider"`
	InputPrice  float64 `json:"input_price"`
	OutputPrice float64 `json:"output_price"`
	Tier        int     `json:"tier"`
}

func listPricing(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	entries := getAllPricing(ctx)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func getAllPricing(ctx context.Context) []pricingEntry {
	seen := make(map[string]bool)
	var entries []pricingEntry

	keys, err := rdb.Keys(ctx, "pricing:*").Result()
	if err == nil {
		for _, key := range keys {
			name := strings.TrimPrefix(key, "pricing:")
			data, err := rdb.HGetAll(ctx, key).Result()
			if err != nil || len(data) == 0 {
				continue
			}
			e := pricingEntry{Name: name}
			if v, ok := data["provider"]; ok {
				e.Provider = v
			}
			fmt.Sscanf(data["input_price"], "%f", &e.InputPrice)
			fmt.Sscanf(data["output_price"], "%f", &e.OutputPrice)
			fmt.Sscanf(data["tier"], "%d", &e.Tier)
			entries = append(entries, e)
			seen[name] = true
		}
	}

	for _, m := range modelCatalog {
		if !seen[m.Name] {
			entries = append(entries, pricingEntry{
				Name:        m.Name,
				Provider:    m.Provider,
				InputPrice:  m.InputPrice,
				OutputPrice: m.OutputPrice,
				Tier:        int(m.Tier),
			})
		}
	}
	return entries
}

func upsertPricing(w http.ResponseWriter, r *http.Request) {
	var e pricingEntry
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if e.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	key := fmt.Sprintf("pricing:%s", strings.ToLower(e.Name))
	data := map[string]interface{}{
		"name":         e.Name,
		"provider":     e.Provider,
		"input_price":  e.InputPrice,
		"output_price": e.OutputPrice,
		"tier":         e.Tier,
	}
	if err := rdb.HSet(ctx, key, data).Err(); err != nil {
		log.Printf("pricing: upsert failed for %s: %v", e.Name, err)
		http.Error(w, "redis error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(e)
}

func deletePricing(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/admin/pricing/")
	name = strings.TrimPrefix(name, "/v1/admin/pricing/")
	name = strings.Trim(name, "/")
	if name == "" {
		http.Error(w, "model name required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	key := fmt.Sprintf("pricing:%s", strings.ToLower(name))
	if err := rdb.Del(ctx, key).Err(); err != nil {
		log.Printf("pricing: delete failed for %s: %v", name, err)
		http.Error(w, "redis error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
