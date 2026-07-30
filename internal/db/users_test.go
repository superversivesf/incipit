package db

import (
	"testing"
)

func TestCreateAndGetUser(t *testing.T) {
	d, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer d.Close()
	d.Migrate()

	id, err := d.CreateUser("alice", "hashedpassword123", "admin")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}

	user, err := d.GetUser("alice")
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if user.Username != "alice" {
		t.Errorf("expected username 'alice', got %q", user.Username)
	}
	if user.PasswordHash != "hashedpassword123" {
		t.Errorf("expected hash 'hashedpassword123', got %q", user.PasswordHash)
	}
	if user.Role != "admin" {
		t.Errorf("expected role 'admin', got %q", user.Role)
	}
}

func TestGetUserNotFound(t *testing.T) {
	d, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer d.Close()
	d.Migrate()

	_, err = d.GetUser("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent user, got nil")
	}
}

func TestCreateUserDuplicateUpdatesPassword(t *testing.T) {
	d, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer d.Close()
	d.Migrate()

	d.CreateUser("bob", "oldhash", "user")
	d.CreateUser("bob", "newhash", "user")

	user, _ := d.GetUser("bob")
	if user.PasswordHash != "newhash" {
		t.Errorf("expected updated hash 'newhash', got %q", user.PasswordHash)
	}
}

func TestListUsers(t *testing.T) {
	d, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer d.Close()
	d.Migrate()

	d.CreateUser("alice", "h1", "admin")
	d.CreateUser("bob", "h2", "user")

	users, err := d.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
}

func TestDeleteUser(t *testing.T) {
	d, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer d.Close()
	d.Migrate()

	d.CreateUser("alice", "h1", "admin")
	d.DeleteUser("alice")

	_, err = d.GetUser("alice")
	if err == nil {
		t.Fatal("expected error after deletion, got nil")
	}
}
