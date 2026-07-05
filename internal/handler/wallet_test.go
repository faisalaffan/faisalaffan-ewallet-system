package handler_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/faisalaffan/ewallet-system/internal/domain"
	"github.com/faisalaffan/ewallet-system/internal/handler"
	"github.com/faisalaffan/ewallet-system/internal/router"
	"github.com/faisalaffan/ewallet-system/internal/service"
	"github.com/faisalaffan/ewallet-system/pkg/response"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock types
// ---------------------------------------------------------------------------

type mockWalletService struct {
	createFn    func(domain.CreateWalletRequest) (*domain.Wallet, error)
	getByIDFn   func(uuid.UUID) (*domain.Wallet, error)
	topUpFn     func(uuid.UUID, domain.TopUpRequest) (*domain.TopUpResponse, error)
	payFn       func(uuid.UUID, domain.PayRequest) (*domain.PayResponse, error)
	transferFn  func(domain.TransferRequest) (*domain.TransferResponse, error)
	suspendFn   func(uuid.UUID) (*domain.SuspendResponse, error)
}

func (m *mockWalletService) Create(req domain.CreateWalletRequest) (*domain.Wallet, error) {
	return m.createFn(req)
}
func (m *mockWalletService) GetByID(id uuid.UUID) (*domain.Wallet, error) {
	return m.getByIDFn(id)
}
func (m *mockWalletService) TopUp(id uuid.UUID, req domain.TopUpRequest) (*domain.TopUpResponse, error) {
	return m.topUpFn(id, req)
}
func (m *mockWalletService) Pay(id uuid.UUID, req domain.PayRequest) (*domain.PayResponse, error) {
	return m.payFn(id, req)
}
func (m *mockWalletService) Transfer(req domain.TransferRequest) (*domain.TransferResponse, error) {
	return m.transferFn(req)
}
func (m *mockWalletService) Suspend(id uuid.UUID) (*domain.SuspendResponse, error) {
	return m.suspendFn(id)
}

type mockReconcileService struct {
	reconcileFn func(uuid.UUID) (*domain.ReconcileResponse, error)
}

func (m *mockReconcileService) Reconcile(id uuid.UUID) (*domain.ReconcileResponse, error) {
	return m.reconcileFn(id)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestHandler creates a handler wired to fresh mocks. Each mock function
// panics by default so any unexpected call fails the test loudly.
func newTestHandler() (*handler.WalletHandler, *mockWalletService, *mockReconcileService) {
	mws := &mockWalletService{
		createFn: func(domain.CreateWalletRequest) (*domain.Wallet, error) {
			panic("unexpected call to Create")
		},
		getByIDFn: func(uuid.UUID) (*domain.Wallet, error) {
			panic("unexpected call to GetByID")
		},
		topUpFn: func(uuid.UUID, domain.TopUpRequest) (*domain.TopUpResponse, error) {
			panic("unexpected call to TopUp")
		},
		payFn: func(uuid.UUID, domain.PayRequest) (*domain.PayResponse, error) {
			panic("unexpected call to Pay")
		},
		transferFn: func(domain.TransferRequest) (*domain.TransferResponse, error) {
			panic("unexpected call to Transfer")
		},
		suspendFn: func(uuid.UUID) (*domain.SuspendResponse, error) {
			panic("unexpected call to Suspend")
		},
	}
	mrs := &mockReconcileService{
		reconcileFn: func(uuid.UUID) (*domain.ReconcileResponse, error) {
			panic("unexpected call to Reconcile")
		},
	}
	h := handler.NewWalletHandler(mws, mrs)
	return h, mws, mrs
}

type testHarness struct {
	h   *handler.WalletHandler
	mws *mockWalletService
	mrs *mockReconcileService
}

func newHarness() testHarness {
	h, mws, mrs := newTestHandler()
	return testHarness{h: h, mws: mws, mrs: mrs}
}

func doRequest(app *fiber.App, method, path, body string) (*http.Response, error) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
}

func decodeBody(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&m)
	require.NoError(t, err)
	return m
}

// err returns a non-nil error from the service package for use in mocks.
// (Compile-time assertion that the sentinel is actually an error.)
var (
	errNotFound            = service.ErrNotFound
	errAlreadyExists       = service.ErrAlreadyExists
	errWalletSuspended     = service.ErrWalletSuspended
	errInsufficientBalance = service.ErrInsufficientBalance
	errCurrencyMismatch    = service.ErrCurrencyMismatch
	errSameWallet          = service.ErrSameWallet
	errAmountTooSmall      = service.ErrAmountTooSmall
	errInvalidAmount       = service.ErrInvalidAmount
)

