package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

type Service struct {
	registry *Registry
	store    Store
}

func NewService(registry *Registry, store Store) *Service {
	return &Service{registry: registry, store: store}
}

func (s *Service) Create(ctx context.Context, req CreatePaymentRequest) (Payment, error) {
	provider, ok := s.registry.Get(req.Channel)
	if !ok {
		return Payment{}, ErrChannelNotConfigured
	}
	if err := validateCreate(req); err != nil {
		return Payment{}, err
	}

	payment := Payment{
		ID:         newID("pay"),
		MerchantID: req.MerchantID,
		OutTradeNo: req.OutTradeNo,
		Channel:    req.Channel,
		Amount:     req.Amount,
		Subject:    req.Subject,
		NotifyURL:  req.NotifyURL,
		Status:     StatusCreated,
		Metadata:   req.Metadata,
	}

	created, err := s.store.Create(payment)
	if err != nil {
		return Payment{}, err
	}

	providerResp, err := provider.CreatePayment(ctx, ProviderCreateRequest{Payment: created})
	if err != nil {
		created.Status = StatusFailed
		updated, updateErr := s.store.Update(created)
		if updateErr != nil {
			return Payment{}, updateErr
		}
		return updated, err
	}

	created.ProviderTrade = providerResp.ProviderTrade
	created.PayURL = providerResp.PayURL
	created.Status = providerResp.Status
	if created.Status == "" {
		created.Status = StatusPending
	}
	return s.store.Update(created)
}

func (s *Service) Get(id string) (Payment, error) {
	payment, ok := s.store.Get(id)
	if !ok {
		return Payment{}, ErrPaymentNotFound
	}
	return payment, nil
}

func (s *Service) Close(ctx context.Context, id string) (Payment, error) {
	payment, err := s.Get(id)
	if err != nil {
		return Payment{}, err
	}
	if payment.Status == StatusPaid || payment.Status == StatusClosed {
		return Payment{}, ErrInvalidPaymentState
	}

	provider, ok := s.registry.Get(payment.Channel)
	if !ok {
		return Payment{}, ErrChannelNotConfigured
	}
	if err := provider.ClosePayment(ctx, payment); err != nil {
		return Payment{}, err
	}

	payment.Status = StatusClosed
	return s.store.Update(payment)
}

func (s *Service) ApplyWebhook(ctx context.Context, channel string, raw []byte, headers map[string]string) (Payment, error) {
	provider, ok := s.registry.Get(channel)
	if !ok {
		return Payment{}, ErrChannelNotConfigured
	}
	event, err := provider.ParseWebhook(ctx, raw, headers)
	if err != nil {
		return Payment{}, err
	}

	payment, ok := s.store.FindByOutTradeNo(channel, event.OutTradeNo)
	if !ok {
		return Payment{}, ErrPaymentNotFound
	}
	payment.ProviderTrade = event.ProviderTrade
	payment.Status = event.Status
	return s.store.Update(payment)
}

func validateCreate(req CreatePaymentRequest) error {
	if req.MerchantID == "" {
		return fmt.Errorf("merchant_id is required")
	}
	if req.OutTradeNo == "" {
		return fmt.Errorf("out_trade_no is required")
	}
	if req.Channel == "" {
		return fmt.Errorf("channel is required")
	}
	if req.Amount.Currency == "" {
		return fmt.Errorf("amount.currency is required")
	}
	if req.Amount.Amount <= 0 {
		return fmt.Errorf("amount.amount must be greater than zero")
	}
	if req.Subject == "" {
		return fmt.Errorf("subject is required")
	}
	return nil
}

func newID(prefix string) string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return prefix + "_fallback"
	}
	return prefix + "_" + hex.EncodeToString(buf)
}
