package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type MockProvider struct {
	payURLBase string
}

func NewMockProvider(options map[string]string) *MockProvider {
	base := options["pay_url_base"]
	if base == "" {
		base = "https://pay233.local/mock/pay"
	}
	return &MockProvider{payURLBase: base}
}

func (p *MockProvider) CreatePayment(_ context.Context, req ProviderCreateRequest) (ProviderCreateResponse, error) {
	trade := "mock_" + randomHex(8)
	return ProviderCreateResponse{
		ProviderTrade: trade,
		PayURL:        fmt.Sprintf("%s/%s", p.payURLBase, trade),
		Status:        StatusPending,
	}, nil
}

func (p *MockProvider) ClosePayment(_ context.Context, _ Payment) error {
	return nil
}

func (p *MockProvider) ParseWebhook(_ context.Context, raw []byte, _ map[string]string) (WebhookEvent, error) {
	var event struct {
		ProviderTrade string        `json:"provider_trade"`
		OutTradeNo    string        `json:"out_trade_no"`
		Status        PaymentStatus `json:"status"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return WebhookEvent{}, err
	}
	return WebhookEvent{
		ProviderTrade: event.ProviderTrade,
		OutTradeNo:    event.OutTradeNo,
		Status:        event.Status,
		Raw:           raw,
	}, nil
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "fallback"
	}
	return hex.EncodeToString(buf)
}
