package payment

import (
	"context"
	"errors"
	"time"
)

var (
	ErrChannelNotConfigured = errors.New("payment channel is not configured")
	ErrPaymentNotFound      = errors.New("payment not found")
	ErrInvalidPaymentState  = errors.New("invalid payment state")
)

type Money struct {
	Currency string `json:"currency"`
	Amount   int64  `json:"amount"`
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
	MerchantID string            `json:"merchant_id"`
	OutTradeNo string            `json:"out_trade_no"`
	Channel    string            `json:"channel"`
	Amount     Money             `json:"amount"`
	Subject    string            `json:"subject"`
	NotifyURL  string            `json:"notify_url,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
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
	ProviderTrade string
	OutTradeNo    string
	Status        PaymentStatus
	Raw           []byte
}

type ProviderInfo struct {
	Name         string   `json:"name"`
	DisplayName  string   `json:"display_name"`
	Family       string   `json:"family"`
	Capabilities []string `json:"capabilities"`
	Health       string   `json:"health"`
}

type Provider interface {
	Info() ProviderInfo
	CreatePayment(context.Context, ProviderCreateRequest) (ProviderCreateResponse, error)
	ClosePayment(context.Context, Payment) error
	ParseWebhook(context.Context, []byte, map[string]string) (WebhookEvent, error)
}
