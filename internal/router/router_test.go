package router_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/faisalaffan/ewallet-system/internal/domain"
	"github.com/faisalaffan/ewallet-system/internal/handler"
	"github.com/faisalaffan/ewallet-system/internal/router"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Minimal mock implementations (same pattern as handler_test)
// ---------------------------------------------------------------------------

type mockWalletSvc struct {
	createFn   func(ctx context.Context, req domain.CreateWalletRequest) (*domain.Wallet, error)
	getByIDFn  func(ctx context.Context, id uuid.UUID) (*domain.Wallet, error)
	topUpFn    func(ctx context.Context, id uuid.UUID, req domain.TopUpRequest) (*domain.TopUpResponse, error)
	payFn      func(ctx context.Context, id uuid.UUID, req domain.PayRequest) (*domain.PayResponse, error)
	transferFn func(ctx context.Context, req domain.TransferRequest) (*domain.TransferResponse, error)
	suspendFn  func(ctx context.Context, id uuid.UUID) (*domain.SuspendResponse, error)
}

func (m *mockWalletSvc) Create(ctx context.Context, req domain.CreateWalletRequest) (*domain.Wallet, error) {
	return m.createFn(ctx, req)
}
func (m *mockWalletSvc) GetByID(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
	return m.getByIDFn(ctx, id)
}
func (m *mockWalletSvc) TopUp(ctx context.Context, id uuid.UUID, req domain.TopUpRequest) (*domain.TopUpResponse, error) {
	return m.topUpFn(ctx, id, req)
}
func (m *mockWalletSvc) Pay(ctx context.Context, id uuid.UUID, req domain.PayRequest) (*domain.PayResponse, error) {
	return m.payFn(ctx, id, req)
}
func (m *mockWalletSvc) Transfer(ctx context.Context, req domain.TransferRequest) (*domain.TransferResponse, error) {
	return m.transferFn(ctx, req)
}
func (m *mockWalletSvc) Suspend(ctx context.Context, id uuid.UUID) (*domain.SuspendResponse, error) {
	return m.suspendFn(ctx, id)
}

type mockReconcileSvc struct {
	reconcileFn func(ctx context.Context, id uuid.UUID) (*domain.ReconcileResponse, error)
}

func (m *mockReconcileSvc) Reconcile(ctx context.Context, id uuid.UUID) (*domain.ReconcileResponse, error) {
	return m.reconcileFn(ctx, id)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

var walletUUID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

func newMockHandler() *handler.WalletHandler {
	genericErr := errors.New("mock error")
	return handler.NewWalletHandler(
		&mockWalletSvc{
			createFn: func(ctx context.Context, req domain.CreateWalletRequest) (*domain.Wallet, error) {
				return nil, genericErr
			},
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
				return nil, genericErr
			},
			topUpFn: func(ctx context.Context, id uuid.UUID, req domain.TopUpRequest) (*domain.TopUpResponse, error) {
				return nil, genericErr
			},
			payFn: func(ctx context.Context, id uuid.UUID, req domain.PayRequest) (*domain.PayResponse, error) {
				return nil, genericErr
			},
			transferFn: func(ctx context.Context, req domain.TransferRequest) (*domain.TransferResponse, error) {
				return nil, genericErr
			},
			suspendFn: func(ctx context.Context, id uuid.UUID) (*domain.SuspendResponse, error) {
				return nil, genericErr
			},
		},
		&mockReconcileSvc{
			reconcileFn: func(ctx context.Context, id uuid.UUID) (*domain.ReconcileResponse, error) {
				return nil, genericErr
			},
		},
	)
}

func doRequest(app *fiber.App, method, path, body string) (*http.Response, error) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestSetup_ReturnsApp(t *testing.T) {
	h := newMockHandler()
	app := router.Setup(h)
	assert.NotNil(t, app)
	assert.IsType(t, &fiber.App{}, app)
}

func TestSetup_RoutesRegistered(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "POST /api/wallets",
			method: http.MethodPost,
			path:   "/api/wallets",
			body:   `{"owner_id":"user-1","currency":"USD"}`,
		},
		{
			name:   "GET /api/wallets/:id",
			method: http.MethodGet,
			path:   "/api/wallets/11111111-1111-1111-1111-111111111111",
			body:   "",
		},
		{
			name:   "POST /api/wallets/:id/topup",
			method: http.MethodPost,
			path:   "/api/wallets/11111111-1111-1111-1111-111111111111/topup",
			body:   `{"amount":"100.00","idempotency_key":"idem-001"}`,
		},
		{
			name:   "POST /api/wallets/:id/pay",
			method: http.MethodPost,
			path:   "/api/wallets/11111111-1111-1111-1111-111111111111/pay",
			body:   `{"amount":"50.00","idempotency_key":"idem-002"}`,
		},
		{
			name:   "POST /api/wallets/transfer",
			method: http.MethodPost,
			path:   "/api/wallets/transfer",
			body:   `{"from_wallet_id":"11111111-1111-1111-1111-111111111111","to_wallet_id":"22222222-2222-2222-2222-222222222222","amount":"100.00","idempotency_key":"idem-003"}`,
		},
		{
			name:   "POST /api/wallets/:id/suspend",
			method: http.MethodPost,
			path:   "/api/wallets/11111111-1111-1111-1111-111111111111/suspend",
			body:   "",
		},
		{
			name:   "GET /api/wallets/:id/reconcile",
			method: http.MethodGet,
			path:   "/api/wallets/11111111-1111-1111-1111-111111111111/reconcile",
			body:   "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh app for each subtest to avoid shared-state issues
			// with Fiber's internal router in parallel tests.
			h := newMockHandler()
			app := router.Setup(h)

			resp, err := doRequest(app, tt.method, tt.path, tt.body)
			require.NoError(t, err)
			// All mocks return ErrNotFound, so every registered route should
			// respond with some status code (not 404 due to route not found).
			assert.NotEqual(t, 404, resp.StatusCode,
				"route should be registered but returned 404 (possible route mismatch)")
		})
	}
}

func TestSetup_SwaggerRoutes(t *testing.T) {
	h := newMockHandler()
	app := router.Setup(h)

	t.Run("GET /swagger returns HTML", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/swagger", nil)
		resp, err := app.Test(req, fiber.TestConfig{Timeout: 2 * time.Second})
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")
	})

	t.Run("GET /swagger/doc.json returns JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/swagger/doc.json", nil)
		resp, err := app.Test(req, fiber.TestConfig{Timeout: 2 * time.Second})
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})
}

// Ensure unused variables are referenced (helps if the build complains).
var _ = errors.New
