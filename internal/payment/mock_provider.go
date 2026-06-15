package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"
)

type ProviderEnvConfig struct {
	Credentials map[string]string
	Options     map[string]string
}

type ConfiguredProvider struct {
	provider      string
	display       string
	family        string
	defaultConfig providerRuntimeConfig
	envs          map[EnvType]providerRuntimeConfig
}

type providerRuntimeConfig struct {
	credentials map[string]string
	options     map[string]string
	payURLBase  string
	healthURL   string
	healthMode  string
}

type MockProvider = ConfiguredProvider

func NewMockProvider(options map[string]string) *MockProvider {
	return NewConfiguredProvider("mock", options)
}

func NewConfiguredProvider(provider string, options map[string]string) *ConfiguredProvider {
	return NewConfiguredProviderWithEnvironments(provider, nil, options, nil)
}

func NewConfiguredProviderWithEnvironments(provider string, credentials map[string]string, options map[string]string, envs map[EnvType]ProviderEnvConfig) *ConfiguredProvider {
	info := providerCatalog(provider)
	runtimeEnvs := make(map[EnvType]providerRuntimeConfig, len(envs))
	for env, envConfig := range envs {
		runtimeEnvs[env] = newProviderRuntimeConfig(
			provider,
			mergeStringMaps(credentials, envConfig.Credentials),
			mergeStringMaps(options, envConfig.Options),
		)
	}
	return &ConfiguredProvider{
		provider:      provider,
		display:       info.DisplayName,
		family:        info.Family,
		defaultConfig: newProviderRuntimeConfig(provider, credentials, options),
		envs:          runtimeEnvs,
	}
}

func (p *ConfiguredProvider) Info() ProviderInfo {
	info := providerCatalog(p.provider)
	return ProviderInfo{
		DisplayName:  info.DisplayName,
		Family:       info.Family,
		Capabilities: info.Capabilities,
		Health:       "ok",
	}
}

func (p *ConfiguredProvider) HealthEnvironments() []EnvType {
	if len(p.envs) == 0 {
		return []EnvType{""}
	}
	envs := make([]EnvType, 0, len(p.envs))
	for _, env := range []EnvType{EnvTypeTest, EnvTypeRelease} {
		if _, ok := p.envs[env]; ok {
			envs = append(envs, env)
		}
	}
	extra := make([]EnvType, 0)
	for env := range p.envs {
		if env != EnvTypeTest && env != EnvTypeRelease {
			extra = append(extra, env)
		}
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i] < extra[j] })
	envs = append(envs, extra...)
	return envs
}

