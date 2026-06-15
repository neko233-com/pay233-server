package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrChannelNotConfigured = errors.New("payment channel is not configured")
	ErrPaymentNotFound      = errors.New("payment not found")
	ErrInvalidPaymentState  = errors.New("invalid payment state")
	ErrDuplicatePayment     = errors.New("payment already exists for envType, channel, and out_trade_no")
)

type Money struct {
	Currency string `json:"currency"`
	Amount   int64  `json:"amount"`
}

type EnvType string

const (
	EnvTypeTest    EnvType = "test"
	EnvTypeRelease EnvType = "release"
)

func NormalizeEnvType(value string) (EnvType, error) {
	switch EnvType(value) {
	case "", EnvTypeTest:
		return EnvTypeTest, nil
	case EnvTypeRelease:
		return EnvTypeRelease, nil
	default:
		return "", fmt.Errorf("envType must be test or release")
	}
}

type PaymentStatus string

const (
	StatusCreated  PaymentStatus = "created"
	StatusPending  PaymentStatus = "pending"
	StatusPaid     PaymentStatus = "paid"
	StatusClosed   PaymentStatus = "closed"
	StatusFailed   PaymentStatus = "failed"
	StatusRefunded PaymentStatus = "refunded"
	StatusLost     PaymentStatus = "lost"
)

type CallbackStatus string

const (
	CallbackNone    CallbackStatus = "none"
	CallbackPending CallbackStatus = "pending"
	CallbackSuccess CallbackStatus = "success"
	CallbackFailed  CallbackStatus = "failed"
	CallbackLost    CallbackStatus = "lost"
)

type Payment struct {
	ID               string            `json:"id"`
	EnvType          EnvType           `json:"env_type"`
	MerchantID       string            `json:"merchant_id"`
	OutTradeNo       string            `json:"out_trade_no"`
	Channel          string            `json:"channel"`
	Amount           Money             `json:"amount"`
	Subject          string            `json:"subject"`
	NotifyURL        string            `json:"notify_url,omitempty"`
	Status           PaymentStatus     `json:"status"`
	ProviderTrade    string            `json:"provider_trade,omitempty"`
	PayURL           string            `json:"pay_url,omitempty"`
	CallbackStatus   CallbackStatus    `json:"callback_status"`
	CallbackAttempts int               `json:"callback_attempts"`
	LastCallbackAt   *time.Time        `json:"last_callback_at,omitempty"`
	CallbackError    string            `json:"callback_error,omitempty"`
	FailureReason    string            `json:"failure_reason,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

type CreatePaymentRequest struct {
	EnvType    EnvType           `json:"env_type"`
	MerchantID string            `json:"merchant_id"`
	OutTradeNo string            `json:"out_trade_no"`
	Channel    string            `json:"channel"`
	Amount     Money             `json:"amount"`
	Subject    string            `json:"subject"`
	NotifyURL  string            `json:"notify_url,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

func (r *CreatePaymentRequest) UnmarshalJSON(data []byte) error {
	type alias CreatePaymentRequest
	var raw struct {
		alias
		EnvTypeCamel string `json:"envType"`
		EnvTypeSnake string `json:"env_type"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = CreatePaymentRequest(raw.alias)
	env := raw.EnvTypeSnake
	if env == "" {
		env = raw.EnvTypeCamel
	}
	normalized, err := NormalizeEnvType(env)
	if err != nil {
		return err
	}
	r.EnvType = normalized
	return nil
}

type ProviderCreateRequest struct {
	Payment Payment
}

type ProviderCreateResponse struct {
	ProviderTrade string
	PayURL        string
	Status        PaymentStatus
}

type WebhookEvent struct {
	EnvType       EnvType
	ProviderTrade string
	OutTradeNo    string
	Status        PaymentStatus
	Raw           []byte
}

type ProviderInfo struct {
	Name          string     `json:"name"`
	EnvType       EnvType    `json:"env_type,omitempty"`
	DisplayName   string     `json:"display_name"`
	Family        string     `json:"family"`
	Capabilities  []string   `json:"capabilities"`
	Health        string     `json:"health"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
	LatencyMS     int64      `json:"latency_ms,omitempty"`
}

type Provider interface {
	Info() ProviderInfo
	HealthEnvironments() []EnvType
	CheckHealth(context.Context, EnvType) ProviderInfo
	CreatePayment(context.Context, ProviderCreateRequest) (ProviderCreateResponse, error)
	ClosePayment(context.Context, Payment) error
	ParseWebhook(context.Context, []byte, map[string]string) (WebhookEvent, error)
}