// well-known UUIDs for testing.
var (
	walletUUID = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	entryUUID  = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	transferID = "33333333-3333-3333-3333-333333333333"
)

func defaultWallet() *domain.Wallet {
	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	return &domain.Wallet{
		ID:        walletUUID,
		OwnerID:   "user-001",
		Currency:  "USD",
		Balance:   "500.00",
		Status:    domain.WalletStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// ---------- Create ----------

func TestWalletHandler_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mws.createFn = func(req domain.CreateWalletRequest) (*domain.Wallet, error) {
			assert.Equal(t, "user-001", req.OwnerID)
			assert.Equal(t, "USD", req.Currency)
			w := defaultWallet()
			w.ID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
			w.Balance = "0.00"
			w.Status = domain.WalletStatusActive
			return w, nil
		}

		app := fiber.New()
		app.Post("/api/wallets", th.h.Create)

		resp, err := doRequest(app, http.MethodPost, "/api/wallets", `{"owner_id":"user-001","currency":"USD"}`)
		require.NoError(t, err)
		assert.Equal(t, 201, resp.StatusCode)
		body := decodeBody(t, resp)
		assert.Equal(t, "success", body["status"])
		data := body["data"].(map[string]interface{})
		assert.Equal(t, "00000000-0000-0000-0000-000000000001", data["wallet_id"])
		assert.Equal(t, "0.00", data["balance"])
		assert.Equal(t, "ACTIVE", data["status"])
	})

	t.Run("validation error empty owner_id", func(t *testing.T) {
		t.Parallel()
		th := newHarness()

		app := fiber.New()
		app.Post("/api/wallets", th.h.Create)

		resp, err := doRequest(app, http.MethodPost, "/api/wallets", `{"owner_id":"","currency":"USD"}`)
		require.NoError(t, err)
		assert.Equal(t, 422, resp.StatusCode)
		body := decodeBody(t, resp)
		assert.Equal(t, "fail", body["status"])
	})

	t.Run("conflict", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mws.createFn = func(req domain.CreateWalletRequest) (*domain.Wallet, error) {
			return nil, errAlreadyExists
		}

		app := fiber.New()
		app.Post("/api/wallets", th.h.Create)

		resp, err := doRequest(app, http.MethodPost, "/api/wallets", `{"owner_id":"user-001","currency":"USD"}`)
		require.NoError(t, err)
		assert.Equal(t, 409, resp.StatusCode)
		body := decodeBody(t, resp)
		assert.Equal(t, "error", body["status"])
	})

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()
		th := newHarness()

		app := fiber.New()
		app.Post("/api/wallets", th.h.Create)

		resp, err := doRequest(app, http.MethodPost, "/api/wallets", `{bad json`)
		require.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)
		body := decodeBody(t, resp)
		assert.Equal(t, "error", body["status"])
	})
}

// ---------- Get ----------

func TestWalletHandler_Get(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mws.getByIDFn = func(id uuid.UUID) (*domain.Wallet, error) {
			assert.Equal(t, walletUUID, id)
			return defaultWallet(), nil
		}

		app := fiber.New()
		app.Get("/api/wallets/:id", th.h.Get)

		resp, err := doRequest(app, http.MethodGet, "/api/wallets/11111111-1111-1111-1111-111111111111", "")
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		body := decodeBody(t, resp)
		assert.Equal(t, "success", body["status"])
	})

	t.Run("invalid uuid", func(t *testing.T) {
		t.Parallel()
		th := newHarness()

		app := fiber.New()
		app.Get("/api/wallets/:id", th.h.Get)

		resp, err := doRequest(app, http.MethodGet, "/api/wallets/not-a-uuid", "")
		require.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)
		body := decodeBody(t, resp)
		assert.Equal(t, "error", body["status"])
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mws.getByIDFn = func(id uuid.UUID) (*domain.Wallet, error) {
			return nil, errNotFound
		}

		app := fiber.New()
		app.Get("/api/wallets/:id", th.h.Get)

		resp, err := doRequest(app, http.MethodGet, "/api/wallets/11111111-1111-1111-1111-111111111111", "")
		require.NoError(t, err)
		assert.Equal(t, 404, resp.StatusCode)
		body := decodeBody(t, resp)
		require.Contains(t, body, "error")
		errObj := body["error"].(map[string]interface{})
		assert.Equal(t, "NOT_FOUND", errObj["code"])
	})
}

