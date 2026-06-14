package admin

import (
	"path/filepath"
	"testing"
	"time"
)

func TestUserStoreCreatesAndPersistsUsers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	store, err := NewUserStore(path, "root", "root")
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create("root", CreateUserRequest{
		Username: "ops",
		Password: "secret",
		Role:     RoleAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Role != RoleAdmin {
		t.Fatalf("expected admin role, got %s", created.Role)
	}
	if _, ok := store.Authenticate("ops", "secret"); !ok {
		t.Fatal("expected created user to authenticate")
	}

	reopened, err := NewUserStore(path, "root", "root")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.Authenticate("ops", "secret"); !ok {
		t.Fatal("expected persisted user to authenticate")
	}
	if err := reopened.Delete("root"); err != ErrCannotDeleteRoot {
		t.Fatalf("expected ErrCannotDeleteRoot, got %v", err)
	}
}

func TestUserStoreRejectsInvalidRole(t *testing.T) {
	store := NewMemoryUserStore("root", "root")
	_, err := store.Create("root", CreateUserRequest{
		Username: "bad",
		Password: "secret",
		Role:     RoleRoot,
	})
	if err != ErrInvalidRole {
		t.Fatalf("expected ErrInvalidRole, got %v", err)
	}
}

func TestAuditStorePrunesExpiredEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	store, err := NewAuditStore(path, 31)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := store.Write(AuditEntry{Actor: "root", Role: RoleRoot, Action: "old", CreatedAt: now.AddDate(0, 0, -32)}); err != nil {
		t.Fatal(err)
	}
	if err := store.Write(AuditEntry{Actor: "root", Role: RoleRoot, Action: "fresh", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	removed, err := store.PruneExpired(now)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("expected one removed audit entry, got %d", removed)
	}
	entries := store.List(10)
	if len(entries) != 1 || entries[0].Action != "fresh" {
		t.Fatalf("unexpected audit entries: %#v", entries)
	}
}
