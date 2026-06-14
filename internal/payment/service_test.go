package payment

import (
	"context"
	"testing"
)

func TestServiceCreatePayment(t *testing.T) {
	service := testService()

	created, err := service.Create(context.Background(), CreatePaymentRequest{
		MerchantID: "m1",
		OutTradeNo: "o1",
		Channel:    "mock",
		Amount:     Money{Currency: "CNY", Amount: 100},
		Subject:    "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatal("expected id")
	}
	if created.Status != StatusPending {
		t.Fatalf("expected pending, got %s", created.Status)
	}
	if created.ProviderTrade == "" {
		t.Fatal("expected provider trade")
	}
}

func TestServiceRejectsUnknownChannel(t *testing.T) {
	service := testService()

	_, err := service.Create(context.Background(), CreatePaymentRequest{
		MerchantID: "m1",
		OutTradeNo: "o1",
		Channel:    "missing",
		Amount:     Money{Currency: "CNY", Amount: 100},
		Subject:    "test",
	})
	if err != ErrChannelNotConfigured {
		t.Fatalf("expected ErrChannelNotConfigured, got %v", err)
	}
}

func TestServiceAppliesWebhook(t *testing.T) {
	service := testService()
	created, err := service.Create(context.Background(), CreatePaymentRequest{
		MerchantID: "m1",
		OutTradeNo: "o1",
		Channel:    "mock",
		Amount:     Money{Currency: "CNY", Amount: 100},
		Subject:    "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := service.ApplyWebhook(context.Background(), "mock", []byte(`{"provider_trade":"mock_done","out_trade_no":"o1","status":"paid"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID {
		t.Fatalf("expected %s, got %s", created.ID, updated.ID)
	}
	if updated.Status != StatusPaid {
		t.Fatalf("expected paid, got %s", updated.Status)
	}
}

func TestServiceClosePayment(t *testing.T) {
	service := testService()
	created, err := service.Create(context.Background(), CreatePaymentRequest{
		MerchantID: "m1",
		OutTradeNo: "o1",
		Channel:    "mock",
		Amount:     Money{Currency: "CNY", Amount: 100},
		Subject:    "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	closed, err := service.Close(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != StatusClosed {
		t.Fatalf("expected closed, got %s", closed.Status)
	}
}

func testService() *Service {
	registry := NewRegistry()
	registry.Register("mock", NewMockProvider(nil))
	return NewService(registry, NewMemoryStore())
}