// ---------- TopUp ----------

func TestWalletHandler_TopUp(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mws.topUpFn = func(id uuid.UUID, req domain.TopUpRequest) (*domain.TopUpResponse, error) {
			assert.Equal(t, walletUUID, id)
			return &domain.TopUpResponse{
				WalletID: walletUUID.String(),
				Balance:  "600.00",
				EntryID:  entryUUID.String(),
			}, nil
		}

		app := fiber.New()
		app.Post("/api/wallets/:id/topup", th.h.TopUp)

		resp, err := doRequest(app, http.MethodPost,
			"/api/wallets/11111111-1111-1111-1111-111111111111/topup",
			`{"amount":"100.00","idempotency_key":"idem-001"}`)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		body := decodeBody(t, resp)
		assert.Equal(t, "success", body["status"])
		data := body["data"].(map[string]interface{})
		assert.Equal(t, walletUUID.String(), data["wallet_id"])
		assert.Equal(t, "600.00", data["balance"])
		assert.Equal(t, entryUUID.String(), data["entry_id"])
	})

	t.Run("missing idempotency key", func(t *testing.T) {
		t.Parallel()
		th := newHarness()

		app := fiber.New()
		app.Post("/api/wallets/:id/topup", th.h.TopUp)

		resp, err := doRequest(app, http.MethodPost,
			"/api/wallets/11111111-1111-1111-1111-111111111111/topup",
			`{"amount":"100.00"}`)
		require.NoError(t, err)
		assert.Equal(t, 422, resp.StatusCode)
	})

	t.Run("wallet not found", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mws.topUpFn = func(id uuid.UUID, req domain.TopUpRequest) (*domain.TopUpResponse, error) {
			return nil, errNotFound
		}

		app := fiber.New()
		app.Post("/api/wallets/:id/topup", th.h.TopUp)

		resp, err := doRequest(app, http.MethodPost,
			"/api/wallets/11111111-1111-1111-1111-111111111111/topup",
			`{"amount":"100.00","idempotency_key":"idem-002"}`)
		require.NoError(t, err)
		assert.Equal(t, 404, resp.StatusCode)
	})

	t.Run("wallet suspended", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mws.topUpFn = func(id uuid.UUID, req domain.TopUpRequest) (*domain.TopUpResponse, error) {
			return nil, errWalletSuspended
		}

		app := fiber.New()
		app.Post("/api/wallets/:id/topup", th.h.TopUp)

		resp, err := doRequest(app, http.MethodPost,
			"/api/wallets/11111111-1111-1111-1111-111111111111/topup",
			`{"amount":"100.00","idempotency_key":"idem-003"}`)
		require.NoError(t, err)
		assert.Equal(t, 409, resp.StatusCode)
	})

	t.Run("amount too small", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mws.topUpFn = func(id uuid.UUID, req domain.TopUpRequest) (*domain.TopUpResponse, error) {
			return nil, errAmountTooSmall
		}

		app := fiber.New()
		app.Post("/api/wallets/:id/topup", th.h.TopUp)

		resp, err := doRequest(app, http.MethodPost,
			"/api/wallets/11111111-1111-1111-1111-111111111111/topup",
			`{"amount":"0.001","idempotency_key":"idem-004"}`)
		require.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)
	})

	t.Run("invalid wallet id", func(t *testing.T) {
		t.Parallel()
		th := newHarness()

		app := fiber.New()
		app.Post("/api/wallets/:id/topup", th.h.TopUp)

		resp, err := doRequest(app, http.MethodPost,
			"/api/wallets/not-a-uuid/topup",
			`{"amount":"100.00","idempotency_key":"idem-005"}`)
		require.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)
	})
}

// ---------- Pay ----------

