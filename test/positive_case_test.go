package test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPositive_CreateWallet(t *testing.T) {
	resp := doRequest(t, http.MethodPost, "/wallets", map[string]string{
		"owner_id": testOwner,
		"currency": "USD",
	})
	require.Equal(t, 201, resp.StatusCode)
	body, status := getBody(t, resp)
	assert.Equal(t, "success", status)
	data := body["data"].(map[string]any)
	assert.NotEmpty(t, data["wallet_id"])
	assert.Equal(t, "0.00", data["balance"])
	assert.Equal(t, "ACTIVE", data["status"])
}

func TestPositive_GetWallet(t *testing.T) {
	walletID := createWallet(t, testOwner+"-get", "USD")

	resp := doRequest(t, http.MethodGet, "/wallets/"+walletID, nil)
	require.Equal(t, 200, resp.StatusCode)
	body, status := getBody(t, resp)
	assert.Equal(t, "success", status)
	data := body["data"].(map[string]any)
	assert.Equal(t, walletID, data["wallet_id"])
	assert.Equal(t, testOwner+"-get", data["owner_id"])
	assert.Equal(t, "USD", data["currency"])
	assert.Equal(t, "0.00", data["balance"])
	assert.Equal(t, "ACTIVE", data["status"])
}

func TestPositive_TopUp(t *testing.T) {
	walletID := createWallet(t, testOwner+"-topup", "USD")

	t.Run("topup 100", func(t *testing.T) {
		resp := doRequest(t, http.MethodPost, "/wallets/"+walletID+"/topup", map[string]string{
			"amount":          "100.00",
			"idempotency_key": testOwner + "-topup-1",
		})
		require.Equal(t, 200, resp.StatusCode)
		body, status := getBody(t, resp)
		assert.Equal(t, "success", status)
		data := body["data"].(map[string]any)
		assert.Equal(t, walletID, data["wallet_id"])
		assert.Equal(t, "100.00", data["balance"])
		assert.NotEmpty(t, data["entry_id"])
	})

	t.Run("topup 50 more", func(t *testing.T) {
		resp := doRequest(t, http.MethodPost, "/wallets/"+walletID+"/topup", map[string]string{
			"amount":          "50.00",
			"idempotency_key": testOwner + "-topup-2",
		})
		require.Equal(t, 200, resp.StatusCode)
		data := decodeBody(t, resp)["data"].(map[string]any)
		assert.Equal(t, "150.00", data["balance"])
	})

	t.Run("topup idempotent", func(t *testing.T) {
		resp := doRequest(t, http.MethodPost, "/wallets/"+walletID+"/topup", map[string]string{
			"amount":          "100.00",
			"idempotency_key": testOwner + "-topup-1",
		})
		require.Equal(t, 200, resp.StatusCode)
		data := decodeBody(t, resp)["data"].(map[string]any)
		assert.Equal(t, "150.00", data["balance"])
	})
}

func TestPositive_Pay(t *testing.T) {
	walletID := createWallet(t, testOwner+"-pay", "USD")
	doRequest(t, http.MethodPost, "/wallets/"+walletID+"/topup", map[string]string{
		"amount":          "200.00",
		"idempotency_key": testOwner + "-pay-topup",
	})

	resp := doRequest(t, http.MethodPost, "/wallets/"+walletID+"/pay", map[string]string{
		"amount":          "50.00",
		"idempotency_key": testOwner + "-pay-1",
	})
	require.Equal(t, 200, resp.StatusCode)
	data := decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, "150.00", data["balance"])
}

func TestPositive_Transfer(t *testing.T) {
	from := createWallet(t, testOwner+"-transfer-from", "USD")
	to := createWallet(t, testOwner+"-transfer-to", "USD")
	doRequest(t, http.MethodPost, "/wallets/"+from+"/topup", map[string]string{
		"amount":          "500.00",
		"idempotency_key": testOwner + "-transfer-topup",
	})

	resp := doRequest(t, http.MethodPost, "/wallets/transfer", map[string]string{
		"from_wallet_id":  from,
		"to_wallet_id":    to,
		"amount":          "100.00",
		"idempotency_key": testOwner + "-transfer-1",
	})
	require.Equal(t, 200, resp.StatusCode)
	data := decodeBody(t, resp)["data"].(map[string]any)
	assert.NotEmpty(t, data["transfer_id"])
	assert.Equal(t, "400.00", data["from_balance"])
	assert.Equal(t, "100.00", data["to_balance"])
}

