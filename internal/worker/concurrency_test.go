package worker_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/faisalaffan/ewallet-system/internal/domain"
	"github.com/faisalaffan/ewallet-system/internal/worker"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestEventBus_PubSub(t *testing.T) {
	bus := worker.NewEventBus()
	ch := bus.Subscribe("ledger", 10)

	entry := domain.LedgerEntry{
		EntryID:   uuid.New(),
		WalletID:  uuid.New(),
		EntryType: domain.EntryTypeTopUp,
		Amount:    "100.00",
	}

	bus.Publish("ledger", entry)

	select {
	case received := <-ch:
		evt, ok := received.(domain.LedgerEntry)
		assert.True(t, ok)
		assert.Equal(t, entry.Amount, evt.Amount)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	bus.Close()
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	bus := worker.NewEventBus()
	ch1 := bus.Subscribe("ledger", 10)
	ch2 := bus.Subscribe("ledger", 10)

	entry := domain.LedgerEntry{
		EntryID:   uuid.New(),
		WalletID:  uuid.New(),
		EntryType: domain.EntryTypePayment,
		Amount:    "50.00",
	}

	bus.Publish("ledger", entry)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		select {
		case <-ch1:
		case <-time.After(time.Second):
			t.Error("ch1 timeout")
		}
	}()

	go func() {
		defer wg.Done()
		select {
		case <-ch2:
		case <-time.After(time.Second):
			t.Error("ch2 timeout")
		}
	}()

	wg.Wait()
	bus.Close()
}

func TestEventBus_Close(t *testing.T) {
	bus := worker.NewEventBus()
	ch := bus.Subscribe("ledger", 10)
	bus.Close()

	// channel should be closed
	_, open := <-ch
	assert.False(t, open, "channel should be closed after bus.Close()")
}

func TestEventBus_ConcurrentPublish(t *testing.T) {
	bus := worker.NewEventBus()
	ch := bus.Subscribe("ledger", 1000)

	var wg sync.WaitGroup
	n := 100

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(amt string) {
			defer wg.Done()
			bus.Publish("ledger", domain.LedgerEntry{
				EntryID:   uuid.New(),
				WalletID:  uuid.New(),
				EntryType: domain.EntryTypeTopUp,
				Amount:    amt,
			})
		}("100.00")
	}

	wg.Wait()

	// drain channel
	time.Sleep(50 * time.Millisecond)
	count := len(ch)
	assert.Equal(t, n, count, "all messages should be delivered")

	bus.Close()
}

func TestReconcileWorker_StartShutdown(t *testing.T) {
	// Just verify it doesn't panic
	w := worker.NewReconcileWorker(nil, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		w.Start(ctx)
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	wg.Wait()
	w.Shutdown()
}
