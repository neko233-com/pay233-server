package payment

import (
	"context"
	"errors"
	"path/filepath"
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

func TestServiceDashboardTracksAbnormalPayments(t *testing.T) {
	service := testService()
	created, err := service.Create(context.Background(), CreatePaymentRequest{
		MerchantID: "m1",
		OutTradeNo: "o1",
		Channel:    "mock",
		Amount:     Money{Currency: "CNY", Amount: 100},
		Subject:    "test",
		NotifyURL:  "https://merchant.test/notify",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.CallbackStatus != CallbackPending {
		t.Fatalf("expected callback pending, got %s", created.CallbackStatus)
	}

	dashboard := service.Dashboard(ListFilter{})
	if dashboard.KPIs.TotalPayments != 1 {
		t.Fatalf("expected one payment, got %d", dashboard.KPIs.TotalPayments)
	}
	if len(dashboard.Abnormal) != 1 {
		t.Fatalf("expected abnormal payment, got %d", len(dashboard.Abnormal))
	}
	if len(dashboard.ChannelInfo) == 0 {
		t.Fatal("expected channel info")
	}
}

func TestServiceSeparatesEnvironments(t *testing.T) {
	service := testService()
	for _, envType := range []EnvType{EnvTypeTest, EnvTypeRelease} {
		_, err := service.Create(context.Background(), CreatePaymentRequest{
			EnvType:    envType,
			MerchantID: "m1",
			OutTradeNo: "same-order",
			Channel:    "mock",
			Amount:     Money{Currency: "CNY", Amount: 100},
			Subject:    "test",
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	testDashboard := service.Dashboard(ListFilter{EnvType: EnvTypeTest})
	if testDashboard.KPIs.TotalPayments != 1 {
		t.Fatalf("expected one test payment, got %d", testDashboard.KPIs.TotalPayments)
	}
	releaseDashboard := service.Dashboard(ListFilter{EnvType: EnvTypeRelease})
	if releaseDashboard.KPIs.TotalPayments != 1 {
		t.Fatalf("expected one release payment, got %d", releaseDashboard.KPIs.TotalPayments)
	}
	allDashboard := service.Dashboard(ListFilter{})
	if allDashboard.KPIs.TotalPayments != 2 {
		t.Fatalf("expected two all payments, got %d", allDashboard.KPIs.TotalPayments)
	}
}

func TestServiceDashboardFiltersChannelInfoByEnvironment(t *testing.T) {
	registry := NewRegistry()
	registry.Register("mock", NewConfiguredProviderWithEnvironments("mock", nil, nil, map[EnvType]ProviderEnvConfig{
		EnvTypeTest:    {Options: map[string]string{"health_status": "ok"}},
		EnvTypeRelease: {Options: map[string]string{"health_status": "down"}},
	}))
	service := NewService(registry, NewMemoryStore())

	allDashboard := service.Dashboard(ListFilter{})
	if len(allDashboard.ChannelInfo) != 2 {
		t.Fatalf("expected all channel info rows, got %#v", allDashboard.ChannelInfo)
	}

	testDashboard := service.Dashboard(ListFilter{EnvType: EnvTypeTest})
	if len(testDashboard.ChannelInfo) != 1 || testDashboard.ChannelInfo[0].EnvType != EnvTypeTest {
		t.Fatalf("expected only test channel info, got %#v", testDashboard.ChannelInfo)
	}

	releaseDashboard := service.Dashboard(ListFilter{EnvType: EnvTypeRelease})
	if len(releaseDashboard.ChannelInfo) != 1 || releaseDashboard.ChannelInfo[0].EnvType != EnvTypeRelease {
		t.Fatalf("expected only release channel info, got %#v", releaseDashboard.ChannelInfo)
	}
}

func TestServiceRejectsDuplicateOrderInSameEnvironment(t *testing.T) {
	service := testService()
	req := CreatePaymentRequest{
		EnvType:    EnvTypeTest,
		MerchantID: "m1",
		OutTradeNo: "same-order",
		Channel:    "mock",
		Amount:     Money{Currency: "CNY", Amount: 100},
		Subject:    "test",
	}
	if _, err := service.Create(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(context.Background(), req); !errors.Is(err, ErrDuplicatePayment) {
		t.Fatalf("expected ErrDuplicatePayment, got %v", err)
	}
}

func TestFileStorePersistsPayments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payments.jsonl")
	store, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	service := testServiceWithStore(store)
	created, err := service.Create(context.Background(), CreatePaymentRequest{
		EnvType:    EnvTypeRelease,
		MerchantID: "m1",
		OutTradeNo: "persist-1",
		Channel:    "mock",
		Amount:     Money{Currency: "CNY", Amount: 100},
		Subject:    "persist",
	})
	if err != nil {
		t.Fatal(err)
	}
	closed, err := service.Close(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, ok := reopened.FindByOutTradeNo(EnvTypeRelease, "mock", "persist-1")
	if !ok {
		t.Fatal("expected persisted payment")
	}
	if got.ID != created.ID || got.Status != closed.Status {
		t.Fatalf("unexpected persisted payment: %#v", got)
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
	return testServiceWithStore(NewMemoryStore())
}

func testServiceWithStore(store Store) *Service {
	registry := NewRegistry()
	registry.Register("mock", NewMockProvider(nil))
	return NewService(registry, store)
}
