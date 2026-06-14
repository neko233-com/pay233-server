package admin

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrUserExists       = errors.New("admin user already exists")
	ErrUserNotFound     = errors.New("admin user not found")
	ErrInvalidRole      = errors.New("admin role must be admin or employee")
	ErrInvalidPassword  = errors.New("admin password is required")
	ErrCannotDeleteRoot = errors.New("root user cannot be deleted")
)

type Role string

const (
	RoleRoot     Role = "root"
	RoleAdmin    Role = "admin"
	RoleEmployee Role = "employee"
)

type User struct {
	Username     string    `json:"username"`
	Role         Role      `json:"role"`
	PasswordHash string    `json:"password_hash,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	CreatedBy    string    `json:"created_by,omitempty"`
}

type PublicUser struct {
	Username  string    `json:"username"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedBy string    `json:"created_by,omitempty"`
}

type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     Role   `json:"role"`
}

type UserStore struct {
	mu    sync.RWMutex
	path  string
	users map[string]User
}

func NewUserStore(path string, rootUsername string, rootPassword string) (*UserStore, error) {
	store := &UserStore{path: path, users: make(map[string]User)}
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := store.load(); err != nil {
			return nil, err
		}
	}
	if rootUsername == "" {
		rootUsername = "root"
	}
	if rootPassword == "" {
		rootPassword = "root"
	}
	if _, ok := store.users[rootUsername]; !ok {
		hash, err := hashPassword(rootPassword)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		store.users[rootUsername] = User{
			Username:     rootUsername,
			Role:         RoleRoot,
			PasswordHash: hash,
			CreatedAt:    now,
			UpdatedAt:    now,
			CreatedBy:    "system",
		}
		if err := store.saveLocked(); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func NewMemoryUserStore(rootUsername string, rootPassword string) *UserStore {
	store, err := NewUserStore("", rootUsername, rootPassword)
	if err != nil {
		panic(err)
	}
	return store
}

func (s *UserStore) Authenticate(username string, password string) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[username]
	if !ok || !verifyPassword(user.PasswordHash, password) {
		return User{}, false
	}
	return user, true
}

func (s *UserStore) Get(username string) (User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[username]
	return user, ok
}

func (s *UserStore) List() []PublicUser {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]PublicUser, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user.Public())
	}
	sort.Slice(users, func(i, j int) bool {
		if users[i].Role == users[j].Role {
			return users[i].Username < users[j].Username
		}
		return roleRank(users[i].Role) < roleRank(users[j].Role)
	})
	return users
}

func (s *UserStore) Create(actor string, req CreateUserRequest) (PublicUser, error) {
	if req.Username == "" {
		return PublicUser{}, fmt.Errorf("username is required")
	}
	if req.Password == "" {
		return PublicUser{}, ErrInvalidPassword
	}
	if req.Role != RoleAdmin && req.Role != RoleEmployee {
		return PublicUser{}, ErrInvalidRole
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		return PublicUser{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[req.Username]; ok {
		return PublicUser{}, ErrUserExists
	}
	now := time.Now().UTC()
	user := User{
		Username:     req.Username,
		Role:         req.Role,
		PasswordHash: hash,
		CreatedAt:    now,
		UpdatedAt:    now,
		CreatedBy:    actor,
	}
	s.users[user.Username] = user
	if err := s.saveLocked(); err != nil {
		delete(s.users, user.Username)
		return PublicUser{}, err
	}
	return user.Public(), nil
}

func (s *UserStore) Delete(username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[username]
	if !ok {
		return ErrUserNotFound
	}
	if user.Role == RoleRoot {
		return ErrCannotDeleteRoot
	}
	delete(s.users, username)
	return s.saveLocked()
}

func (u User) Public() PublicUser {
	return PublicUser{
		Username:  u.Username,
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
		CreatedBy: u.CreatedBy,
	}
}

func (s *UserStore) load() error {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	var users []User
	if err := json.Unmarshal(raw, &users); err != nil {
		return err
	}
	for _, user := range users {
		s.users[user.Username] = user
	}
	return nil
}

func (s *UserStore) saveLocked() error {
	if s.path == "" {
		return nil
	}
	users := make([]User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].Username < users[j].Username })
	raw, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func roleRank(role Role) int {
	switch role {
	case RoleRoot:
		return 0
	case RoleAdmin:
		return 1
	default:
		return 2
	}
}

const passwordIterations = 100000

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := derivePassword(password, salt, passwordIterations)
	return fmt.Sprintf("sha256$%d$%s$%s", passwordIterations, hex.EncodeToString(salt), hex.EncodeToString(hash)), nil
}

