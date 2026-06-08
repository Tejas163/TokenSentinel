package main

import (
	"sync"
	"sync/atomic"
	"time"
)

type contextKey string

const reqIDKey contextKey = "request_id"

var sensitiveHeaders = map[string]bool{
	"authorization":       true,
	"cookie":              true,
	"set-cookie":          true,
	"x-api-key":           true,
	"proxy-authorization": true,
}

func isSensitiveHeader(k string) bool {
	return sensitiveHeaders[k]
}

type CircuitState int32

const (
	Closed CircuitState = iota
	Open
	HalfOpen
)

type CircuitBreaker struct {
	state     atomic.Int32
	failures  atomic.Int32
	threshold int32
	cooldown  time.Duration
	mu        sync.Mutex
	openSince time.Time
}

func NewCircuitBreaker(threshold int, cooldown time.Duration) *CircuitBreaker {
	cb := &CircuitBreaker{threshold: int32(threshold), cooldown: cooldown}
	cb.state.Store(int32(Closed))
	return cb
}

func (cb *CircuitBreaker) Allow() bool {
	state := CircuitState(cb.state.Load())
	if state == Open {
		cb.mu.Lock()
		defer cb.mu.Unlock()
		if time.Since(cb.openSince) > cb.cooldown {
			cb.state.Store(int32(HalfOpen))
			return true
		}
		return false
	}
	return true
}

func (cb *CircuitBreaker) Success() {
	cb.state.Store(int32(Closed))
	cb.failures.Store(0)
}

func (cb *CircuitBreaker) Failure() {
	f := cb.failures.Add(1)
	if f >= cb.threshold {
		cb.mu.Lock()
		cb.state.Store(int32(Open))
		cb.openSince = time.Now()
		cb.mu.Unlock()
	}
}

type UpstreamConfig struct {
	URL     string `json:"url"`
	Timeout int    `json:"timeout"`
	Model   string `json:"model"`
	Weight  int    `json:"weight"`
}

type RouteConfig struct {
	Pattern   string           `json:"pattern"`
	Providers []UpstreamConfig `json:"providers"`
	AutoModel bool             `json:"auto_model"`
}

type routeCacheEntry struct {
	cfg    *RouteConfig
	expiry time.Time
}

type ErrorResponse struct {
	Error     string `json:"error"`
	Request   string `json:"request_id,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

var cheapModels = []string{"gpt-3.5", "claude-3-haiku", "llama-3-8b", "mistral-small", "gemini-1.5-flash"}