func TestWalletHandler_Pay(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mws.payFn = func(id uuid.UUID, req domain.PayRequest) (*domain.PayResponse, error) {
			assert.Equal(t, walletUUID, id)
			return &domain.PayResponse{
				WalletID: walletUUID.String(),
				Balance:  "400.00",
				EntryID:  entryUUID.String(),
			}, nil
		}

		app := fiber.New()
		app.Post("/api/wallets/:id/pay", th.h.Pay)

		resp, err := doRequest(app, http.MethodPost,
			"/api/wallets/11111111-1111-1111-1111-111111111111/pay",
			`{"amount":"100.00","idempotency_key":"idem-pay-001"}`)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		body := decodeBody(t, resp)
		assert.Equal(t, "success", body["status"])
	})

	t.Run("missing idempotency key", func(t *testing.T) {
		t.Parallel()
		th := newHarness()

		app := fiber.New()
		app.Post("/api/wallets/:id/pay", th.h.Pay)

		resp, err := doRequest(app, http.MethodPost,
			"/api/wallets/11111111-1111-1111-1111-111111111111/pay",
			`{"amount":"100.00"}`)
		require.NoError(t, err)
		assert.Equal(t, 422, resp.StatusCode)
	})

	t.Run("insufficient balance", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mws.payFn = func(id uuid.UUID, req domain.PayRequest) (*domain.PayResponse, error) {
			return nil, errInsufficientBalance
		}

		app := fiber.New()
		app.Post("/api/wallets/:id/pay", th.h.Pay)

		resp, err := doRequest(app, http.MethodPost,
			"/api/wallets/11111111-1111-1111-1111-111111111111/pay",
			`{"amount":"99999.00","idempotency_key":"idem-pay-002"}`)
		require.NoError(t, err)
		assert.Equal(t, 422, resp.StatusCode)
		body := decodeBody(t, resp)
		assert.Equal(t, "error", body["status"])
	})

	t.Run("wallet suspended", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mws.payFn = func(id uuid.UUID, req domain.PayRequest) (*domain.PayResponse, error) {
			return nil, errWalletSuspended
		}

		app := fiber.New()
		app.Post("/api/wallets/:id/pay", th.h.Pay)

		resp, err := doRequest(app, http.MethodPost,
			"/api/wallets/11111111-1111-1111-1111-111111111111/pay",
			`{"amount":"100.00","idempotency_key":"idem-pay-003"}`)
		require.NoError(t, err)
		assert.Equal(t, 409, resp.StatusCode)
	})

	t.Run("wallet not found", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mws.payFn = func(id uuid.UUID, req domain.PayRequest) (*domain.PayResponse, error) {
			return nil, errNotFound
		}

		app := fiber.New()
		app.Post("/api/wallets/:id/pay", th.h.Pay)

		resp, err := doRequest(app, http.MethodPost,
			"/api/wallets/11111111-1111-1111-1111-111111111111/pay",
			`{"amount":"100.00","idempotency_key":"idem-pay-004"}`)
		require.NoError(t, err)
		assert.Equal(t, 404, resp.StatusCode)
	})

	t.Run("invalid wallet id", func(t *testing.T) {
		t.Parallel()
		th := newHarness()

		app := fiber.New()
		app.Post("/api/wallets/:id/pay", th.h.Pay)

		resp, err := doRequest(app, http.MethodPost,
			"/api/wallets/not-a-uuid/pay",
			`{"amount":"100.00","idempotency_key":"idem-pay-005"}`)
		require.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)
		body := decodeBody(t, resp)
		errObj := body["error"].(map[string]interface{})
		assert.Equal(t, "INVALID_ID", errObj["code"])
	})
}

// ---------- Transfer ----------

