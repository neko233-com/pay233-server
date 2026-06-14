package payment

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Store interface {
	Create(Payment) (Payment, error)
	Get(id string) (Payment, bool)
	List() []Payment
	Update(Payment) (Payment, error)
	FindByOutTradeNo(envType EnvType, channel string, outTradeNo string) (Payment, bool)
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
	if _, exists := s.outIndex[indexKey(payment.EnvType, payment.Channel, payment.OutTradeNo)]; exists {
		return Payment{}, ErrDuplicatePayment
	}
	s.records[payment.ID] = payment
	s.outIndex[indexKey(payment.EnvType, payment.Channel, payment.OutTradeNo)] = payment.ID
	return payment, nil
}

func (s *MemoryStore) Get(id string) (Payment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	payment, ok := s.records[id]
	return payment, ok
}

func (s *MemoryStore) List() []Payment {
	s.mu.RLock()
	defer s.mu.RUnlock()

	payments := make([]Payment, 0, len(s.records))
	for _, payment := range s.records {
		payments = append(payments, payment)
	}
	return payments
}

func (s *MemoryStore) Update(payment Payment) (Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.records[payment.ID]; !ok {
		return Payment{}, ErrPaymentNotFound
	}
	payment.UpdatedAt = time.Now().UTC()
	s.records[payment.ID] = payment
	s.outIndex[indexKey(payment.EnvType, payment.Channel, payment.OutTradeNo)] = payment.ID
	return payment, nil
}

func (s *MemoryStore) FindByOutTradeNo(envType EnvType, channel string, outTradeNo string) (Payment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.outIndex[indexKey(envType, channel, outTradeNo)]
	if !ok {
		return Payment{}, false
	}
	payment, ok := s.records[id]
	return payment, ok
}

func indexKey(envType EnvType, channel string, outTradeNo string) string {
	return string(envType) + "\x00" + channel + "\x00" + outTradeNo
}

type FileStore struct {
	mu       sync.RWMutex
	path     string
	file     *os.File
	records  map[string]Payment
	outIndex map[string]string
}

func NewFileStore(path string) (*FileStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	store := &FileStore{
		path:     path,
		records:  make(map[string]Payment),
		outIndex: make(map[string]string),
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	store.file = file
	return store, nil
}

func (s *FileStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	return err
}

func (s *FileStore) Create(payment Payment) (Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	payment.CreatedAt = now
	payment.UpdatedAt = now
	key := indexKey(payment.EnvType, payment.Channel, payment.OutTradeNo)
	if _, exists := s.outIndex[key]; exists {
		return Payment{}, ErrDuplicatePayment
	}
	if err := s.append(payment); err != nil {
		return Payment{}, err
	}
	s.records[payment.ID] = payment
	s.outIndex[key] = payment.ID
	return payment, nil
}

func (s *FileStore) Get(id string) (Payment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	payment, ok := s.records[id]
	return payment, ok
}

func (s *FileStore) List() []Payment {
	s.mu.RLock()
	defer s.mu.RUnlock()

	payments := make([]Payment, 0, len(s.records))
	for _, payment := range s.records {
		payments = append(payments, payment)
	}
	return payments
}

func (s *FileStore) Update(payment Payment) (Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.records[payment.ID]; !ok {
		return Payment{}, ErrPaymentNotFound
	}
	payment.UpdatedAt = time.Now().UTC()
	if err := s.append(payment); err != nil {
		return Payment{}, err
	}
	s.records[payment.ID] = payment
	s.outIndex[indexKey(payment.EnvType, payment.Channel, payment.OutTradeNo)] = payment.ID
	return payment, nil
}

func (s *FileStore) FindByOutTradeNo(envType EnvType, channel string, outTradeNo string) (Payment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.outIndex[indexKey(envType, channel, outTradeNo)]
	if !ok {
		return Payment{}, false
	}
	payment, ok := s.records[id]
	return payment, ok
}

func (s *FileStore) load() error {
	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var payment Payment
		if err := json.Unmarshal(line, &payment); err != nil {
			return err
		}
		s.records[payment.ID] = payment
		s.outIndex[indexKey(payment.EnvType, payment.Channel, payment.OutTradeNo)] = payment.ID
	}
	return scanner.Err()
}

func (s *FileStore) append(payment Payment) error {
	if s.file == nil {
		return os.ErrClosed
	}
	encoded, err := json.Marshal(payment)
	if err != nil {
		return err
	}
	if _, err := s.file.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return s.file.Sync()
}