func TestPositive_Transfer_Idempotent(t *testing.T) {
	from := createWallet(t, testOwner+"-tr-idem-from", "USD")
	to := createWallet(t, testOwner+"-tr-idem-to", "USD")
	doRequest(t, http.MethodPost, "/wallets/"+from+"/topup", map[string]string{
		"amount":          "500.00",
		"idempotency_key": testOwner + "-tr-idem-topup",
	})

	doRequest(t, http.MethodPost, "/wallets/transfer", map[string]string{
		"from_wallet_id":  from,
		"to_wallet_id":    to,
		"amount":          "100.00",
		"idempotency_key": testOwner + "-tr-idem-1",
	})

	resp := doRequest(t, http.MethodPost, "/wallets/transfer", map[string]string{
		"from_wallet_id":  from,
		"to_wallet_id":    to,
		"amount":          "100.00",
		"idempotency_key": testOwner + "-tr-idem-1",
	})
	require.Equal(t, 200, resp.StatusCode)
	data := decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, "400.00", data["from_balance"])
	assert.Equal(t, "100.00", data["to_balance"])
}

func TestPositive_Reconcile_Match(t *testing.T) {
	walletID := createWallet(t, testOwner+"-reconcile", "USD")
	doRequest(t, http.MethodPost, "/wallets/"+walletID+"/topup", map[string]string{
		"amount":          "100.00",
		"idempotency_key": testOwner + "-reconcile-topup",
	})
	doRequest(t, http.MethodPost, "/wallets/"+walletID+"/pay", map[string]string{
		"amount":          "30.00",
		"idempotency_key": testOwner + "-reconcile-pay",
	})

	resp := doRequest(t, http.MethodGet, "/wallets/"+walletID+"/reconcile", nil)
	require.Equal(t, 200, resp.StatusCode)
	data := decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, true, data["match"])
	assert.Equal(t, "70.00", data["cached_balance"])
	assert.Equal(t, "70.00", data["ledger_sum"])
	assert.Equal(t, "0.00", data["diff"])
}

func TestPositive_Suspend(t *testing.T) {
	walletID := createWallet(t, testOwner+"-suspend", "USD")

	resp := doRequest(t, http.MethodPost, "/wallets/"+walletID+"/suspend", nil)
	require.Equal(t, 200, resp.StatusCode)
	data := decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, walletID, data["wallet_id"])
	assert.Equal(t, "SUSPENDED", data["status"])
}

func TestPositive_Suspend_Idempotent(t *testing.T) {
	walletID := createWallet(t, testOwner+"-suspend-idem", "USD")
	doRequest(t, http.MethodPost, "/wallets/"+walletID+"/suspend", nil)

	resp := doRequest(t, http.MethodPost, "/wallets/"+walletID+"/suspend", nil)
	require.Equal(t, 200, resp.StatusCode)
	data := decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, "SUSPENDED", data["status"])
}

func TestPositive_Transfer_SameCurrency(t *testing.T) {
	from := createWallet(t, testOwner+"-pos-tr-idr-from", "IDR")
	to := createWallet(t, testOwner+"-pos-tr-idr-to", "IDR")
	doRequest(t, http.MethodPost, "/wallets/"+from+"/topup", map[string]string{
		"amount":          "1000000.00",
		"idempotency_key": testOwner + "-pos-tr-idr-topup",
	})

	resp := doRequest(t, http.MethodPost, "/wallets/transfer", map[string]string{
		"from_wallet_id":  from,
		"to_wallet_id":    to,
		"amount":          "250000.50",
		"idempotency_key": testOwner + "-pos-tr-idr-1",
	})
	require.Equal(t, 200, resp.StatusCode)
	data := decodeBody(t, resp)["data"].(map[string]any)
	assert.NotEmpty(t, data["transfer_id"])
	assert.Equal(t, "749999.50", data["from_balance"])
	assert.Equal(t, "250000.50", data["to_balance"])

	resp = doRequest(t, http.MethodGet, "/wallets/"+from, nil)
	require.Equal(t, 200, resp.StatusCode)
	data = decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, "IDR", data["currency"])
	assert.Equal(t, "749999.50", data["balance"])

	resp = doRequest(t, http.MethodGet, "/wallets/"+to, nil)
	require.Equal(t, 200, resp.StatusCode)
	data = decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, "IDR", data["currency"])
	assert.Equal(t, "250000.50", data["balance"])
}

