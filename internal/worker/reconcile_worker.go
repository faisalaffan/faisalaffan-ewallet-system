package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"
)

type ReconcileWorker struct {
	db       *gorm.DB
	interval time.Duration
	wg       sync.WaitGroup
}

func NewReconcileWorker(db *gorm.DB, interval time.Duration) *ReconcileWorker {
	if interval <= 0 {
		interval = 1 * time.Hour
	}
	return &ReconcileWorker{db: db, interval: interval}
}

func (w *ReconcileWorker) Start(ctx context.Context) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()

		slog.Info("reconcile-worker started", "interval", w.interval.String())

		for {
			select {
			case <-ctx.Done():
				slog.Info("reconcile-worker stopped")
				return
			case <-ticker.C:
				w.run(ctx)
			}
		}
	}()
}

func (w *ReconcileWorker) Shutdown() {
	w.wg.Wait()
}

type walletBalance struct {
	ID      string
	Balance string
}

type ledgerTotal struct {
	WalletID string
	Total    string
}

func (w *ReconcileWorker) run(ctx context.Context) {
	var wallets []walletBalance
	if err := w.db.WithContext(ctx).
		Table("wallets").
		Select("id, balance").
		Find(&wallets).Error; err != nil {
		slog.Error("reconcile-worker failed to fetch wallets", "error", err)
		return
	}

	if len(wallets) == 0 {
		return
	}

	var totals []ledgerTotal
	if err := w.db.WithContext(ctx).
		Table("ledger_entries").
		Select("wallet_id, SUM(CASE WHEN entry_type IN ('TOPUP','TRANSFER_IN') THEN amount ELSE -amount END) as total").
		Group("wallet_id").
		Find(&totals).Error; err != nil {
		slog.Error("reconcile-worker failed to fetch ledger totals", "error", err)
		return
	}

	ledgerMap := make(map[string]string, len(totals))
	for _, t := range totals {
		ledgerMap[t.WalletID] = t.Total
	}

	// fan-out: parallel mismatch checks using goroutine pool
	const workerCount = 4
	walletCh := make(chan walletBalance)
	resultCh := make(chan string)

	var workerWg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			for wl := range walletCh {
				ledgerSum := ledgerMap[wl.ID]
				if wl.Balance != ledgerSum {
					resultCh <- fmt.Sprintf(
						"MISMATCH wallet=%s cached=%s ledger=%s",
						wl.ID, wl.Balance, ledgerSum,
					)
				}
			}
		}()
	}

	go func() {
		for _, wl := range wallets {
			walletCh <- wl
		}
		close(walletCh)
	}()

	go func() {
		workerWg.Wait()
		close(resultCh)
	}()

	var mismatches int
	for msg := range resultCh {
		mismatches++
		slog.Warn("reconcile-worker mismatch", "detail", msg)
	}

	if mismatches > 0 {
		slog.Warn("reconcile-worker summary", "mismatches", mismatches, "total", len(wallets))
	}
}
