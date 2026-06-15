package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAppliesDefaults(t *testing.T) {
	path := writeConfig(t, `{
		"api": {"signing_secret": "secret"},
		"channels": [{"name": "mock", "provider": "mock", "enabled": true}]
	}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Addr != ":5500" {
		t.Fatalf("expected default addr, got %q", cfg.HTTP.Addr)
	}
	if cfg.API.SignatureMaxSkewSeconds != 300 {
		t.Fatalf("expected default signature skew, got %d", cfg.API.SignatureMaxSkewSeconds)
	}
	if cfg.Logging.Dir != "logs" {
		t.Fatalf("expected default log dir, got %q", cfg.Logging.Dir)
	}
	if cfg.Logging.RetentionDays != 31 {
		t.Fatalf("expected default log retention, got %d", cfg.Logging.RetentionDays)
	}
	if cfg.Storage.PaymentsPath != "data/payments.jsonl" {
		t.Fatalf("expected default payment store, got %q", cfg.Storage.PaymentsPath)
	}
	if cfg.Storage.AdminUsersPath != "data/admin-users.json" {
		t.Fatalf("expected default admin user store, got %q", cfg.Storage.AdminUsersPath)
	}
	if cfg.Storage.AuditPath != "data/audit.jsonl" {
		t.Fatalf("expected default audit store, got %q", cfg.Storage.AuditPath)
	}
	if cfg.Storage.AuditRetentionDays != 31 {
		t.Fatalf("expected default audit retention, got %d", cfg.Storage.AuditRetentionDays)
	}
	if cfg.Monitor.ChannelHealthIntervalSeconds != 60 {
		t.Fatalf("expected default health interval, got %d", cfg.Monitor.ChannelHealthIntervalSeconds)
	}
	if cfg.Monitor.ChannelHealthTimeoutSeconds != 5 {
		t.Fatalf("expected default health timeout, got %d", cfg.Monitor.ChannelHealthTimeoutSeconds)
	}
}

func TestLoadValidatesChannel(t *testing.T) {
	path := writeConfig(t, `{"channels": [{"name": "mock"}]}`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLoadValidatesChannelEnvironment(t *testing.T) {
	path := writeConfig(t, `{
		"channels": [{
			"name": "mock",
			"provider": "mock",
			"enabled": true,
			"environments": {"sandbox": {"options": {"health_status": "ok"}}}
		}]
	}`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected environment validation error")
	}
}

func TestLoadAcceptsUTF8BOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	raw := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{
		"channels": [{"name": "mock", "provider": "mock", "enabled": true}]
	}`)...)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(cfg.Channels))
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