func TestPositive_BalanceLedgerConsistent(t *testing.T) {
	walletID := createWallet(t, testOwner+"-blc", "USD")

	ops := []struct {
		endpoint string
		amount   string
		key      string
	}{
		{"/wallets/" + walletID + "/topup", "1000.00", testOwner + "-blc-1"},
		{"/wallets/" + walletID + "/pay", "250.75", testOwner + "-blc-2"},
		{"/wallets/" + walletID + "/topup", "500.50", testOwner + "-blc-3"},
		{"/wallets/" + walletID + "/pay", "125.25", testOwner + "-blc-4"},
	}
	for _, op := range ops {
		resp := doRequest(t, http.MethodPost, op.endpoint, map[string]string{
			"amount":          op.amount,
			"idempotency_key": op.key,
		})
		require.Equal(t, 200, resp.StatusCode)
	}

	resp := doRequest(t, http.MethodGet, "/wallets/"+walletID, nil)
	require.Equal(t, 200, resp.StatusCode)
	data := decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, "1124.50", data["balance"])

	resp = doRequest(t, http.MethodGet, "/wallets/"+walletID+"/reconcile", nil)
	require.Equal(t, 200, resp.StatusCode)
	data = decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, true, data["match"])
	assert.Equal(t, "1124.50", data["cached_balance"])
	assert.Equal(t, "1124.50", data["ledger_sum"])
	assert.Equal(t, "0.00", data["diff"])
}

func TestPositive_LedgerEntries_Precision(t *testing.T) {
	walletID := createWallet(t, testOwner+"-ledger-prec", "EUR")

	resp := doRequest(t, http.MethodPost, "/wallets/"+walletID+"/topup", map[string]string{
		"amount":          "99.99",
		"idempotency_key": testOwner + "-ledger-prec-1",
	})
	require.Equal(t, 200, resp.StatusCode)
	topupData := decodeBody(t, resp)["data"].(map[string]any)
	assert.NotEmpty(t, topupData["entry_id"])
	assert.Equal(t, "99.99", topupData["balance"])

	resp = doRequest(t, http.MethodPost, "/wallets/"+walletID+"/pay", map[string]string{
		"amount":          "33.33",
		"idempotency_key": testOwner + "-ledger-prec-2",
	})
	require.Equal(t, 200, resp.StatusCode)
	payData := decodeBody(t, resp)["data"].(map[string]any)
	assert.NotEmpty(t, payData["entry_id"])
	assert.Equal(t, "66.66", payData["balance"])

	resp = doRequest(t, http.MethodGet, "/wallets/"+walletID+"/reconcile", nil)
	require.Equal(t, 200, resp.StatusCode)
	data := decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, true, data["match"])
	assert.Equal(t, "66.66", data["cached_balance"])
	assert.Equal(t, "66.66", data["ledger_sum"])

	resp = doRequest(t, http.MethodGet, "/wallets/"+walletID, nil)
	require.Equal(t, 200, resp.StatusCode)
	walletData := decodeBody(t, resp)["data"].(map[string]any)
	assert.Equal(t, "EUR", walletData["currency"])
	assert.Equal(t, "66.66", walletData["balance"])
}

func TestPositive_FullFlow(t *testing.T) {
	u1 := NewSeeder(t, "user1", "USD", "EUR")
	u2 := NewSeeder(t, "user2", "USD")

	// top-ups
	bal, _ := u1.TopUp("USD", "1000.50")
	assert.Equal(t, "1000.50", bal)
	bal, _ = u1.TopUp("EUR", "500.25")
	assert.Equal(t, "500.25", bal)
	bal, _ = u2.TopUp("USD", "200.75")
	assert.Equal(t, "200.75", bal)

	// payments
	bal, _ = u1.Pay("USD", "200.10")
	assert.Equal(t, "800.40", bal)
	bal, _ = u1.Pay("EUR", "100.50")
	assert.Equal(t, "399.75", bal)

	// transfer USD user1 → USD user2
	resp := doRequest(t, http.MethodPost, "/wallets/transfer", map[string]string{
		"from_wallet_id":  u1.Get("USD"),
		"to_wallet_id":    u2.Get("USD"),
		"amount":          "300.40",
		"idempotency_key": testOwner + "-flow-tr1",
	})
	require.Equal(t, 200, resp.StatusCode)
	tr := decodeBody(t, resp)["data"].(map[string]any)
	assert.NotEmpty(t, tr["transfer_id"])
	assert.Equal(t, "500.00", tr["from_balance"])
	assert.Equal(t, "501.15", tr["to_balance"])

	// transfer cross-currency must fail
	resp = doRequest(t, http.MethodPost, "/wallets/transfer", map[string]string{
		"from_wallet_id":  u1.Get("EUR"),
		"to_wallet_id":    u2.Get("USD"),
		"amount":          "100.00",
		"idempotency_key": testOwner + "-flow-fail",
	})
	assert.Equal(t, 400, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "CURRENCY_MISMATCH", errObj["code"])

	// verify final state
	d := u1.Balance("USD")
	assert.Equal(t, "USD", d["currency"])
	assert.Equal(t, "500.00", d["balance"])

	d = u1.Balance("EUR")
	assert.Equal(t, "EUR", d["currency"])
	assert.Equal(t, "399.75", d["balance"])

	d = u2.Balance("USD")
	assert.Equal(t, "USD", d["currency"])
	assert.Equal(t, "501.15", d["balance"])
}
