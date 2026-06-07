package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type SimilarityIndex interface {
	Store(ctx context.Context, promptText string, resp *CachedResponse) error
	Lookup(ctx context.Context, promptText string) (*CachedResponse, error)
}

type CachedResponse struct {
	Body         []byte `json:"body"`
	Model        string `json:"model"`
	StatusCode   int    `json:"status_code"`
	OutputTokens int    `json:"output_tokens"`
	CachedAt     int64  `json:"cached_at"`
}

type SemanticCacheConfig struct {
	Enabled   bool
	Threshold float64
	TTL       time.Duration
	NumHashes int
	NumBands  int
	NGramLen  int
}

func defaultCacheConfig() SemanticCacheConfig {
	return SemanticCacheConfig{
		Enabled:   false,
		Threshold: 0.85,
		TTL:       1 * time.Hour,
		NumHashes: 64,
		NumBands:  16,
		NGramLen:  3,
	}
}

func cacheConfigFromEnv() SemanticCacheConfig {
	cfg := defaultCacheConfig()
	if v := os.Getenv("SEMANTIC_CACHE_ENABLED"); v == "true" || v == "1" {
		cfg.Enabled = true
	}
	if v := os.Getenv("SEMANTIC_CACHE_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f <= 1 {
			cfg.Threshold = f
		}
	}
	if v := os.Getenv("SEMANTIC_CACHE_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.TTL = d
		}
	}
	return cfg
}

type MinHashIndex struct {
	rdb  *redis.Client
	cfg  SemanticCacheConfig
	bandSize int
}

func NewMinHashIndex(rdb *redis.Client, cfg SemanticCacheConfig) *MinHashIndex {
	return &MinHashIndex{
		rdb:  rdb,
		cfg:  cfg,
		bandSize: cfg.NumHashes / cfg.NumBands,
	}
}

func (m *MinHashIndex) Enabled() bool {
	return m.cfg.Enabled
}

func extractPromptText(body []byte) string {
	var chat struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &chat); err == nil && len(chat.Messages) > 0 {
		var sb strings.Builder
		for _, msg := range chat.Messages {
			sb.WriteString(msg.Content)
			sb.WriteString(" ")
		}
		return strings.TrimSpace(sb.String())
	}

	var completion struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(body, &completion); err == nil && completion.Prompt != "" {
		return completion.Prompt
	}

	return string(body)
}

func ngrams(text string, n int) []string {
	runes := []rune(text)
	if len(runes) < n {
		return []string{text}
	}
	result := make([]string, 0, len(runes)-n+1)
	for i := 0; i <= len(runes)-n; i++ {
		result = append(result, string(runes[i:i+n]))
	}
	return result
}

func hashBytes(data []byte) uint64 {
	h := fnv.New64a()
	h.Write(data)
	return h.Sum64()
}

func minHashSignature(text string, numHashes, ngramLen int) []uint64 {
	grams := ngrams(text, ngramLen)
	if len(grams) == 0 {
		return make([]uint64, numHashes)
	}

	sig := make([]uint64, numHashes)
	seedBuf := make([]byte, 8)

	for i := 0; i < numHashes; i++ {
		minVal := uint64(math.MaxUint64)
		binary.LittleEndian.PutUint64(seedBuf, uint64(i))

		for _, g := range grams {
			h := fnv.New64a()
			h.Write(seedBuf)
			h.Write([]byte(g))
			val := h.Sum64()
			if val < minVal {
				minVal = val
			}
		}
		sig[i] = minVal
	}
	return sig
}

func (m *MinHashIndex) bands(sig []uint64) []uint64 {
	buckets := make([]uint64, m.cfg.NumBands)
	seedBuf := make([]byte, 8)
	rowBuf := make([]byte, 8)

	for b := 0; b < m.cfg.NumBands; b++ {
		h := fnv.New64a()
		binary.LittleEndian.PutUint64(seedBuf, uint64(b))
		h.Write(seedBuf)

		for r := 0; r < m.bandSize; r++ {
			idx := b*m.bandSize + r
			if idx < len(sig) {
				binary.LittleEndian.PutUint64(rowBuf, sig[idx])
				h.Write(rowBuf)
			}
		}
		buckets[b] = h.Sum64()
	}
	return buckets
}

func jaccard(sigA, sigB []uint64) float64 {
	if len(sigA) != len(sigB) || len(sigA) == 0 {
		return 0
	}
	matching := 0
	for i := 0; i < len(sigA); i++ {
		if sigA[i] == sigB[i] {
			matching++
		}
	}
	return float64(matching) / float64(len(sigA))
}

