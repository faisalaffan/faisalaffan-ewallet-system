package handler

import (
	"errors"

	"github.com/faisalaffan/ewallet-system/internal/domain"
	"github.com/faisalaffan/ewallet-system/internal/service"
	"github.com/faisalaffan/ewallet-system/pkg/response"
	"github.com/faisalaffan/ewallet-system/pkg/telemetry"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// @title Multi-Currency E-Wallet API
// @version 1.0
// @description Ledger-based e-wallet backend with multi-currency support.
// @BasePath /api

type WalletHandler struct {
	walletSvc   service.WalletServiceInterface
	reconcileSvc service.ReconcileServiceInterface
}

func NewWalletHandler(walletSvc service.WalletServiceInterface, reconcileSvc service.ReconcileServiceInterface) *WalletHandler {
	return &WalletHandler{walletSvc: walletSvc, reconcileSvc: reconcileSvc}
}

// Create godoc
// @Summary Create a new wallet
// @Description Create wallet for an owner with a specific currency
// @Tags wallets
// @Accept json
// @Produce json
// @Param body body domain.CreateWalletRequest true "Create wallet request"
// @Success 201 {object} response.Envelope{data=domain.Wallet}
// @Failure 400 {object} response.Envelope
// @Failure 409 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /wallets [post]
func (h *WalletHandler) Create(c fiber.Ctx) error {
	c, span := telemetry.SetupTracerHandler(c)
	var err error
	defer func() { telemetry.EndTracerHandler(c, span, err) }()

	var req domain.CreateWalletRequest
	if err = c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(response.Error(400, "INVALID_JSON", "invalid request body"))
	}
	if req.OwnerID == "" || req.Currency == "" {
		return c.Status(422).JSON(response.ValidationError("owner_id and currency are required", nil))
	}

	w, svcErr := h.walletSvc.Create(req)
	if svcErr != nil {
		err = svcErr
		if errors.Is(err, service.ErrAlreadyExists) {
			return c.Status(409).JSON(response.Error(409, "WALLET_EXISTS", "wallet already exists for this owner and currency"))
		}
		if errors.Is(err, service.ErrInvalidCurrency) {
			return c.Status(400).JSON(response.Error(400, "INVALID_CURRENCY", "currency must be a 3-character ISO 4217 code"))
		}
		return c.Status(500).JSON(response.Error(500, "INTERNAL_ERROR", err.Error()))
	}

	return c.Status(201).JSON(response.SuccessCreated(w))
}

// Get godoc
// @Summary Get wallet by ID
// @Description Retrieve wallet details including balance and status
// @Tags wallets
// @Produce json
// @Param id path string true "Wallet ID (UUID)"
// @Success 200 {object} response.Envelope{data=domain.Wallet}
// @Failure 400 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Router /wallets/{id} [get]
func (h *WalletHandler) Get(c fiber.Ctx) error {
	c, span := telemetry.SetupTracerHandler(c)
	var err error
	defer func() { telemetry.EndTracerHandler(c, span, err) }()

	id, parseErr := uuid.Parse(c.Params("id"))
	if parseErr != nil {
		return c.Status(400).JSON(response.Error(400, "INVALID_ID", "invalid wallet id"))
	}

	w, svcErr := h.walletSvc.GetByID(id)
	if svcErr != nil {
		err = svcErr
		if errors.Is(err, service.ErrNotFound) {
			return c.Status(404).JSON(response.Error(404, "NOT_FOUND", "wallet not found"))
		}
		return c.Status(500).JSON(response.Error(500, "INTERNAL_ERROR", err.Error()))
	}

	return c.Status(200).JSON(response.SuccessOK(w))
}

// TopUp godoc
// @Summary Top-up wallet balance
// @Description Add funds to wallet. Idempotent via idempotency_key.
// @Tags wallets
// @Accept json
// @Produce json
// @Param id path string true "Wallet ID (UUID)"
// @Param body body domain.TopUpRequest true "Top-up request"
// @Success 200 {object} response.Envelope{data=domain.TopUpResponse}
// @Failure 400 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Failure 409 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /wallets/{id}/topup [post]
func (h *WalletHandler) TopUp(c fiber.Ctx) error {
	c, span := telemetry.SetupTracerHandler(c)
	var err error
	defer func() { telemetry.EndTracerHandler(c, span, err) }()

	id, parseErr := uuid.Parse(c.Params("id"))
	if parseErr != nil {
		return c.Status(400).JSON(response.Error(400, "INVALID_ID", "invalid wallet id"))
	}

	var req domain.TopUpRequest
	if err = c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(response.Error(400, "INVALID_JSON", "invalid request body"))
	}
	if req.IdempotencyKey == "" {
		return c.Status(422).JSON(response.ValidationError("idempotency_key is required", nil))
	}

	result, svcErr := h.walletSvc.TopUp(id, req)
	if svcErr != nil {
		err = svcErr
		return mapWalletError(c, err)
	}

	return c.Status(200).JSON(response.SuccessOK(result))
}