func TestWalletHandler_Transfer(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mws.transferFn = func(req domain.TransferRequest) (*domain.TransferResponse, error) {
			return &domain.TransferResponse{
				TransferID:  transferID,
				FromBalance: "400.00",
				ToBalance:   "600.00",
			}, nil
		}

		app := fiber.New()
		app.Post("/api/wallets/transfer", th.h.Transfer)

		resp, err := doRequest(app, http.MethodPost, "/api/wallets/transfer",
			`{"from_wallet_id":"11111111-1111-1111-1111-111111111111","to_wallet_id":"22222222-2222-2222-2222-222222222222","amount":"100.00","idempotency_key":"idem-tr-001"}`)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		body := decodeBody(t, resp)
		assert.Equal(t, "success", body["status"])
		data := body["data"].(map[string]interface{})
		assert.Equal(t, transferID, data["transfer_id"])
		assert.Equal(t, "400.00", data["from_balance"])
		assert.Equal(t, "600.00", data["to_balance"])
	})

	t.Run("currency mismatch", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mws.transferFn = func(req domain.TransferRequest) (*domain.TransferResponse, error) {
			return nil, errCurrencyMismatch
		}

		app := fiber.New()
		app.Post("/api/wallets/transfer", th.h.Transfer)

		resp, err := doRequest(app, http.MethodPost, "/api/wallets/transfer",
			`{"from_wallet_id":"11111111-1111-1111-1111-111111111111","to_wallet_id":"22222222-2222-2222-2222-222222222222","amount":"100.00","idempotency_key":"idem-tr-002"}`)
		require.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)
		body := decodeBody(t, resp)
		errObj := body["error"].(map[string]interface{})
		assert.Equal(t, "CURRENCY_MISMATCH", errObj["code"])
	})

	t.Run("same wallet", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mws.transferFn = func(req domain.TransferRequest) (*domain.TransferResponse, error) {
			return nil, errSameWallet
		}

		app := fiber.New()
		app.Post("/api/wallets/transfer", th.h.Transfer)

		resp, err := doRequest(app, http.MethodPost, "/api/wallets/transfer",
			`{"from_wallet_id":"11111111-1111-1111-1111-111111111111","to_wallet_id":"11111111-1111-1111-1111-111111111111","amount":"100.00","idempotency_key":"idem-tr-003"}`)
		require.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)
		body := decodeBody(t, resp)
		errObj := body["error"].(map[string]interface{})
		assert.Equal(t, "SAME_WALLET", errObj["code"])
	})

	t.Run("insufficient balance", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mws.transferFn = func(req domain.TransferRequest) (*domain.TransferResponse, error) {
			return nil, errInsufficientBalance
		}

		app := fiber.New()
		app.Post("/api/wallets/transfer", th.h.Transfer)

		resp, err := doRequest(app, http.MethodPost, "/api/wallets/transfer",
			`{"from_wallet_id":"11111111-1111-1111-1111-111111111111","to_wallet_id":"22222222-2222-2222-2222-222222222222","amount":"99999.00","idempotency_key":"idem-tr-004"}`)
		require.NoError(t, err)
		assert.Equal(t, 422, resp.StatusCode)
	})

	t.Run("wallet suspended", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mws.transferFn = func(req domain.TransferRequest) (*domain.TransferResponse, error) {
			return nil, errWalletSuspended
		}

		app := fiber.New()
		app.Post("/api/wallets/transfer", th.h.Transfer)

		resp, err := doRequest(app, http.MethodPost, "/api/wallets/transfer",
			`{"from_wallet_id":"11111111-1111-1111-1111-111111111111","to_wallet_id":"22222222-2222-2222-2222-222222222222","amount":"100.00","idempotency_key":"idem-tr-005"}`)
		require.NoError(t, err)
		assert.Equal(t, 409, resp.StatusCode)
	})

	t.Run("wallet not found", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mws.transferFn = func(req domain.TransferRequest) (*domain.TransferResponse, error) {
			return nil, errNotFound
		}

		app := fiber.New()
		app.Post("/api/wallets/transfer", th.h.Transfer)

		resp, err := doRequest(app, http.MethodPost, "/api/wallets/transfer",
			`{"from_wallet_id":"11111111-1111-1111-1111-111111111111","to_wallet_id":"22222222-2222-2222-2222-222222222222","amount":"100.00","idempotency_key":"idem-tr-006"}`)
		require.NoError(t, err)
		assert.Equal(t, 404, resp.StatusCode)
	})

	t.Run("missing idempotency key", func(t *testing.T) {
		t.Parallel()
		th := newHarness()

		app := fiber.New()
		app.Post("/api/wallets/transfer", th.h.Transfer)

		resp, err := doRequest(app, http.MethodPost, "/api/wallets/transfer",
			`{"from_wallet_id":"11111111-1111-1111-1111-111111111111","to_wallet_id":"22222222-2222-2222-2222-222222222222","amount":"100.00"}`)
		require.NoError(t, err)
		assert.Equal(t, 422, resp.StatusCode)
	})
}

// ---------- Suspend ----------

