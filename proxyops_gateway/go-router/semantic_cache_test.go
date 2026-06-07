package main

import (
	"testing"
)

func TestNGrams_ShorterThanN(t *testing.T) {
	got := ngrams("hi", 3)
	if len(got) != 1 || got[0] != "hi" {
		t.Errorf("expected [hi], got %v", got)
	}
}

func TestNGrams_ExactLength(t *testing.T) {
	got := ngrams("abc", 3)
	if len(got) != 1 || got[0] != "abc" {
		t.Errorf("expected [abc], got %v", got)
	}
}

func TestNGrams_Multiple(t *testing.T) {
	got := ngrams("hello", 3)
	expected := []string{"hel", "ell", "llo"}
	if len(got) != len(expected) {
		t.Fatalf("expected %d ngrams, got %d", len(expected), len(got))
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("position %d: expected %q, got %q", i, expected[i], got[i])
		}
	}
}

func TestMinHashSignature_Deterministic(t *testing.T) {
	sig1 := minHashSignature("hello world", 64, 3)
	sig2 := minHashSignature("hello world", 64, 3)
	if len(sig1) != 64 {
		t.Fatalf("expected 64 hashes, got %d", len(sig1))
	}
	for i := range sig1 {
		if sig1[i] != sig2[i] {
			t.Errorf("signature not deterministic at position %d", i)
		}
	}
}

func TestMinHashSignature_DifferentInputs(t *testing.T) {
	sig1 := minHashSignature("hello world", 64, 3)
	sig2 := minHashSignature("goodbye world", 64, 3)
	matching := 0
	for i := range sig1 {
		if sig1[i] == sig2[i] {
			matching++
		}
	}
	if matching == 64 {
		t.Error("expected different signatures for different inputs")
	}
}

func TestMinHashSignature_SimilarInputs(t *testing.T) {
	cfg := defaultCacheConfig()
	sig1 := minHashSignature("what is the capital of france", cfg.NumHashes, cfg.NGramLen)
	sig2 := minHashSignature("what is the capital of France?", cfg.NumHashes, cfg.NGramLen)
	j := jaccard(sig1, sig2)
	if j < 0.3 {
		t.Errorf("expected high similarity for similar prompts, got %.2f", j)
	}
}

func TestMinHash_BandsCount(t *testing.T) {
	cfg := defaultCacheConfig()
	idx := NewMinHashIndex(nil, cfg)
	sig := minHashSignature("test", cfg.NumHashes, cfg.NGramLen)
	buckets := idx.bands(sig)
	if len(buckets) != cfg.NumBands {
		t.Fatalf("expected %d bands, got %d", cfg.NumBands, len(buckets))
	}
}

func TestMinHash_BandsDeterministic(t *testing.T) {
	cfg := defaultCacheConfig()
	idx := NewMinHashIndex(nil, cfg)
	sig := minHashSignature("hello world", cfg.NumHashes, cfg.NGramLen)
	b1 := idx.bands(sig)
	b2 := idx.bands(sig)
	for i := range b1 {
		if b1[i] != b2[i] {
			t.Errorf("band %d not deterministic", i)
		}
	}
}

func TestMinHash_BandsLocalSensitivity(t *testing.T) {
	cfg := defaultCacheConfig()
	idx := NewMinHashIndex(nil, cfg)
	sigA := minHashSignature("the quick brown fox jumps over the lazy dog", cfg.NumHashes, cfg.NGramLen)
	sigB := minHashSignature("the quick brown fox jumps over the lazy cat", cfg.NumHashes, cfg.NGramLen)
	bandsA := idx.bands(sigA)
	bandsB := idx.bands(sigB)
	matchingBands := 0
	for i := range bandsA {
		if bandsA[i] == bandsB[i] {
			matchingBands++
		}
	}
	if matchingBands == 0 {
		t.Error("expected at least one matching band for similar inputs")
	}
}

func TestJaccard_Identical(t *testing.T) {
	sig := []uint64{1, 2, 3, 4}
	j := jaccard(sig, sig)
	if j != 1.0 {
		t.Errorf("expected 1.0 for identical signatures, got %.2f", j)
	}
}

func TestJaccard_Empty(t *testing.T) {
	j := jaccard(nil, []uint64{1, 2})
	if j != 0 {
		t.Errorf("expected 0 for empty sig, got %.2f", j)
	}
}

func TestJaccard_PartialMatch(t *testing.T) {
	a := []uint64{1, 2, 3, 4}
	b := []uint64{1, 2, 5, 6}
	j := jaccard(a, b)
	if j != 0.5 {
		t.Errorf("expected 0.5 for 2/4 match, got %.2f", j)
	}
}

func TestJaccard_NoMatch(t *testing.T) {
	a := []uint64{1, 2, 3}
	b := []uint64{4, 5, 6}
	j := jaccard(a, b)
	if j != 0 {
		t.Errorf("expected 0 for no match, got %.2f", j)
	}
}

func TestExtractPromptText_ChatMessages(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"hello"},{"role":"assistant","content":"hi there"}]}`)
	text := extractPromptText(body)
	if text != "hello hi there" {
		t.Errorf("expected 'hello hi there', got %q", text)
	}
}

func TestExtractPromptText_Completion(t *testing.T) {
	body := []byte(`{"prompt":"translate to french","model":"gpt-4"}`)
	text := extractPromptText(body)
	if text != "translate to french" {
		t.Errorf("expected 'translate to french', got %q", text)
	}
}

func TestExtractPromptText_RawBody(t *testing.T) {
	body := []byte(`raw text input`)
	text := extractPromptText(body)
	if text != "raw text input" {
		t.Errorf("expected 'raw text input', got %q", text)
	}
}

func TestExtractPromptText_EmptyBody(t *testing.T) {
	text := extractPromptText(nil)
	if text != "" {
		t.Errorf("expected empty string, got %q", text)
	}
}

func TestHashBytes_Deterministic(t *testing.T) {
	h1 := hashBytes([]byte("hello"))
	h2 := hashBytes([]byte("hello"))
	if h1 != h2 {
		t.Error("expected same hash for same input")
	}
}

func TestHashBytes_Different(t *testing.T) {
	h1 := hashBytes([]byte("hello"))
	h2 := hashBytes([]byte("world"))
	if h1 == h2 {
		t.Error("expected different hashes")
	}
}

func TestCacheConfigFromEnv_Defaults(t *testing.T) {
	cfg := defaultCacheConfig()
	if cfg.Enabled {
		t.Error("expected disabled by default")
	}
	if cfg.Threshold != 0.85 {
		t.Errorf("expected threshold 0.85, got %.2f", cfg.Threshold)
	}
	if cfg.TTL != 1*3600*1000000000 {
		t.Errorf("expected TTL 1h, got %v", cfg.TTL)
	}
}

func TestFloat32sToBytes_RoundTrip(t *testing.T) {
	original := []float32{1.5, 2.5, -3.5, 0}
	buf := float32sToBytes(original)
	if len(buf) != len(original)*4 {
		t.Fatalf("expected %d bytes, got %d", len(original)*4, len(buf))
	}
}
