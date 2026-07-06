package test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

var (
	baseURL   string
	apiKey    string
	client    *http.Client
	testOwner string

	db     *sql.DB
	dbOnce sync.Once
)

func init() {
	baseURL = os.Getenv("E2E_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8030/api"
	}
	apiKey = os.Getenv("E2E_API_KEY")
	if apiKey == "" {
		apiKey = "your-secret-api-key-here"
	}
	client = &http.Client{Timeout: 10 * time.Second}
	testOwner = fmt.Sprintf("e2e-test-%s", uuid.New().String()[:8])
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

func newRequest(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, baseURL+path, bodyReader)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	return req
}

func doRequest(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()
	req := newRequest(t, method, path, body)
	resp, err := client.Do(req)
	require.NoError(t, err)
	bodyBytes, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	return resp
}

func decodeBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var m map[string]any
	err := json.NewDecoder(resp.Body).Decode(&m)
	require.NoError(t, err)
	return m
}

func getBody(t *testing.T, resp *http.Response) (map[string]any, string) {
	t.Helper()
	m := decodeBody(t, resp)
	status, _ := m["status"].(string)
	return m, status
}

// ---------------------------------------------------------------------------
// single wallet create
// ---------------------------------------------------------------------------

func createWallet(t *testing.T, ownerID, currency string) string {
	t.Helper()
	resp := doRequest(t, http.MethodPost, "/wallets", map[string]string{
		"owner_id": ownerID,
		"currency": currency,
	})
	require.Equal(t, 201, resp.StatusCode, "failed to create wallet for %s/%s", ownerID, currency)
	return decodeBody(t, resp)["data"].(map[string]any)["wallet_id"].(string)
}

// ---------------------------------------------------------------------------
// seeder — setup test data via API
// ---------------------------------------------------------------------------

type SeededWallets struct {
	OwnerID string
	Wallets map[string]string // key: "user1-usd", "user1-eur", etc.
	idemSeq int
	t       *testing.T
}

// NewSeeder creates wallets and seeds initial topups. Returns SeededWallets
// with wallet IDs accessible by label. Registers cleanup via t.Cleanup.
//
// Usage:
//
//	seed := NewSeeder(t, "alice", "usd", "eur")
//	seed.TopUp("usd", "500.00")  // label = currency
//	seed.TopUp("eur", "300.00")
//	fromID := seed.Get("usd")
func NewSeeder(t *testing.T, ownerID string, currencies ...string) *SeededWallets {
	t.Helper()
	uid := ownerID + "-" + uuid.New().String()[:6]
	s := &SeededWallets{
		OwnerID: uid,
		Wallets: make(map[string]string),
		idemSeq: 0,
		t:       t,
	}
	for _, cur := range currencies {
		s.Wallets[cur] = createWallet(t, uid, cur)
	}
	t.Cleanup(func() {
		s.teardown()
	})
	return s
}

// Get returns wallet ID for the given label.
func (s *SeededWallets) Get(label string) string {
	id, ok := s.Wallets[label]
	if !ok {
		s.t.Fatalf("seeded wallet %q not found (available: %v)", label, s.labels())
	}
	return id
}

// TopUp tops up the wallet with the given label.
func (s *SeededWallets) TopUp(label, amount string) (balance, entryID string) {
	s.t.Helper()
	s.idemSeq++
	resp := doRequest(s.t, http.MethodPost, "/wallets/"+s.Get(label)+"/topup", map[string]string{
		"amount":          amount,
		"idempotency_key": fmt.Sprintf("%s-seed-%s-%s-%d", testOwner, s.OwnerID, label, s.idemSeq),
	})
	require.Equal(s.t, 200, resp.StatusCode)
	d := decodeBody(s.t, resp)["data"].(map[string]any)
	return d["balance"].(string), d["entry_id"].(string)
}

// Pay deducts from the wallet with the given label.
func (s *SeededWallets) Pay(label, amount string) (balance, entryID string) {
	s.t.Helper()
	s.idemSeq++
	resp := doRequest(s.t, http.MethodPost, "/wallets/"+s.Get(label)+"/pay", map[string]string{
		"amount":          amount,
		"idempotency_key": fmt.Sprintf("%s-seed-%s-%s-%d", testOwner, s.OwnerID, label, s.idemSeq),
	})
	require.Equal(s.t, 200, resp.StatusCode)
	d := decodeBody(s.t, resp)["data"].(map[string]any)
	return d["balance"].(string), d["entry_id"].(string)
}

// Transfer transfers between two labels within the same seeder.
func (s *SeededWallets) Transfer(fromLabel, toLabel, amount string) (int, map[string]any) {
	return s.transferToWalletID(s.Get(fromLabel), s.Get(toLabel), amount)
}

// TransferTo transfers from this seeder's wallet to another seeder's wallet.
// Returns the decoded response data (may not be status 200).
func (s *SeededWallets) TransferTo(fromLabel string, to *SeededWallets, toLabel, amount string) (int, map[string]any) {
	return s.transferToWalletID(s.Get(fromLabel), to.Get(toLabel), amount)
}

func (s *SeededWallets) transferToWalletID(fromID, toID, amount string) (int, map[string]any) {
	s.t.Helper()
	resp := doRequest(s.t, http.MethodPost, "/wallets/transfer", map[string]string{
		"from_wallet_id":  fromID,
		"to_wallet_id":    toID,
		"amount":          amount,
		"idempotency_key": fmt.Sprintf("%s-seed-transfer-%s-%s-%d", testOwner, fromID[:8], toID[:8], time.Now().UnixNano()),
	})
	body := decodeBody(s.t, resp)
	data, _ := body["data"].(map[string]any)
	return resp.StatusCode, data
}

// Balance returns the balance of a seeded wallet.
func (s *SeededWallets) Balance(label string) map[string]any {
	s.t.Helper()
	resp := doRequest(s.t, http.MethodGet, "/wallets/"+s.Get(label), nil)
	require.Equal(s.t, 200, resp.StatusCode)
	return decodeBody(s.t, resp)["data"].(map[string]any)
}

func (s *SeededWallets) labels() []string {
	ls := make([]string, 0, len(s.Wallets))
	for k := range s.Wallets {
		ls = append(ls, k)
	}
	return ls
}

// teardown deletes all test data from DB. Called automatically via t.Cleanup.
func (s *SeededWallets) teardown() {
	ensureDB()
	for _, walletID := range s.Wallets {
		_, _ = db.Exec(`DELETE FROM ledger_entries WHERE wallet_id = $1`, walletID)
		_, _ = db.Exec(`DELETE FROM wallets WHERE id = $1`, walletID)
	}
}

// ---------------------------------------------------------------------------
// DB connection (lazy, for cleanup only)
// ---------------------------------------------------------------------------

func ensureDB() {
	dbOnce.Do(func() {
		host := envOr("DB_HOST", "postgres-faisalaffan")
		port := envOr("DB_PORT", "5432")
		user := envOr("DB_USER", "postgres")
		pass := envOr("DB_PASSWORD", "postgres_super_secret_2026")
		name := envOr("DB_NAME", "ewallet")

		dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", host, port, user, pass, name)
		var err error
		db, err = sql.Open("postgres", dsn)
		if err != nil {
			return
		}
	})
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
