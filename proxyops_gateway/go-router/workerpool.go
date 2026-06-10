package main

import (
	"context"
	"log/slog"
	"runtime"
)

type costTask struct {
	reqID        string
	model        string
	inputTokens  int
	outputTokens int
	team         string
}

func (t costTask) execute(ctx context.Context) {
	recordCost(ctx, t.reqID, t.model, t.team, t.inputTokens, t.outputTokens)
}

type cacheTask struct {
	promptText string
	resp       *CachedResponse
}

func (t cacheTask) execute(ctx context.Context) {
	if semanticCache != nil {
		if err := semanticCache.Store(ctx, t.promptText, t.resp); err != nil {
			slog.Error("failed to store semantic cache", "error", err)
		}
	}
}

type workerPool struct {
	tasks chan task
}

type task interface {
	execute(ctx context.Context)
}

func newWorkerPool(size int) *workerPool {
	if size <= 0 {
		size = runtime.NumCPU() * 2
	}
	p := &workerPool{
		tasks: make(chan task, size*4),
	}
	for i := 0; i < size; i++ {
		go p.worker()
	}
	return p
}

func (p *workerPool) worker() {
	for t := range p.tasks {
		t.execute(context.Background())
	}
}

func (p *workerPool) Submit(t task) {
	select {
	case p.tasks <- t:
	default:
		slog.Warn("worker pool full, dropping task")
	}
}

func (p *workerPool) Stop() {
	close(p.tasks)
}
