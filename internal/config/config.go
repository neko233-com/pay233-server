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
		cfg.HTTP.Addr = ":8080"
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