func verifyPassword(encoded string, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "sha256" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
		return false
	}
	salt, err := hex.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got := derivePassword(password, salt, iterations)
	return subtleEqual(got, want)
}

func derivePassword(password string, salt []byte, iterations int) []byte {
	input := append(append([]byte{}, salt...), []byte(password)...)
	sum := sha256.Sum256(input)
	for i := 1; i < iterations; i++ {
		input = append(append(sum[:], salt...), []byte(password)...)
		sum = sha256.Sum256(input)
	}
	return sum[:]
}

func subtleEqual(a []byte, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var out byte
	for i := range a {
		out |= a[i] ^ b[i]
	}
	return out == 0
}

type AuditEntry struct {
	ID        string            `json:"id"`
	Actor     string            `json:"actor"`
	Role      Role              `json:"role"`
	Action    string            `json:"action"`
	Target    string            `json:"target,omitempty"`
	Details   map[string]string `json:"details,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

type AuditStore struct {
	mu            sync.RWMutex
	path          string
	retentionDays int
	entries       []AuditEntry
}

func NewAuditStore(path string, retentionDays int) (*AuditStore, error) {
	if retentionDays <= 0 {
		retentionDays = 31
	}
	store := &AuditStore{path: path, retentionDays: retentionDays}
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := store.loadAudit(); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func NewMemoryAuditStore(retentionDays int) *AuditStore {
	store, err := NewAuditStore("", retentionDays)
	if err != nil {
		panic(err)
	}
	return store
}

func (s *AuditStore) Write(entry AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry.ID == "" {
		entry.ID = newAuditID()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	if entry.Details != nil && len(entry.Details) == 0 {
		entry.Details = nil
	}
	s.entries = append(s.entries, entry)
	if s.path == "" {
		return nil
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(raw, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func (s *AuditStore) List(limit int) []AuditEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	entries := make([]AuditEntry, len(s.entries))
	copy(entries, s.entries)
	sort.Slice(entries, func(i, j int) bool { return entries[i].CreatedAt.After(entries[j].CreatedAt) })
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries
}

func (s *AuditStore) PruneExpired(now time.Time) (int, error) {
	cutoff := now.UTC().AddDate(0, 0, -s.retentionDays)
	return s.PruneBefore(cutoff)
}

func (s *AuditStore) PruneBefore(cutoff time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.entries[:0]
	removed := 0
	for _, entry := range s.entries {
		if entry.CreatedAt.Before(cutoff) {
			removed++
			continue
		}
		kept = append(kept, entry)
	}
	s.entries = kept
	if removed == 0 {
		return 0, nil
	}
	return removed, s.rewriteLocked()
}

func (s *AuditStore) loadAudit() error {
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
		var entry AuditEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return err
		}
		s.entries = append(s.entries, entry)
	}
	return scanner.Err()
}

func (s *AuditStore) rewriteLocked() error {
	if s.path == "" {
		return nil
	}
	tmp := s.path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	for _, entry := range s.entries {
		raw, err := json.Marshal(entry)
		if err != nil {
			_ = file.Close()
			return err
		}
		if _, err := file.Write(append(raw, '\n')); err != nil {
			_ = file.Close()
			return err
		}
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func newAuditID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("audit_%d", time.Now().UnixNano())
	}
	return "audit_" + hex.EncodeToString(buf)
}
