package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/neko233-com/pay233-server/internal/config"
	"github.com/neko233-com/pay233-server/internal/payment"
)

func TestCreatePaymentRequiresValidSignature(t *testing.T) {
	handler := testServer().Routes()
	body := []byte(`{"envType":"test","merchant_id":"m1","out_trade_no":"o1","channel":"mock","amount":{"currency":"CNY","amount":100},"subject":"test"}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/payments", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCreatePayment(t *testing.T) {
	handler := testServer().Routes()
	body := []byte(`{"envType":"test","merchant_id":"m1","out_trade_no":"o1","channel":"mock","amount":{"currency":"CNY","amount":100},"subject":"test"}`)

	req := signedRequest(http.MethodPost, "/v1/payments", body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var got payment.Payment
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != payment.StatusPending {
		t.Fatalf("expected pending, got %s", got.Status)
	}
	if got.PayURL == "" {
		t.Fatal("expected pay_url")
	}
}

func TestCreatePaymentDuplicateReturnsConflict(t *testing.T) {
	handler := testServer().Routes()
	body := []byte(`{"envType":"test","merchant_id":"m1","out_trade_no":"dup-1","channel":"mock","amount":{"currency":"CNY","amount":100},"subject":"test"}`)

	firstReq := signedRequest(http.MethodPost, "/v1/payments", body)
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", firstRec.Code, firstRec.Body.String())
	}

	secondReq := signedRequest(http.MethodPost, "/v1/payments", body)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", secondRec.Code, secondRec.Body.String())
	}
}

func TestPaymentLifecycle(t *testing.T) {
	handler := testServer().Routes()
	body := []byte(`{"envType":"test","merchant_id":"m1","out_trade_no":"life-1","channel":"mock","amount":{"currency":"CNY","amount":100},"subject":"test"}`)

	createReq := signedRequest(http.MethodPost, "/v1/payments", body)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	var created payment.Payment
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/payments/"+created.ID, nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", getRec.Code, getRec.Body.String())
	}

	closeReq := signedRequest(http.MethodPost, "/v1/payments/"+created.ID+"/close", []byte(`{}`))
	closeRec := httptest.NewRecorder()
	handler.ServeHTTP(closeRec, closeReq)
	if closeRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", closeRec.Code, closeRec.Body.String())
	}

	var closed payment.Payment
	if err := json.Unmarshal(closeRec.Body.Bytes(), &closed); err != nil {
		t.Fatal(err)
	}
	if closed.Status != payment.StatusClosed {
		t.Fatalf("expected closed, got %s", closed.Status)
	}
}

func TestWebhookMarksPaymentPaid(t *testing.T) {
	handler := testServer().Routes()
	body := []byte(`{"envType":"release","merchant_id":"m1","out_trade_no":"webhook-1","channel":"mock","amount":{"currency":"CNY","amount":100},"subject":"test"}`)

	createReq := signedRequest(http.MethodPost, "/v1/payments", body)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	webhookBody := []byte(`{"envType":"release","provider_trade":"mock_paid","out_trade_no":"webhook-1","status":"paid"}`)
	webhookReq := httptest.NewRequest(http.MethodPost, "/v1/webhooks/mock", bytes.NewReader(webhookBody))
	webhookRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookRec, webhookReq)
	if webhookRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", webhookRec.Code, webhookRec.Body.String())
	}

	var updated payment.Payment
	if err := json.Unmarshal(webhookRec.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status != payment.StatusPaid {
		t.Fatalf("expected paid, got %s", updated.Status)
	}
}

func TestWebhookDeliversMerchantCallback(t *testing.T) {
	var callbackBody []byte
	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if r.Header.Get(headerTimestamp) == "" || r.Header.Get(headerSignature) == "" {
			t.Fatal("missing callback signature")
		}
		callbackBody = body
		w.WriteHeader(http.StatusNoContent)
	}))
	defer callbackServer.Close()

	handler := testServer().Routes()
	body := []byte(`{"envType":"test","merchant_id":"m1","out_trade_no":"callback-1","channel":"mock","amount":{"currency":"CNY","amount":100},"subject":"test","notify_url":"` + callbackServer.URL + `"}`)
	createReq := signedRequest(http.MethodPost, "/v1/payments", body)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	webhookReq := httptest.NewRequest(http.MethodPost, "/v1/webhooks/mock", bytes.NewReader([]byte(`{"envType":"test","provider_trade":"mock_paid","out_trade_no":"callback-1","status":"paid"}`)))
	webhookRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookRec, webhookReq)
	if webhookRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", webhookRec.Code, webhookRec.Body.String())
	}

	var updated payment.Payment
	if err := json.Unmarshal(webhookRec.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.CallbackStatus != payment.CallbackSuccess || updated.CallbackAttempts != 1 {
		t.Fatalf("expected successful callback, got status=%s attempts=%d", updated.CallbackStatus, updated.CallbackAttempts)
	}
	if !bytes.Contains(callbackBody, []byte(`"out_trade_no":"callback-1"`)) {
		t.Fatalf("unexpected callback body: %s", callbackBody)
	}
}

func TestAdminRetryCallback(t *testing.T) {
	attempts := 0
	callbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "try again", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer callbackServer.Close()

	server := testServer()
	handler := server.Routes()
	body := []byte(`{"envType":"test","merchant_id":"m1","out_trade_no":"retry-1","channel":"mock","amount":{"currency":"CNY","amount":100},"subject":"test","notify_url":"` + callbackServer.URL + `"}`)
	createReq := signedRequest(http.MethodPost, "/v1/payments", body)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	var created payment.Payment
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	webhookReq := httptest.NewRequest(http.MethodPost, "/v1/webhooks/mock", bytes.NewReader([]byte(`{"envType":"test","provider_trade":"mock_paid","out_trade_no":"retry-1","status":"paid"}`)))
	webhookRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookRec, webhookReq)
	if webhookRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", webhookRec.Code, webhookRec.Body.String())
	}
	var failed payment.Payment
	if err := json.Unmarshal(webhookRec.Body.Bytes(), &failed); err != nil {
		t.Fatal(err)
	}
	if failed.CallbackStatus != payment.CallbackFailed {
		t.Fatalf("expected failed callback, got %s", failed.CallbackStatus)
	}

	loginReq := httptest.NewRequest(http.MethodPost, "/admin/login", bytes.NewReader([]byte(`{"username":"root","password":"root"}`)))
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d", loginRec.Code)
	}
	retryReq := httptest.NewRequest(http.MethodPost, "/admin/api/payments/"+created.ID+"/retry-callback", bytes.NewReader([]byte(`{}`)))
	for _, cookie := range loginRec.Result().Cookies() {
		retryReq.AddCookie(cookie)
	}
	retryRec := httptest.NewRecorder()
	handler.ServeHTTP(retryRec, retryReq)
	if retryRec.Code != http.StatusOK {
		t.Fatalf("expected retry 200, got %d: %s", retryRec.Code, retryRec.Body.String())
	}
	var retried payment.Payment
	if err := json.Unmarshal(retryRec.Body.Bytes(), &retried); err != nil {
		t.Fatal(err)
	}
	if retried.CallbackStatus != payment.CallbackSuccess || retried.CallbackAttempts != 2 {
		t.Fatalf("expected retry success, got status=%s attempts=%d", retried.CallbackStatus, retried.CallbackAttempts)
	}
}

func TestChannels(t *testing.T) {
	handler := testServer().Routes()
	req := httptest.NewRequest(http.MethodGet, "/v1/channels", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got struct {
		Channels []string `json:"channels"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Channels) != 1 || got.Channels[0] != "mock" {
		t.Fatalf("unexpected channels: %#v", got.Channels)
	}
}

func testServer() *Server {
	return testServerWithChannels([]config.ChannelConfig{{
		Name:     "mock",
		Provider: "mock",
		Enabled:  true,
	}})
}

func testServerWithChannels(channels []config.ChannelConfig) *Server {
	cfg := config.Config{
		API:      config.APIConfig{SigningSecret: "secret"},
		Admin:    config.AdminConfig{Username: "root", Password: "root", SessionSecret: "admin-secret"},
		Monitor:  config.MonitorConfig{ChannelHealthTimeoutSeconds: 1, ChannelHealthIntervalSeconds: 60},
		Channels: channels,
	}
	registry := payment.NewRegistry()
	if err := payment.RegisterConfiguredProviders(registry, cfg.Channels); err != nil {
		panic(err)
	}
	return NewServer(Dependencies{
		Config:   cfg,
		Registry: registry,
		Store:    payment.NewMemoryStore(),
	})
}

func signedRequest(method string, path string, body []byte) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	timestamp := time.Now().UTC().Format(time.RFC3339)
	req.Header.Set(headerTimestamp, timestamp)
	req.Header.Set(headerSignature, signPayload("secret", timestamp, body))
	return req
}