func (p *ConfiguredProvider) CheckHealth(ctx context.Context, env EnvType) ProviderInfo {
	start := time.Now()
	info := p.Info()
	if env != "" {
		info.EnvType = env
	}
	runtime := p.runtimeFor(env)
	now := start.UTC()
	info.LastCheckedAt = &now
	finish := func() ProviderInfo {
		info.LatencyMS = time.Since(start).Milliseconds()
		return info
	}
	if runtime.healthMode != "" {
		switch runtime.healthMode {
		case "ok", "degraded", "down":
			info.Health = runtime.healthMode
		default:
			info.Health = "down"
			info.LastError = "invalid health_status option"
		}
		return finish()
	}
	if runtime.healthURL == "" {
		info.Health = "ok"
		return finish()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, runtime.healthURL, nil)
	if err != nil {
		info.Health = "down"
		info.LastError = err.Error()
		return finish()
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		info.Health = "down"
		info.LastError = err.Error()
		return finish()
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		info.Health = "ok"
		return finish()
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 500 {
		info.Health = "degraded"
	} else {
		info.Health = "down"
	}
	info.LastError = fmt.Sprintf("health endpoint returned %d", resp.StatusCode)
	return finish()
}

func (p *ConfiguredProvider) CreatePayment(_ context.Context, req ProviderCreateRequest) (ProviderCreateResponse, error) {
	trade := p.provider + "_" + randomHex(8)
	runtime := p.runtimeFor(req.Payment.EnvType)
	return ProviderCreateResponse{
		ProviderTrade: trade,
		PayURL:        fmt.Sprintf("%s/%s", runtime.payURLBase, trade),
		Status:        StatusPending,
	}, nil
}

func (p *ConfiguredProvider) ClosePayment(_ context.Context, _ Payment) error {
	return nil
}

func (p *ConfiguredProvider) ParseWebhook(_ context.Context, raw []byte, _ map[string]string) (WebhookEvent, error) {
	var event struct {
		EnvTypeCamel  string        `json:"envType"`
		EnvTypeSnake  string        `json:"env_type"`
		ProviderTrade string        `json:"provider_trade"`
		OutTradeNo    string        `json:"out_trade_no"`
		Status        PaymentStatus `json:"status"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return WebhookEvent{}, err
	}
	env := event.EnvTypeSnake
	if env == "" {
		env = event.EnvTypeCamel
	}
	envType, err := NormalizeEnvType(env)
	if err != nil {
		return WebhookEvent{}, err
	}
	return WebhookEvent{
		EnvType:       envType,
		ProviderTrade: event.ProviderTrade,
		OutTradeNo:    event.OutTradeNo,
		Status:        event.Status,
		Raw:           raw,
	}, nil
}

func providerCatalog(provider string) ProviderInfo {
	switch provider {
	case "wechat_pay":
		return ProviderInfo{DisplayName: "微信支付", Family: "wallet", Capabilities: []string{"native", "jsapi", "mini_program", "refund", "webhook"}}
	case "alipay":
		return ProviderInfo{DisplayName: "支付宝", Family: "wallet", Capabilities: []string{"page_pay", "app_pay", "refund", "webhook"}}
	case "stripe":
		return ProviderInfo{DisplayName: "Stripe", Family: "card", Capabilities: []string{"card", "checkout", "refund", "webhook"}}
	case "paypal":
		return ProviderInfo{DisplayName: "PayPal", Family: "wallet", Capabilities: []string{"checkout", "refund", "webhook"}}
	case "google_pay":
		return ProviderInfo{DisplayName: "Google Pay", Family: "wallet", Capabilities: []string{"tokenized_card", "app_pay", "webhook"}}
	case "apple_iap":
		return ProviderInfo{DisplayName: "Apple iOS Pay", Family: "iap", Capabilities: []string{"receipt_verify", "subscription", "server_notification"}}
	case "apple_pay":
		return ProviderInfo{DisplayName: "Apple Pay", Family: "wallet", Capabilities: []string{"tokenized_card", "app_pay", "webhook"}}
	case "unionpay":
		return ProviderInfo{DisplayName: "银联", Family: "card", Capabilities: []string{"quick_pay", "refund", "webhook"}}
	case "mock":
		return ProviderInfo{DisplayName: "Mock", Family: "test", Capabilities: []string{"create", "close", "webhook"}}
	default:
		return ProviderInfo{DisplayName: provider, Family: "third_party", Capabilities: []string{"create", "close", "webhook"}}
	}
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "fallback"
	}
	return hex.EncodeToString(buf)
}

func (p *ConfiguredProvider) runtimeFor(env EnvType) providerRuntimeConfig {
	if env != "" {
		if runtime, ok := p.envs[env]; ok {
			return runtime
		}
	}
	return p.defaultConfig
}

func newProviderRuntimeConfig(provider string, credentials map[string]string, options map[string]string) providerRuntimeConfig {
	base := options["pay_url_base"]
	if base == "" {
		base = "https://pay233.local/" + provider + "/pay"
	}
	return providerRuntimeConfig{
		credentials: cloneStringMap(credentials),
		options:     cloneStringMap(options),
		payURLBase:  base,
		healthURL:   options["health_url"],
		healthMode:  options["health_status"],
	}
}

func mergeStringMaps(base map[string]string, override map[string]string) map[string]string {
	merged := cloneStringMap(base)
	if len(override) == 0 {
		return merged
	}
	if merged == nil {
		merged = make(map[string]string, len(override))
	}
	for key, value := range override {
		merged[key] = value
	}
	return merged
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
