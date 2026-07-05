package worker

import (
	"log"
	"sync"

	"github.com/faisalaffan/ewallet-system/internal/domain"
)

type LedgerEvent struct {
	Entry    domain.LedgerEntry
	WalletID string
}

type EventBus struct {
	mu       sync.RWMutex
	subs     map[string][]chan any
	wg       sync.WaitGroup
	closed   bool
}

func NewEventBus() *EventBus {
	return &EventBus{
		subs: make(map[string][]chan any),
	}
}

func (b *EventBus) Subscribe(topic string, bufSize int) <-chan any {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan any, bufSize)
	b.subs[topic] = append(b.subs[topic], ch)
	return ch
}

func (b *EventBus) Publish(topic string, evt any) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return
	}
	for _, ch := range b.subs[topic] {
		select {
		case ch <- evt:
		default:
			log.Printf("WARN: event bus channel full, dropping event for topic %s", topic)
		}
	}
}

func (b *EventBus) Close() {
	b.mu.Lock()
	b.closed = true
	for _, chs := range b.subs {
		for _, ch := range chs {
			close(ch)
		}
	}
	b.mu.Unlock()
	b.wg.Wait()
}

func (b *EventBus) Done() {
	b.wg.Done()
}

func (b *EventBus) Add(delta int) {
	b.wg.Add(delta)
}