// Pay godoc
// @Summary Make a payment
// @Description Deduct funds from wallet. Insufficient balance returns 422.
// @Tags wallets
// @Accept json
// @Produce json
// @Param id path string true "Wallet ID (UUID)"
// @Param body body domain.PayRequest true "Payment request"
// @Success 200 {object} response.Envelope{data=domain.PayResponse}
// @Failure 400 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Failure 409 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /wallets/{id}/pay [post]
func (h *WalletHandler) Pay(c fiber.Ctx) error {
	c, span := telemetry.SetupTracerHandler(c)
	var err error
	defer func() { telemetry.EndTracerHandler(c, span, err) }()

	id, parseErr := uuid.Parse(c.Params("id"))
	if parseErr != nil {
		return c.Status(400).JSON(response.Error(400, "INVALID_ID", "invalid wallet id"))
	}

	var req domain.PayRequest
	if err = c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(response.Error(400, "INVALID_JSON", "invalid request body"))
	}
	if req.IdempotencyKey == "" {
		return c.Status(422).JSON(response.ValidationError("idempotency_key is required", nil))
	}

	result, svcErr := h.walletSvc.Pay(id, req)
	if svcErr != nil {
		err = svcErr
		return mapWalletError(c, err)
	}

	return c.Status(200).JSON(response.SuccessOK(result))
}

// Transfer godoc
// @Summary Transfer between wallets
// @Description Transfer funds from one wallet to another. Both must be same currency.
// @Tags wallets
// @Accept json
// @Produce json
// @Param body body domain.TransferRequest true "Transfer request"
// @Success 200 {object} response.Envelope{data=domain.TransferResponse}
// @Failure 400 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Failure 409 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Router /wallets/transfer [post]
func (h *WalletHandler) Transfer(c fiber.Ctx) error {
	c, span := telemetry.SetupTracerHandler(c)
	var err error
	defer func() { telemetry.EndTracerHandler(c, span, err) }()

	var req domain.TransferRequest
	if err = c.Bind().Body(&req); err != nil {
		return c.Status(400).JSON(response.Error(400, "INVALID_JSON", "invalid request body"))
	}
	if req.IdempotencyKey == "" {
		return c.Status(422).JSON(response.ValidationError("idempotency_key is required", nil))
	}

	result, svcErr := h.walletSvc.Transfer(req)
	if svcErr != nil {
		err = svcErr
		return mapWalletError(c, err)
	}

	return c.Status(200).JSON(response.SuccessOK(result))
}

// Suspend godoc
// @Summary Suspend a wallet
// @Description Suspend wallet. Idempotent — already suspended returns 200.
// @Tags wallets
// @Produce json
// @Param id path string true "Wallet ID (UUID)"
// @Success 200 {object} response.Envelope{data=domain.SuspendResponse}
// @Failure 400 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Router /wallets/{id}/suspend [post]
func (h *WalletHandler) Suspend(c fiber.Ctx) error {
	c, span := telemetry.SetupTracerHandler(c)
	var err error
	defer func() { telemetry.EndTracerHandler(c, span, err) }()

	id, parseErr := uuid.Parse(c.Params("id"))
	if parseErr != nil {
		return c.Status(400).JSON(response.Error(400, "INVALID_ID", "invalid wallet id"))
	}

	result, svcErr := h.walletSvc.Suspend(id)
	if svcErr != nil {
		err = svcErr
		if errors.Is(err, service.ErrNotFound) {
			return c.Status(404).JSON(response.Error(404, "NOT_FOUND", "wallet not found"))
		}
		return c.Status(500).JSON(response.Error(500, "INTERNAL_ERROR", err.Error()))
	}

	return c.Status(200).JSON(response.SuccessOK(result))
}

// Reconcile godoc
// @Summary Reconcile wallet balance
// @Description Compare cached balance against SUM(ledger_entries). Returns match status and diff.
// @Tags wallets
// @Produce json
// @Param id path string true "Wallet ID (UUID)"
// @Success 200 {object} response.Envelope{data=domain.ReconcileResponse}
// @Failure 400 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Router /wallets/{id}/reconcile [get]
func (h *WalletHandler) Reconcile(c fiber.Ctx) error {
	c, span := telemetry.SetupTracerHandler(c)
	var err error
	defer func() { telemetry.EndTracerHandler(c, span, err) }()

	id, parseErr := uuid.Parse(c.Params("id"))
	if parseErr != nil {
		return c.Status(400).JSON(response.Error(400, "INVALID_ID", "invalid wallet id"))
	}

	result, svcErr := h.reconcileSvc.Reconcile(id)
	if svcErr != nil {
		err = svcErr
		if errors.Is(err, service.ErrNotFound) {
			return c.Status(404).JSON(response.Error(404, "NOT_FOUND", "wallet not found"))
		}
		return c.Status(500).JSON(response.Error(500, "INTERNAL_ERROR", err.Error()))
	}

	return c.Status(200).JSON(response.SuccessOK(result))
}

func mapWalletError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrNotFound):
		return c.Status(404).JSON(response.Error(404, "NOT_FOUND", "wallet not found"))
	case errors.Is(err, service.ErrWalletSuspended):
		return c.Status(409).JSON(response.Error(409, "WALLET_SUSPENDED", "wallet is suspended"))
	case errors.Is(err, service.ErrInsufficientBalance):
		return c.Status(422).JSON(response.Error(422, "INSUFFICIENT_BALANCE", "insufficient balance"))
	case errors.Is(err, service.ErrCurrencyMismatch):
		return c.Status(400).JSON(response.Error(400, "CURRENCY_MISMATCH", "currency mismatch between wallets"))
	case errors.Is(err, service.ErrSameWallet):
		return c.Status(400).JSON(response.Error(400, "SAME_WALLET", "cannot transfer to the same wallet"))
	case errors.Is(err, service.ErrInvalidAmount):
		return c.Status(400).JSON(response.Error(400, "INVALID_AMOUNT", "amount must be greater than 0"))
	case errors.Is(err, service.ErrAmountTooSmall):
		return c.Status(400).JSON(response.Error(400, "AMOUNT_TOO_SMALL", "amount must be at least 0.01 after rounding"))
	default:
		return c.Status(500).JSON(response.Error(500, "INTERNAL_ERROR", err.Error()))
	}
}