func (m *MinHashIndex) Store(ctx context.Context, promptText string, resp *CachedResponse) error {
	if !m.cfg.Enabled || promptText == "" {
		return nil
	}

	sig := minHashSignature(promptText, m.cfg.NumHashes, m.cfg.NGramLen)

	sigData, err := json.Marshal(sig)
	if err != nil {
		return fmt.Errorf("marshal sig: %w", err)
	}

	respData, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal resp: %w", err)
	}

	entryKey := fmt.Sprintf("semantic:entry:%x", hashBytes([]byte(promptText)))

	pipe := m.rdb.Pipeline()
	pipe.HSet(ctx, entryKey, "sig", sigData, "resp", respData)
	pipe.Expire(ctx, entryKey, m.cfg.TTL)

	buckets := m.bands(sig)
	for _, bucket := range buckets {
		bucketKey := fmt.Sprintf("semantic:bucket:%d:%x", bucket%64, bucket)
		pipe.SAdd(ctx, bucketKey, entryKey)
		pipe.Expire(ctx, bucketKey, m.cfg.TTL)
	}

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("store cache: %w", err)
	}
	return nil
}

func (m *MinHashIndex) Lookup(ctx context.Context, promptText string) (*CachedResponse, error) {
	if !m.cfg.Enabled || promptText == "" {
		return nil, nil
	}

	querySig := minHashSignature(promptText, m.cfg.NumHashes, m.cfg.NGramLen)
	buckets := m.bands(querySig)

	candidateKeys := map[string]struct{}{}
	for _, bucket := range buckets {
		bucketKey := fmt.Sprintf("semantic:bucket:%d:%x", bucket%64, bucket)
		members, err := m.rdb.SMembers(ctx, bucketKey).Result()
		if err != nil {
			continue
		}
		for _, k := range members {
			candidateKeys[k] = struct{}{}
		}
	}

	if len(candidateKeys) == 0 {
		return nil, nil
	}

	var bestResp *CachedResponse
	bestSim := 0.0

	for key := range candidateKeys {
		data, err := m.rdb.HGetAll(ctx, key).Result()
		if err != nil || len(data) == 0 {
			continue
		}

		var cachedResp CachedResponse
		if respRaw, ok := data["resp"]; ok {
			if err := json.Unmarshal([]byte(respRaw), &cachedResp); err != nil {
				continue
			}
		} else {
			continue
		}

		var storedSig []uint64
		if sigRaw, ok := data["sig"]; ok {
			if err := json.Unmarshal([]byte(sigRaw), &storedSig); err != nil {
				continue
			}
		} else {
			continue
		}

		sim := jaccard(querySig, storedSig)
		if sim > bestSim {
			bestSim = sim
			bestResp = &cachedResp
		}
	}

	if bestResp != nil && bestSim >= m.cfg.Threshold {
		slog.Debug("semantic cache hit", "similarity", bestSim, "threshold", m.cfg.Threshold, "model", bestResp.Model)
		return bestResp, nil
	}

	return nil, nil
}

type EmbeddingIndex struct {
	rdb        *redis.Client
	cfg        SemanticCacheConfig
	embedURL   string
	embedModel string
	embedDim   int
	httpClient *http.Client
	initOnce   sync.Once
}

type embeddingRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func NewEmbeddingIndex(rdb *redis.Client, cfg SemanticCacheConfig) *EmbeddingIndex {
	dim := 1536
	if v := os.Getenv("EMBEDDING_DIM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			dim = n
		}
	}
	return &EmbeddingIndex{
		rdb:        rdb,
		cfg:        cfg,
		embedURL:   getEnv("EMBEDDING_API_URL", "https://api.openai.com/v1/embeddings"),
		embedModel: getEnv("EMBEDDING_MODEL", "text-embedding-ada-002"),
		embedDim:   dim,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *EmbeddingIndex) getEmbedding(ctx context.Context, text string) ([]float32, error) {
	cacheKey := fmt.Sprintf("semantic:embed:%x", hashBytes([]byte(text)))
	cached, err := e.rdb.Get(ctx, cacheKey).Result()
	if err == nil {
		var vec []float32
		if err := json.Unmarshal([]byte(cached), &vec); err == nil && len(vec) == e.embedDim {
			return vec, nil
		}
	}

	body, err := json.Marshal(embeddingRequest{
		Input: []string{text},
		Model: e.embedModel,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.embedURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key := os.Getenv("EMBEDDING_API_KEY"); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embed response: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("embed API error %d: %s", resp.StatusCode, string(respBody))
	}

	var embedResp embeddingResponse
	if err := json.Unmarshal(respBody, &embedResp); err != nil {
		return nil, fmt.Errorf("parse embed response: %w", err)
	}
	if len(embedResp.Data) == 0 {
		return nil, fmt.Errorf("no embedding data in response")
	}

	vec := embedResp.Data[0].Embedding
	if len(vec) != e.embedDim {
		return nil, fmt.Errorf("unexpected embedding dimension %d (expected %d)", len(vec), e.embedDim)
	}

	vecData, _ := json.Marshal(vec)
	e.rdb.Set(ctx, cacheKey, vecData, 24*time.Hour)

	return vec, nil
}

func (e *EmbeddingIndex) ensureIndex(ctx context.Context) error {
	var initErr error
	e.initOnce.Do(func() {
		idxKey := "idx:semantic"
		exists, err := e.rdb.Do(ctx, "FT.INFO", idxKey).Result()
		if err == nil && exists != nil {
			return
		}

		initErr = e.rdb.Do(ctx,
			"FT.CREATE", idxKey,
			"ON", "HASH",
			"PREFIX", "1", "semantic:vec:",
			"SCHEMA",
			"embedding", "VECTOR", "FLAT", "6",
			"TYPE", "FLOAT32",
			"DIM", e.embedDim,
			"DISTANCE_METRIC", "COSINE",
		).Err()
	})
	return initErr
}

func float32sToBytes(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

func (e *EmbeddingIndex) Store(ctx context.Context, promptText string, resp *CachedResponse) error {
	if !e.cfg.Enabled || promptText == "" {
		return nil
	}

	if err := e.ensureIndex(ctx); err != nil {
		return fmt.Errorf("ensure vector index: %w", err)
	}

	vec, err := e.getEmbedding(ctx, promptText)
	if err != nil {
		return fmt.Errorf("get embedding: %w", err)
	}

	respData, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal resp: %w", err)
	}

	entryKey := fmt.Sprintf("semantic:vec:%x", hashBytes([]byte(promptText)))

	err = e.rdb.HSet(ctx, entryKey,
		"embedding", float32sToBytes(vec),
		"resp", respData,
		"prompt", promptText,
	).Err()
	if err != nil {
		return fmt.Errorf("hset vector entry: %w", err)
	}

	e.rdb.Expire(ctx, entryKey, e.cfg.TTL)
	return nil
}

func (e *EmbeddingIndex) Lookup(ctx context.Context, promptText string) (*CachedResponse, error) {
	if !e.cfg.Enabled || promptText == "" {
		return nil, nil
	}

	if err := e.ensureIndex(ctx); err != nil {
		return nil, fmt.Errorf("ensure vector index: %w", err)
	}

	queryVec, err := e.getEmbedding(ctx, promptText)
	if err != nil {
		return nil, fmt.Errorf("get query embedding: %w", err)
	}

	queryBytes := float32sToBytes(queryVec)

	results, err := e.rdb.Do(ctx,
		"FT.SEARCH", "idx:semantic",
		"*=>[KNN 5 @embedding $vec AS score]",
		"PARAMS", "2", "vec", queryBytes,
		"SORTBY", "score", "ASC",
		"RETURN", "4", "score", "resp", "prompt",
		"DIALECT", "2",
	).Result()
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	respArr, ok := results.([]interface{})
	if !ok || len(respArr) < 2 {
		return nil, nil
	}

	var bestResp *CachedResponse
	bestSim := 0.0

	for i := 1; i < len(respArr); i++ {
		entry, ok := respArr[i].([]interface{})
		if !ok || len(entry) < 2 {
			continue
		}

		fields, ok := entry[1].([]interface{})
		if !ok {
			continue
		}

		var score float64
		var respRaw string
		for j := 0; j < len(fields)-1; j += 2 {
			key, _ := fields[j].(string)
			val, _ := fields[j+1].(string)
			switch key {
			case "score":
				score, _ = strconv.ParseFloat(val, 64)
			case "resp":
				respRaw = val
			}
		}

		if respRaw == "" {
			continue
		}

		var cachedResp CachedResponse
		if err := json.Unmarshal([]byte(respRaw), &cachedResp); err != nil {
			continue
		}

		sim := 1.0 - score
		if sim > bestSim {
			bestSim = sim
			bestResp = &cachedResp
		}
	}

	if bestResp != nil && bestSim >= e.cfg.Threshold {
		slog.Debug("semantic cache hit (embedding)", "similarity", bestSim, "threshold", e.cfg.Threshold, "model", bestResp.Model)
		return bestResp, nil
	}

	return nil, nil
}

type cacheMode int

const (
	cacheModeDisabled cacheMode = iota
	cacheModeMinHash
	cacheModeEmbedding
)

func parseCacheMode() cacheMode {
	if v := os.Getenv("SEMANTIC_CACHE_ENABLED"); !(v == "true" || v == "1") {
		return cacheModeDisabled
	}
	switch strings.ToLower(os.Getenv("SEMANTIC_CACHE_MODE")) {
	case "embedding", "embeddings", "vec", "vector":
		return cacheModeEmbedding
	case "minhash", "min_hash", "lsb", "lsh":
		return cacheModeMinHash
	default:
		return cacheModeMinHash
	}
}

var semanticCache SimilarityIndex

func initSemanticCache(rdb *redis.Client) {
	mode := parseCacheMode()
	switch mode {
	case cacheModeEmbedding:
		cfg := cacheConfigFromEnv()
		semanticCache = NewEmbeddingIndex(rdb, cfg)
		slog.Info("semantic cache enabled", "mode", "embedding", "threshold", cfg.Threshold, "ttl", cfg.TTL, "model", getEnv("EMBEDDING_MODEL", "text-embedding-ada-002"))
	case cacheModeMinHash:
		cfg := cacheConfigFromEnv()
		semanticCache = NewMinHashIndex(rdb, cfg)
		slog.Info("semantic cache enabled", "mode", "minhash", "threshold", cfg.Threshold, "ttl", cfg.TTL, "hashes", cfg.NumHashes, "bands", cfg.NumBands)
	default:
		semanticCache = nil
		slog.Info("semantic cache disabled (set SEMANTIC_CACHE_ENABLED=true to enable)")
	}
}
