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
	StatusCreated PaymentStatus = "created"
	StatusPending PaymentStatus = "pending"
	StatusPaid    PaymentStatus = "paid"
	StatusClosed  PaymentStatus = "closed"
	StatusFailed  PaymentStatus = "failed"
)

type Payment struct {
	ID            string            `json:"id"`
	MerchantID    string            `json:"merchant_id"`
	OutTradeNo    string            `json:"out_trade_no"`
	Channel       string            `json:"channel"`
	Amount        Money             `json:"amount"`
	Subject       string            `json:"subject"`
	NotifyURL     string            `json:"notify_url,omitempty"`
	Status        PaymentStatus     `json:"status"`
	ProviderTrade string            `json:"provider_trade,omitempty"`
	PayURL        string            `json:"pay_url,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
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

type Provider interface {
	CreatePayment(context.Context, ProviderCreateRequest) (ProviderCreateResponse, error)
	ClosePayment(context.Context, Payment) error
	ParseWebhook(context.Context, []byte, map[string]string) (WebhookEvent, error)
}
