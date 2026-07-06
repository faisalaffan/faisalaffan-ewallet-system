package test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNegative_Create_MissingFields(t *testing.T) {
	resp := doRequest(t, http.MethodPost, "/wallets", map[string]string{
		"owner_id": "",
		"currency": "",
	})
	assert.Equal(t, 422, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
}

func TestNegative_Create_InvalidJSON(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/wallets", bytes.NewReader([]byte("{bad")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 400, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "INVALID_JSON", errObj["code"])
}

func TestNegative_Create_Duplicate(t *testing.T) {
	owner := testOwner + "-dup"
	createWallet(t, owner, "USD")

	resp := doRequest(t, http.MethodPost, "/wallets", map[string]string{
		"owner_id": owner,
		"currency": "USD",
	})
	assert.Equal(t, 409, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "WALLET_EXISTS", errObj["code"])
}

func TestNegative_Get_NotFound(t *testing.T) {
	resp := doRequest(t, http.MethodGet, "/wallets/"+uuid.New().String(), nil)
	assert.Equal(t, 404, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestNegative_Get_InvalidUUID(t *testing.T) {
	resp := doRequest(t, http.MethodGet, "/wallets/not-a-uuid", nil)
	assert.Equal(t, 400, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "INVALID_ID", errObj["code"])
}

func TestNegative_TopUp_NoIdempotencyKey(t *testing.T) {
	resp := doRequest(t, http.MethodPost, "/wallets/"+uuid.New().String()+"/topup", map[string]string{
		"amount": "100.00",
	})
	assert.Equal(t, 422, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
}

func TestNegative_Pay_NoIdempotencyKey(t *testing.T) {
	resp := doRequest(t, http.MethodPost, "/wallets/"+uuid.New().String()+"/pay", map[string]string{
		"amount": "100.00",
	})
	assert.Equal(t, 422, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
}

func TestNegative_Pay_InsufficientBalance(t *testing.T) {
	walletID := createWallet(t, testOwner+"-pay-insuff", "USD")
	doRequest(t, http.MethodPost, "/wallets/"+walletID+"/topup", map[string]string{
		"amount":          "50.00",
		"idempotency_key": testOwner + "-pay-insuff-topup",
	})

	resp := doRequest(t, http.MethodPost, "/wallets/"+walletID+"/pay", map[string]string{
		"amount":          "99999.00",
		"idempotency_key": testOwner + "-pay-insuff-1",
	})
	assert.Equal(t, 422, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "INSUFFICIENT_BALANCE", errObj["code"])
}

func TestNegative_Transfer_NoIdempotencyKey(t *testing.T) {
	resp := doRequest(t, http.MethodPost, "/wallets/transfer", map[string]string{
		"from_wallet_id": uuid.New().String(),
		"to_wallet_id":   uuid.New().String(),
		"amount":         "100.00",
	})
	assert.Equal(t, 422, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
}

func TestNegative_Transfer_CurrencyMismatch(t *testing.T) {
	from := createWallet(t, testOwner+"-tr-mismatch-from", "USD")
	to := createWallet(t, testOwner+"-tr-mismatch-to", "IDR")
	doRequest(t, http.MethodPost, "/wallets/"+from+"/topup", map[string]string{
		"amount":          "500.00",
		"idempotency_key": testOwner + "-tr-mismatch-topup",
	})

	resp := doRequest(t, http.MethodPost, "/wallets/transfer", map[string]string{
		"from_wallet_id":  from,
		"to_wallet_id":    to,
		"amount":          "100.00",
		"idempotency_key": testOwner + "-tr-mismatch-1",
	})
	assert.Equal(t, 400, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "CURRENCY_MISMATCH", errObj["code"])
}

func TestNegative_Transfer_SameWallet(t *testing.T) {
	from := createWallet(t, testOwner+"-tr-same", "USD")
	doRequest(t, http.MethodPost, "/wallets/"+from+"/topup", map[string]string{
		"amount":          "500.00",
		"idempotency_key": testOwner + "-tr-same-topup",
	})

	resp := doRequest(t, http.MethodPost, "/wallets/transfer", map[string]string{
		"from_wallet_id":  from,
		"to_wallet_id":    from,
		"amount":          "100.00",
		"idempotency_key": testOwner + "-tr-same-1",
	})
	assert.Equal(t, 400, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "SAME_WALLET", errObj["code"])
}

func TestNegative_Auth_NoHeader(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, baseURL+"/wallets/"+uuid.New().String(), nil)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 401, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "UNAUTHORIZED", errObj["code"])
}

func TestNegative_Auth_InvalidKey(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, baseURL+"/wallets/"+uuid.New().String(), nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 401, resp.StatusCode)
	body, _ := getBody(t, resp)
	errObj := body["error"].(map[string]any)
	assert.Equal(t, "UNAUTHORIZED", errObj["code"])
}
