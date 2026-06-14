package payment

import (
	"sync"
	"time"
)

type Store interface {
	Create(Payment) (Payment, error)
	Get(id string) (Payment, bool)
	Update(Payment) (Payment, error)
	FindByOutTradeNo(channel string, outTradeNo string) (Payment, bool)
}

type MemoryStore struct {
	mu       sync.RWMutex
	records  map[string]Payment
	outIndex map[string]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		records:  make(map[string]Payment),
		outIndex: make(map[string]string),
	}
}

func (s *MemoryStore) Create(payment Payment) (Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	payment.CreatedAt = now
	payment.UpdatedAt = now
	s.records[payment.ID] = payment
	s.outIndex[indexKey(payment.Channel, payment.OutTradeNo)] = payment.ID
	return payment, nil
}

func (s *MemoryStore) Get(id string) (Payment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	payment, ok := s.records[id]
	return payment, ok
}

func (s *MemoryStore) Update(payment Payment) (Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.records[payment.ID]; !ok {
		return Payment{}, ErrPaymentNotFound
	}
	payment.UpdatedAt = time.Now().UTC()
	s.records[payment.ID] = payment
	s.outIndex[indexKey(payment.Channel, payment.OutTradeNo)] = payment.ID
	return payment, nil
}

func (s *MemoryStore) FindByOutTradeNo(channel string, outTradeNo string) (Payment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.outIndex[indexKey(channel, outTradeNo)]
	if !ok {
		return Payment{}, false
	}
	payment, ok := s.records[id]
	return payment, ok
}

func indexKey(channel string, outTradeNo string) string {
	return channel + "\x00" + outTradeNo
}
