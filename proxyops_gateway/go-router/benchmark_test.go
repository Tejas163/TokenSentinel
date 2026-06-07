package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"
)

var testRDB *redis.Client

func init() {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	testRDB = redis.NewClient(&redis.Options{Addr: addr})
}

var benchmarkPrompts = []struct {
	name string
	text string
}{
	{"short", "hello world"},
	{"medium", "What is the capital of France and what is its population?"},
	{"long", strings.Repeat("The quick brown fox jumps over the lazy dog. ", 20)},
}

var similarVariants = map[string][]string{
	"What is the capital of France and what is its population?": {
		"What is the capital of France?",
		"What is the capital of France and what is its population size?",
		"Tell me about France and its capital city",
		"What is the weather like in France?",
		"The quick brown fox jumps over the lazy dog",
	},
}

func BenchmarkNGrams(b *testing.B) {
	for _, bp := range benchmarkPrompts {
		b.Run(bp.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				ngrams(bp.text, 3)
			}
		})
	}
}

func BenchmarkMinHashSignature(b *testing.B) {
	for _, bp := range benchmarkPrompts {
		b.Run(bp.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				minHashSignature(bp.text, 64, 3)
			}
		})
	}
}

func BenchmarkMinHashBands(b *testing.B) {
	cfg := defaultCacheConfig()
	idx := NewMinHashIndex(nil, cfg)
	sig := minHashSignature(benchmarkPrompts[1].text, cfg.NumHashes, cfg.NGramLen)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.bands(sig)
	}
}

func BenchmarkJaccard(b *testing.B) {
	sigA := minHashSignature("What is the capital of France?", 64, 3)
	sigB := minHashSignature("What is the capital of France and its population?", 64, 3)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		jaccard(sigA, sigB)
	}
}

func BenchmarkExtractPromptText(b *testing.B) {
	body := []byte(`{"messages":[{"role":"user","content":"What is the capital of France?"}]}`)
	for i := 0; i < b.N; i++ {
		extractPromptText(body)
	}
}

func BenchmarkEmbeddingGet(b *testing.B) {
	ctx := context.Background()
	embedURL := os.Getenv("EMBEDDING_API_URL")
	if embedURL == "" {
		b.Skip("EMBEDDING_API_URL not set")
	}

	idx := NewEmbeddingIndex(testRDB, defaultCacheConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := idx.getEmbedding(ctx, benchmarkPrompts[1].text)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestSimilarityComparison(t *testing.T) {
	if os.Getenv("SKIP_SIMILARITY") != "" {
		t.Skip("SKIP_SIMILARITY set")
	}

	baseText := "What is the capital of France and what is its population?"
	variants := similarVariants[baseText]

	baseSig := minHashSignature(baseText, 64, 3)

	fmt.Println("\n=== Similarity Comparison: MinHash vs Embedding ===")
	fmt.Printf("%-60s %-12s %-12s %-12s\n", "Variant", "MinHash", "Match", "Expected")
	fmt.Println(strings.Repeat("-", 96))

	for _, variant := range variants {
		variantSig := minHashSignature(variant, 64, 3)
		j := jaccard(baseSig, variantSig)

		isSimilar := "NO"
		if j >= 0.85 {
			isSimilar = "YES"
		}

		expected := "different"
		if j >= 0.5 {
			expected = "similar"
		}

		fmt.Printf("%-60s %-12.4f %-12s %-12s\n", truncate(variant, 58), j, isSimilar, expected)
	}

	embedURL := os.Getenv("EMBEDDING_API_URL")
	if embedURL == "" {
		t.Log("EMBEDDING_API_URL not set — skipping embedding comparison")
		return
	}

	ctx := context.Background()
	idx := NewEmbeddingIndex(testRDB, defaultCacheConfig())

	baseVec, err := idx.getEmbedding(ctx, baseText)
	if err != nil {
		t.Fatalf("base embedding: %v", err)
	}

	fmt.Println("\n--- Embedding Cosine Similarity ---")

	for _, variant := range variants {
		variantVec, err := idx.getEmbedding(ctx, variant)
		if err != nil {
			t.Fatalf("variant embedding %q: %v", variant, err)
		}

		cos := cosineSimilarity(baseVec, variantVec)
		isSimilar := "NO"
		if cos >= 0.85 {
			isSimilar = "YES"
		}

		expected := "different"
		if cos >= 0.5 {
			expected = "similar"
		}

		fmt.Printf("%-60s %-12.4f %-12s %-12s\n", truncate(variant, 58), cos, isSimilar, expected)
	}
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		va := float64(a[i])
		vb := float64(b[i])
		dot += va * vb
		normA += va * va
		normB += vb * vb
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
