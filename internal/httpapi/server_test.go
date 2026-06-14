package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/neko233-com/pay233-server/internal/config"
	"github.com/neko233-com/pay233-server/internal/payment"
)

func TestCreatePaymentRequiresValidSignature(t *testing.T) {
	handler := testServer().Routes()
	body := []byte(`{"merchant_id":"m1","out_trade_no":"o1","channel":"mock","amount":{"currency":"CNY","amount":100},"subject":"test"}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/payments", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestCreatePayment(t *testing.T) {
	handler := testServer().Routes()
	body := []byte(`{"merchant_id":"m1","out_trade_no":"o1","channel":"mock","amount":{"currency":"CNY","amount":100},"subject":"test"}`)

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

func TestPaymentLifecycle(t *testing.T) {
	handler := testServer().Routes()
	body := []byte(`{"merchant_id":"m1","out_trade_no":"life-1","channel":"mock","amount":{"currency":"CNY","amount":100},"subject":"test"}`)

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
	body := []byte(`{"merchant_id":"m1","out_trade_no":"webhook-1","channel":"mock","amount":{"currency":"CNY","amount":100},"subject":"test"}`)

	createReq := signedRequest(http.MethodPost, "/v1/payments", body)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	webhookBody := []byte(`{"provider_trade":"mock_paid","out_trade_no":"webhook-1","status":"paid"}`)
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
	cfg := config.Config{
		API: config.APIConfig{SigningSecret: "secret"},
		Channels: []config.ChannelConfig{{
			Name:     "mock",
			Provider: "mock",
			Enabled:  true,
		}},
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
