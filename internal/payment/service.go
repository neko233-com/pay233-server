package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"time"
)

type Service struct {
	registry *Registry
	store    Store
}

type ListFilter struct {
	EnvType EnvType
}

func NewService(registry *Registry, store Store) *Service {
	return &Service{registry: registry, store: store}
}

func (s *Service) Create(ctx context.Context, req CreatePaymentRequest) (Payment, error) {
	envType, err := NormalizeEnvType(string(req.EnvType))
	if err != nil {
		return Payment{}, err
	}
	req.EnvType = envType
	provider, ok := s.registry.Get(req.Channel)
	if !ok {
		return Payment{}, ErrChannelNotConfigured
	}
	if err := validateCreate(req); err != nil {
		return Payment{}, err
	}

	payment := Payment{
		ID:             newID("pay"),
		EnvType:        req.EnvType,
		MerchantID:     req.MerchantID,
		OutTradeNo:     req.OutTradeNo,
		Channel:        req.Channel,
		Amount:         req.Amount,
		Subject:        req.Subject,
		NotifyURL:      req.NotifyURL,
		Status:         StatusCreated,
		CallbackStatus: CallbackNone,
		Metadata:       req.Metadata,
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
	if created.NotifyURL != "" {
		created.CallbackStatus = CallbackPending
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

	payment, ok := s.store.FindByOutTradeNo(event.EnvType, channel, event.OutTradeNo)
	if !ok {
		return Payment{}, ErrPaymentNotFound
	}
	payment.ProviderTrade = event.ProviderTrade
	payment.Status = event.Status
	payment.CallbackError = ""
	if event.Status == StatusPaid {
		if payment.NotifyURL != "" {
			payment.CallbackStatus = CallbackPending
		}
	} else if event.Status == StatusFailed {
		payment.FailureReason = "provider reported failure"
		if payment.NotifyURL != "" {
			payment.CallbackStatus = CallbackPending
		}
	}
	return s.store.Update(payment)
}

func (s *Service) List() []Payment {
	return s.ListFiltered(ListFilter{})
}

func (s *Service) ListFiltered(filter ListFilter) []Payment {
	payments := s.store.List()
	if filter.EnvType != "" {
		filtered := make([]Payment, 0, len(payments))
		for _, payment := range payments {
			if payment.EnvType == filter.EnvType {
				filtered = append(filtered, payment)
			}
		}
		payments = filtered
	}
	sort.Slice(payments, func(i, j int) bool {
		return payments[i].UpdatedAt.After(payments[j].UpdatedAt)
	})
	return payments
}

func (s *Service) MarkLost(id string, reason string) (Payment, error) {
	payment, err := s.Get(id)
	if err != nil {
		return Payment{}, err
	}
	payment.Status = StatusLost
	payment.CallbackStatus = CallbackLost
	payment.FailureReason = reason
	if payment.FailureReason == "" {
		payment.FailureReason = "manual reconciliation marked lost"
	}
	return s.store.Update(payment)
}

func (s *Service) RecordMerchantCallback(id string, success bool, message string) (Payment, error) {
	payment, err := s.Get(id)
	if err != nil {
		return Payment{}, err
	}
	now := time.Now().UTC()
	payment.LastCallbackAt = &now
	payment.CallbackAttempts++
	if success {
		payment.CallbackStatus = CallbackSuccess
		payment.CallbackError = ""
	} else {
		payment.CallbackStatus = CallbackFailed
		payment.CallbackError = message
		if payment.CallbackError == "" {
			payment.CallbackError = "merchant callback failed"
		}
	}
	return s.store.Update(payment)
}

type Dashboard struct {
	KPIs        DashboardKPIs   `json:"kpis"`
	Series      []DailyMetric   `json:"series"`
	Channels    []ChannelMetric `json:"channels"`
	Failures    []ReasonMetric  `json:"failures"`
	Abnormal    []Payment       `json:"abnormal"`
	ChannelInfo []ProviderInfo  `json:"channel_info"`
}

type DashboardKPIs struct {
	GMV              int64   `json:"gmv"`
	TotalPayments    int     `json:"total_payments"`
	SuccessRate      float64 `json:"success_rate"`
	CallbackFailures int     `json:"callback_failures"`
	LostOrders       int     `json:"lost_orders"`
	UnsettledAmount  int64   `json:"unsettled_amount"`
}

type DailyMetric struct {
	Date        string  `json:"date"`
	Amount      int64   `json:"amount"`
	SuccessRate float64 `json:"success_rate"`
}

type ChannelMetric struct {
	Channel string `json:"channel"`
	Amount  int64  `json:"amount"`
	Count   int    `json:"count"`
}

type ReasonMetric struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

func (s *Service) Dashboard(filter ListFilter) Dashboard {
	payments := s.ListFiltered(filter)
	dashboard := Dashboard{Abnormal: make([]Payment, 0)}
	byDate := map[string]*DailyMetric{}
	byChannel := map[string]*ChannelMetric{}
	byReason := map[string]*ReasonMetric{}
	success := 0
	totalByDate := map[string]int{}
	successByDate := map[string]int{}

	for _, p := range payments {
		dashboard.KPIs.TotalPayments++
		if p.Status == StatusPaid {
			success++
			dashboard.KPIs.GMV += p.Amount.Amount
		}
		if p.Status == StatusPending || p.CallbackStatus == CallbackPending {
			dashboard.KPIs.UnsettledAmount += p.Amount.Amount
		}
		if p.CallbackStatus == CallbackFailed {
			dashboard.KPIs.CallbackFailures++
		}
		if p.Status == StatusLost || p.CallbackStatus == CallbackLost {
			dashboard.KPIs.LostOrders++
		}
		if isAbnormal(p) {
			dashboard.Abnormal = append(dashboard.Abnormal, p)
		}
		date := p.CreatedAt.Format("2006-01-02")
		totalByDate[date]++
		if p.Status == StatusPaid {
			successByDate[date]++
		}
		if _, ok := byDate[date]; !ok {
			byDate[date] = &DailyMetric{Date: date}
		}
		byDate[date].Amount += p.Amount.Amount
		if _, ok := byChannel[p.Channel]; !ok {
			byChannel[p.Channel] = &ChannelMetric{Channel: p.Channel}
		}
		byChannel[p.Channel].Amount += p.Amount.Amount
		byChannel[p.Channel].Count++
		reason := p.FailureReason
		if reason == "" && p.CallbackStatus == CallbackPending {
			reason = "waiting callback"
		}
		if reason != "" {
			if _, ok := byReason[reason]; !ok {
				byReason[reason] = &ReasonMetric{Reason: reason}
			}
			byReason[reason].Count++
		}
	}

	if dashboard.KPIs.TotalPayments > 0 {
		dashboard.KPIs.SuccessRate = float64(success) / float64(dashboard.KPIs.TotalPayments)
	}
	for date, metric := range byDate {
		if totalByDate[date] > 0 {
			metric.SuccessRate = float64(successByDate[date]) / float64(totalByDate[date])
		}
		dashboard.Series = append(dashboard.Series, *metric)
	}
	for _, metric := range byChannel {
		dashboard.Channels = append(dashboard.Channels, *metric)
	}
	for _, metric := range byReason {
		dashboard.Failures = append(dashboard.Failures, *metric)
	}
	sort.Slice(dashboard.Series, func(i, j int) bool { return dashboard.Series[i].Date < dashboard.Series[j].Date })
	sort.Slice(dashboard.Channels, func(i, j int) bool { return dashboard.Channels[i].Amount > dashboard.Channels[j].Amount })
	sort.Slice(dashboard.Failures, func(i, j int) bool { return dashboard.Failures[i].Count > dashboard.Failures[j].Count })
	if len(dashboard.Abnormal) > 50 {
		dashboard.Abnormal = dashboard.Abnormal[:50]
	}
	dashboard.ChannelInfo = s.registry.ChannelInfos()
	return dashboard
}

func isAbnormal(p Payment) bool {
	return p.Status == StatusLost || p.Status == StatusFailed || p.CallbackStatus == CallbackFailed || p.CallbackStatus == CallbackLost || p.CallbackStatus == CallbackPending
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
