package payment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfiguredProviderCheckHealthFromOption(t *testing.T) {
	provider := NewConfiguredProvider("mock", map[string]string{"health_status": "down"})
	info := provider.CheckHealth(context.Background(), "")
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
	info := provider.CheckHealth(context.Background(), "")
	if info.Health != "down" {
		t.Fatalf("expected down, got %s", info.Health)
	}
	if info.LastError == "" {
		t.Fatal("expected health error")
	}
}

func TestConfiguredProviderUsesEnvironmentOptions(t *testing.T) {
	provider := NewConfiguredProviderWithEnvironments("mock", nil, map[string]string{
		"pay_url_base": "https://default.pay.local",
	}, map[EnvType]ProviderEnvConfig{
		EnvTypeTest: {
			Options: map[string]string{"pay_url_base": "https://test.pay.local"},
		},
		EnvTypeRelease: {
			Options: map[string]string{"pay_url_base": "https://release.pay.local", "health_status": "degraded"},
		},
	})

	testResp, err := provider.CreatePayment(context.Background(), ProviderCreateRequest{Payment: Payment{EnvType: EnvTypeTest}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(testResp.PayURL, "https://test.pay.local/") {
		t.Fatalf("expected test pay url, got %s", testResp.PayURL)
	}

	releaseResp, err := provider.CreatePayment(context.Background(), ProviderCreateRequest{Payment: Payment{EnvType: EnvTypeRelease}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(releaseResp.PayURL, "https://release.pay.local/") {
		t.Fatalf("expected release pay url, got %s", releaseResp.PayURL)
	}

	envs := provider.HealthEnvironments()
	if len(envs) != 2 || envs[0] != EnvTypeTest || envs[1] != EnvTypeRelease {
		t.Fatalf("unexpected health envs: %#v", envs)
	}
	info := provider.CheckHealth(context.Background(), EnvTypeRelease)
	if info.EnvType != EnvTypeRelease || info.Health != "degraded" {
		t.Fatalf("expected release degraded health, got env=%s health=%s", info.EnvType, info.Health)
	}
}