func TestWalletHandler_Suspend(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mws.suspendFn = func(id uuid.UUID) (*domain.SuspendResponse, error) {
			return &domain.SuspendResponse{
				WalletID: walletUUID.String(),
				Status:   domain.WalletStatusSuspended,
			}, nil
		}

		app := fiber.New()
		app.Post("/api/wallets/:id/suspend", th.h.Suspend)

		resp, err := doRequest(app, http.MethodPost,
			"/api/wallets/11111111-1111-1111-1111-111111111111/suspend", "")
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		body := decodeBody(t, resp)
		assert.Equal(t, "success", body["status"])
		data := body["data"].(map[string]interface{})
		assert.Equal(t, walletUUID.String(), data["wallet_id"])
		assert.Equal(t, "SUSPENDED", data["status"])
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mws.suspendFn = func(id uuid.UUID) (*domain.SuspendResponse, error) {
			return nil, errNotFound
		}

		app := fiber.New()
		app.Post("/api/wallets/:id/suspend", th.h.Suspend)

		resp, err := doRequest(app, http.MethodPost,
			"/api/wallets/11111111-1111-1111-1111-111111111111/suspend", "")
		require.NoError(t, err)
		assert.Equal(t, 404, resp.StatusCode)
	})

	t.Run("invalid uuid", func(t *testing.T) {
		t.Parallel()
		th := newHarness()

		app := fiber.New()
		app.Post("/api/wallets/:id/suspend", th.h.Suspend)

		resp, err := doRequest(app, http.MethodPost,
			"/api/wallets/not-a-uuid/suspend", "")
		require.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)
	})
}

// ---------- Reconcile ----------

func TestWalletHandler_Reconcile(t *testing.T) {
	t.Parallel()

	t.Run("success match", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mrs.reconcileFn = func(id uuid.UUID) (*domain.ReconcileResponse, error) {
			return &domain.ReconcileResponse{
				WalletID:      walletUUID.String(),
				CachedBalance: "500.00",
				LedgerSum:     "500.00",
				Match:         true,
				Diff:          "0.00",
			}, nil
		}

		app := fiber.New()
		app.Get("/api/wallets/:id/reconcile", th.h.Reconcile)

		resp, err := doRequest(app, http.MethodGet,
			"/api/wallets/11111111-1111-1111-1111-111111111111/reconcile", "")
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		body := decodeBody(t, resp)
		assert.Equal(t, "success", body["status"])
		data := body["data"].(map[string]interface{})
		assert.Equal(t, true, data["match"])
		assert.Equal(t, "0.00", data["diff"])
		assert.Equal(t, walletUUID.String(), data["wallet_id"])
		assert.Equal(t, "500.00", data["cached_balance"])
		assert.Equal(t, "500.00", data["ledger_sum"])
	})

	t.Run("success mismatch", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mrs.reconcileFn = func(id uuid.UUID) (*domain.ReconcileResponse, error) {
			return &domain.ReconcileResponse{
				WalletID:      walletUUID.String(),
				CachedBalance: "500.00",
				LedgerSum:     "450.00",
				Match:         false,
				Diff:          "50.00",
			}, nil
		}

		app := fiber.New()
		app.Get("/api/wallets/:id/reconcile", th.h.Reconcile)

		resp, err := doRequest(app, http.MethodGet,
			"/api/wallets/11111111-1111-1111-1111-111111111111/reconcile", "")
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		body := decodeBody(t, resp)
		data := body["data"].(map[string]interface{})
		assert.Equal(t, false, data["match"])
		assert.Equal(t, "50.00", data["diff"])
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		th := newHarness()
		th.mrs.reconcileFn = func(id uuid.UUID) (*domain.ReconcileResponse, error) {
			return nil, errNotFound
		}

		app := fiber.New()
		app.Get("/api/wallets/:id/reconcile", th.h.Reconcile)

		resp, err := doRequest(app, http.MethodGet,
			"/api/wallets/11111111-1111-1111-1111-111111111111/reconcile", "")
		require.NoError(t, err)
		assert.Equal(t, 404, resp.StatusCode)
	})

	t.Run("invalid uuid", func(t *testing.T) {
		t.Parallel()
		th := newHarness()

		app := fiber.New()
		app.Get("/api/wallets/:id/reconcile", th.h.Reconcile)

		resp, err := doRequest(app, http.MethodGet,
			"/api/wallets/not-a-uuid/reconcile", "")
		require.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)
	})
}

// ---------- Router Setup ----------

