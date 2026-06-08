package main

import "log/slog"

func newSSEBroker() *sseBroker {
	b := &sseBroker{
		subs:  make(map[chan sseEvent]bool),
		reg:   make(chan chan sseEvent),
		unreg: make(chan chan sseEvent),
		broad: make(chan sseEvent, 64),
	}
	go b.run()
	return b
}

func (b *sseBroker) broadcast(evt sseEvent) {
	select {
	case b.broad <- evt:
	default:
		slog.Warn("sse: broker channel full, dropping event", "type", evt.Type)
	}
}

func (b *sseBroker) run() {
	for {
		select {
		case ch := <-b.reg:
			b.subs[ch] = true
		case ch := <-b.unreg:
			delete(b.subs, ch)
			close(ch)
		case ev := <-b.broad:
			for ch := range b.subs {
				select {
				case ch <- ev:
				default:
				}
			}
		}
	}
}

func (b *sseBroker) subscribe() chan sseEvent {
	ch := make(chan sseEvent, 16)
	b.reg <- ch
	return ch
}

func (b *sseBroker) unsubscribe(ch chan sseEvent) {
	b.unreg <- ch
}