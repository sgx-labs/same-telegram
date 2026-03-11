package machines

import (
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "machines.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSaveAndRetrieve(t *testing.T) {
	s := testStore(t)

	now := time.Now().UTC().Truncate(time.Second)
	um := &UserMachine{
		UserID:    "user-42",
		MachineID: "m-abc123",
		VolumeID:  "vol-xyz",
		Region:    "dfw",
		State:     "running",
		Token:     "tok-secret",
		CreatedAt: now,
		LastUsed:  now,
	}

	if err := s.SaveUserMachine(um); err != nil {
		t.Fatalf("SaveUserMachine: %v", err)
	}

	got, err := s.GetUserMachine("user-42")
	if err != nil {
		t.Fatalf("GetUserMachine: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil UserMachine")
	}
	if got.UserID != "user-42" {
		t.Errorf("UserID = %q, want user-42", got.UserID)
	}
	if got.MachineID != "m-abc123" {
		t.Errorf("MachineID = %q, want m-abc123", got.MachineID)
	}
	if got.VolumeID != "vol-xyz" {
		t.Errorf("VolumeID = %q, want vol-xyz", got.VolumeID)
	}
	if got.Region != "dfw" {
		t.Errorf("Region = %q, want dfw", got.Region)
	}
	if got.State != "running" {
		t.Errorf("State = %q, want running", got.State)
	}
	if got.Token != "tok-secret" {
		t.Errorf("Token = %q, want tok-secret", got.Token)
	}
	if !got.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, now)
	}
	if !got.LastUsed.Equal(now) {
		t.Errorf("LastUsed = %v, want %v", got.LastUsed, now)
	}
}

func TestUpdateExistingRecord(t *testing.T) {
	s := testStore(t)

	now := time.Now().UTC().Truncate(time.Second)
	um := &UserMachine{
		UserID:    "user-99",
		MachineID: "m-old",
		VolumeID:  "vol-old",
		Region:    "ord",
		State:     "running",
		Token:     "tok-old",
		CreatedAt: now,
		LastUsed:  now,
	}
	if err := s.SaveUserMachine(um); err != nil {
		t.Fatalf("SaveUserMachine (create): %v", err)
	}

	// Update fields.
	later := now.Add(5 * time.Minute)
	um.MachineID = "m-new"
	um.State = "stopped"
	um.Token = "tok-new"
	um.LastUsed = later

	if err := s.SaveUserMachine(um); err != nil {
		t.Fatalf("SaveUserMachine (update): %v", err)
	}

	got, err := s.GetUserMachine("user-99")
	if err != nil {
		t.Fatalf("GetUserMachine: %v", err)
	}
	if got.MachineID != "m-new" {
		t.Errorf("MachineID = %q, want m-new", got.MachineID)
	}
	if got.State != "stopped" {
		t.Errorf("State = %q, want stopped", got.State)
	}
	if got.Token != "tok-new" {
		t.Errorf("Token = %q, want tok-new", got.Token)
	}
	if !got.LastUsed.Equal(later) {
		t.Errorf("LastUsed = %v, want %v", got.LastUsed, later)
	}
	// CreatedAt should remain unchanged.
	if !got.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt changed: got %v, want %v", got.CreatedAt, now)
	}
}

func TestGetNonExistentUser(t *testing.T) {
	s := testStore(t)

	got, err := s.GetUserMachine("nonexistent-user")
	if err != nil {
		t.Fatalf("GetUserMachine: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for non-existent user, got %+v", got)
	}
}

func TestClose(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "machines.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// After close, operations should fail.
	_, err = s.GetUserMachine("user-1")
	if err == nil {
		t.Error("expected error after Close, got nil")
	}
}

func TestMultipleUsers(t *testing.T) {
	s := testStore(t)

	now := time.Now().UTC().Truncate(time.Second)
	for _, uid := range []string{"alice", "bob", "carol"} {
		um := &UserMachine{
			UserID:    uid,
			MachineID: "m-" + uid,
			Region:    "dfw",
			State:     "running",
			Token:     "tok-" + uid,
			CreatedAt: now,
			LastUsed:  now,
		}
		if err := s.SaveUserMachine(um); err != nil {
			t.Fatalf("SaveUserMachine(%s): %v", uid, err)
		}
	}

	// Verify each user is independent.
	got, err := s.GetUserMachine("bob")
	if err != nil {
		t.Fatalf("GetUserMachine: %v", err)
	}
	if got.MachineID != "m-bob" {
		t.Errorf("MachineID = %q, want m-bob", got.MachineID)
	}

	// Alice should be unaffected.
	got, err = s.GetUserMachine("alice")
	if err != nil {
		t.Fatalf("GetUserMachine: %v", err)
	}
	if got.MachineID != "m-alice" {
		t.Errorf("MachineID = %q, want m-alice", got.MachineID)
	}
}