func TestWalletHandler_RouterSetup(t *testing.T) {
	th := newHarness()
	app := router.Setup(th.h)
	assert.NotNil(t, app)

	// Quick smoke: GET /api/wallets/:id when wallet doesn't exist should 404.
	th.mws.getByIDFn = func(id uuid.UUID) (*domain.Wallet, error) {
		return nil, errNotFound
	}
	resp, err := doRequest(app, http.MethodGet,
		"/api/wallets/11111111-1111-1111-1111-111111111111", "")
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

// ---------- Edge: check standard error mapping exhaustively ----------

func TestWalletHandler_ErrorMapping(t *testing.T) {
	// Verify that every service sentinel maps to the correct HTTP status.
	tests := []struct {
		name       string
		svcErr     error
		wantStatus int
		wantCode   string
	}{
		{name: "not found", svcErr: errNotFound, wantStatus: 404, wantCode: "NOT_FOUND"},
		{name: "wallet suspended", svcErr: errWalletSuspended, wantStatus: 409, wantCode: "WALLET_SUSPENDED"},
		{name: "insufficient balance", svcErr: errInsufficientBalance, wantStatus: 422, wantCode: "INSUFFICIENT_BALANCE"},
		{name: "currency mismatch", svcErr: errCurrencyMismatch, wantStatus: 400, wantCode: "CURRENCY_MISMATCH"},
		{name: "same wallet", svcErr: errSameWallet, wantStatus: 400, wantCode: "SAME_WALLET"},
		{name: "invalid amount", svcErr: errInvalidAmount, wantStatus: 400, wantCode: "INVALID_AMOUNT"},
		{name: "amount too small", svcErr: errAmountTooSmall, wantStatus: 400, wantCode: "AMOUNT_TOO_SMALL"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			th := newHarness()
			th.mws.topUpFn = func(id uuid.UUID, req domain.TopUpRequest) (*domain.TopUpResponse, error) {
				return nil, tt.svcErr
			}

			app := fiber.New()
			app.Post("/api/wallets/:id/topup", th.h.TopUp)

			resp, err := doRequest(app, http.MethodPost,
				"/api/wallets/11111111-1111-1111-1111-111111111111/topup",
				`{"amount":"100.00","idempotency_key":"idem-err-001"}`)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, resp.StatusCode)

			body := decodeBody(t, resp)
			errObj := body["error"].(map[string]interface{})
			assert.Equal(t, tt.wantCode, errObj["code"])
		})
	}
}

// ---------- Response envelope structure ----------

func TestWalletHandler_ResponseEnvelope(t *testing.T) {
	t.Parallel()
	th := newHarness()
	th.mws.createFn = func(req domain.CreateWalletRequest) (*domain.Wallet, error) {
		return defaultWallet(), nil
	}

	app := fiber.New()
	app.Post("/api/wallets", th.h.Create)

	resp, err := doRequest(app, http.MethodPost, "/api/wallets",
		`{"owner_id":"user-001","currency":"USD"}`)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	// Decode into the real response type to verify the full envelope structure.
	var envelope response.Envelope
	err = json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)
	assert.Equal(t, "success", envelope.Status)
	assert.NotNil(t, envelope.Data)
	assert.Nil(t, envelope.Error_)
	assert.Nil(t, envelope.Pagination)
}

// ---------------------------------------------------------------------------
// Edge case tests for uncovered branches
// ---------------------------------------------------------------------------

func TestWalletHandler_Create_InvalidCurrency(t *testing.T) {
	t.Parallel()
	th := newHarness()
	th.mws.createFn = func(req domain.CreateWalletRequest) (*domain.Wallet, error) {
		return nil, service.ErrInvalidCurrency
	}

	app := fiber.New()
	app.Post("/api/wallets", th.h.Create)

	resp, err := doRequest(app, http.MethodPost, "/api/wallets", `{"owner_id":"user-001","currency":"INVALID"}`)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
	body := decodeBody(t, resp)
	errObj := body["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_CURRENCY", errObj["code"])
}

func TestWalletHandler_Create_GenericError(t *testing.T) {
	t.Parallel()
	genericErr := errors.New("database unreachable")
	th := newHarness()
	th.mws.createFn = func(req domain.CreateWalletRequest) (*domain.Wallet, error) {
		return nil, genericErr
	}

	app := fiber.New()
	app.Post("/api/wallets", th.h.Create)

	resp, err := doRequest(app, http.MethodPost, "/api/wallets", `{"owner_id":"user-001","currency":"USD"}`)
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)
	body := decodeBody(t, resp)
	errObj := body["error"].(map[string]interface{})
	assert.Equal(t, "INTERNAL_ERROR", errObj["code"])
}

