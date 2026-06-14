package payment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConfiguredProviderCheckHealthFromOption(t *testing.T) {
	provider := NewConfiguredProvider("mock", map[string]string{"health_status": "down"})
	info := provider.CheckHealth(context.Background())
	if info.Health != "down" {
		t.Fatalf("expected down, got %s", info.Health)
	}
	if info.LastCheckedAt == nil {
		t.Fatal("expected last checked time")
	}
}

func TestConfiguredProviderCheckHealthURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	provider := NewConfiguredProvider("mock", map[string]string{"health_url": server.URL})
	info := provider.CheckHealth(context.Background())
	if info.Health != "down" {
		t.Fatalf("expected down, got %s", info.Health)
	}
	if info.LastError == "" {
		t.Fatal("expected health error")
	}
}
