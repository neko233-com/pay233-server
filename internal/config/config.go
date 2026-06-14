package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	HTTP     HTTPConfig      `json:"http"`
	API      APIConfig       `json:"api"`
	Admin    AdminConfig     `json:"admin"`
	Logging  LoggingConfig   `json:"logging"`
	Storage  StorageConfig   `json:"storage"`
	Channels []ChannelConfig `json:"channels"`
}

type HTTPConfig struct {
	Addr string `json:"addr"`
}

type APIConfig struct {
	SigningSecret string `json:"signing_secret"`
}

type AdminConfig struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	SessionSecret string `json:"session_secret"`
}

type LoggingConfig struct {
	Dir           string `json:"dir"`
	RetentionDays int    `json:"retention_days"`
}

type StorageConfig struct {
	PaymentsPath       string `json:"payments_path"`
	AdminUsersPath     string `json:"admin_users_path"`
	AuditPath          string `json:"audit_path"`
	AuditRetentionDays int    `json:"audit_retention_days"`
}

type ChannelConfig struct {
	Name        string            `json:"name"`
	Provider    string            `json:"provider"`
	Enabled     bool              `json:"enabled"`
	Credentials map[string]string `json:"credentials,omitempty"`
	Options     map[string]string `json:"options,omitempty"`
}

func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})

	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}

	if cfg.HTTP.Addr == "" {
		cfg.HTTP.Addr = ":5500"
	}
	if cfg.Admin.Username == "" {
		cfg.Admin.Username = "root"
	}
	if cfg.Admin.Password == "" {
		cfg.Admin.Password = "root"
	}
	if cfg.Admin.SessionSecret == "" {
		cfg.Admin.SessionSecret = cfg.API.SigningSecret
	}
	if cfg.Logging.Dir == "" {
		cfg.Logging.Dir = "logs"
	}
	if cfg.Logging.RetentionDays <= 0 {
		cfg.Logging.RetentionDays = 31
	}
	if cfg.Storage.PaymentsPath == "" {
		cfg.Storage.PaymentsPath = "data/payments.jsonl"
	}
	if cfg.Storage.AdminUsersPath == "" {
		cfg.Storage.AdminUsersPath = "data/admin-users.json"
	}
	if cfg.Storage.AuditPath == "" {
		cfg.Storage.AuditPath = "data/audit.jsonl"
	}
	if cfg.Storage.AuditRetentionDays <= 0 {
		cfg.Storage.AuditRetentionDays = 31
	}
	for i, channel := range cfg.Channels {
		if channel.Name == "" {
			return Config{}, fmt.Errorf("channels[%d].name is required", i)
		}
		if channel.Provider == "" {
			return Config{}, fmt.Errorf("channels[%d].provider is required", i)
		}
	}

	return cfg, nil
}