func TestWalletHandler_Get_GenericError(t *testing.T) {
	t.Parallel()
	genericErr := errors.New("database unreachable")
	th := newHarness()
	th.mws.getByIDFn = func(id uuid.UUID) (*domain.Wallet, error) {
		return nil, genericErr
	}

	app := fiber.New()
	app.Get("/api/wallets/:id", th.h.Get)

	resp, err := doRequest(app, http.MethodGet, "/api/wallets/11111111-1111-1111-1111-111111111111", "")
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)
	body := decodeBody(t, resp)
	errObj := body["error"].(map[string]interface{})
	assert.Equal(t, "INTERNAL_ERROR", errObj["code"])
}

func TestWalletHandler_TopUp_InvalidJSON(t *testing.T) {
	t.Parallel()
	th := newHarness()

	app := fiber.New()
	app.Post("/api/wallets/:id/topup", th.h.TopUp)

	resp, err := doRequest(app, http.MethodPost,
		"/api/wallets/11111111-1111-1111-1111-111111111111/topup",
		`{bad json`)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
	body := decodeBody(t, resp)
	errObj := body["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_JSON", errObj["code"])
}

func TestWalletHandler_Pay_InvalidJSON(t *testing.T) {
	t.Parallel()

	bodies := []string{`{bad json`, `[`, `not-json`, ``}
	for _, body := range bodies {
		body := body
		t.Run("body="+body, func(t *testing.T) {
			th := newHarness()
			app := fiber.New()
			app.Post("/api/wallets/:id/pay", th.h.Pay)

			resp, err := doRequest(app, http.MethodPost,
				"/api/wallets/11111111-1111-1111-1111-111111111111/pay",
				body)
			require.NoError(t, err)
			assert.Equal(t, 400, resp.StatusCode)
		})
	}
}

func TestWalletHandler_Transfer_InvalidJSON(t *testing.T) {
	t.Parallel()
	th := newHarness()

	app := fiber.New()
	app.Post("/api/wallets/transfer", th.h.Transfer)

	resp, err := doRequest(app, http.MethodPost, "/api/wallets/transfer", `{bad json`)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
	body := decodeBody(t, resp)
	errObj := body["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_JSON", errObj["code"])
}

func TestWalletHandler_Suspend_GenericError(t *testing.T) {
	t.Parallel()
	genericErr := errors.New("database unreachable")
	th := newHarness()
	th.mws.suspendFn = func(id uuid.UUID) (*domain.SuspendResponse, error) {
		return nil, genericErr
	}

	app := fiber.New()
	app.Post("/api/wallets/:id/suspend", th.h.Suspend)

	resp, err := doRequest(app, http.MethodPost,
		"/api/wallets/11111111-1111-1111-1111-111111111111/suspend", "")
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)
	body := decodeBody(t, resp)
	errObj := body["error"].(map[string]interface{})
	assert.Equal(t, "INTERNAL_ERROR", errObj["code"])
}

func TestWalletHandler_Reconcile_GenericError(t *testing.T) {
	t.Parallel()
	genericErr := errors.New("database unreachable")
	th := newHarness()
	th.mrs.reconcileFn = func(id uuid.UUID) (*domain.ReconcileResponse, error) {
		return nil, genericErr
	}

	app := fiber.New()
	app.Get("/api/wallets/:id/reconcile", th.h.Reconcile)

	resp, err := doRequest(app, http.MethodGet,
		"/api/wallets/11111111-1111-1111-1111-111111111111/reconcile", "")
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)
	body := decodeBody(t, resp)
	errObj := body["error"].(map[string]interface{})
	assert.Equal(t, "INTERNAL_ERROR", errObj["code"])
}

func TestWalletHandler_TopUp_GenericError(t *testing.T) {
	t.Parallel()
	genericErr := errors.New("unexpected error")
	th := newHarness()
	th.mws.topUpFn = func(id uuid.UUID, req domain.TopUpRequest) (*domain.TopUpResponse, error) {
		return nil, genericErr
	}

	app := fiber.New()
	app.Post("/api/wallets/:id/topup", th.h.TopUp)

	resp, err := doRequest(app, http.MethodPost,
		"/api/wallets/11111111-1111-1111-1111-111111111111/topup",
		`{"amount":"100.00","idempotency_key":"idem-gen-topup"}`)
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)
	// Also verify that mapWalletError's default branch is hit.
	body := decodeBody(t, resp)
	errObj := body["error"].(map[string]interface{})
	assert.Equal(t, "INTERNAL_ERROR", errObj["code"])
}

// Ensure unused variables are referenced (helps if the build complains).
var _ = errors.New
