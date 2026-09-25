package invitation

import (
	"sync"
	"testing"
	"time"
)

func TestGenerateToken(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	if len(token) == 0 {
		t.Fatal("GenerateToken() returned empty token")
	}

	if len(token) != len(TokenPrefix)+TokenByteLen*2 {
		t.Fatalf("GenerateToken() returned token of unexpected length: %d", len(token))
	}
}

func TestTokenUniqueness(t *testing.T) {
	tokens := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		token, err := GenerateToken()
		if err != nil {
			t.Fatalf("GenerateToken() error = %v", err)
		}
		if tokens[token] {
			t.Fatalf("GenerateToken() produced duplicate token: %s", token)
		}
		tokens[token] = true
	}
}

func TestCreateInvitation(t *testing.T) {
	m := NewManager()
	inv, err := m.Create("TR-TEST1", 10*time.Minute)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if inv.RoomID != "TR-TEST1" {
		t.Errorf("Create() RoomID = %s, want TR-TEST1", inv.RoomID)
	}

	if inv.Consumed {
		t.Error("Create() created invitation is already consumed")
	}

	if inv.IsExpired() {
		t.Error("Create() created invitation is already expired")
	}
}

func TestValidateInvitation(t *testing.T) {
	m := NewManager()
	inv, err := m.Create("TR-TEST1", 10*time.Minute)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	validated, err := m.Validate(inv.Token, "TR-TEST1")
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if validated.Token != inv.Token {
		t.Errorf("Validate() returned different token")
	}
}

func TestOneTimeUse(t *testing.T) {
	m := NewManager()
	inv, err := m.Create("TR-TEST1", 10*time.Minute)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	_, err = m.Validate(inv.Token, "TR-TEST1")
	if err != nil {
		t.Fatalf("first Validate() error = %v", err)
	}

	_, err = m.Validate(inv.Token, "TR-TEST1")
	if err == nil {
		t.Fatal("second Validate() should have failed")
	}

	if err.Error() != "invitation already used" {
		t.Errorf("second Validate() error = %v, want 'invitation already used'", err)
	}
}

func TestExpiredInvitation(t *testing.T) {
	m := NewManager()
	inv, err := m.Create("TR-TEST1", -1*time.Minute)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	_, err = m.Validate(inv.Token, "TR-TEST1")
	if err == nil {
		t.Fatal("Validate() should have failed for expired invitation")
	}

	if err.Error() != "invitation expired" {
		t.Errorf("Validate() error = %v, want 'invitation expired'", err)
	}
}

func TestInvalidToken(t *testing.T) {
	m := NewManager()

	_, err := m.Validate("TRINV-invalid-token", "TR-TEST1")
	if err == nil {
		t.Fatal("Validate() should have failed for invalid token")
	}

	if err.Error() != "invalid invitation" {
		t.Errorf("Validate() error = %v, want 'invalid invitation'", err)
	}
}

func TestWrongRoomID(t *testing.T) {
	m := NewManager()
	inv, err := m.Create("TR-TEST1", 10*time.Minute)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	_, err = m.Validate(inv.Token, "TR-OTHER")
	if err == nil {
		t.Fatal("Validate() should have failed for wrong room ID")
	}

	if err.Error() != "invitation does not match this room" {
		t.Errorf("Validate() error = %v, want 'invitation does not match this room'", err)
	}
}

func TestConsumeInvitation(t *testing.T) {
	inv := &Invitation{
		Token:     "TRINV-test",
		RoomID:    "TR-TEST1",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Consumed:  false,
	}

	if !inv.Consume() {
		t.Fatal("Consume() should return true for first consumption")
	}

	if inv.Consume() {
		t.Fatal("Consume() should return true for second consumption")
	}
}

func TestConcurrentConsume(t *testing.T) {
	inv := &Invitation{
		Token:     "TRINV-test",
		RoomID:    "TR-TEST1",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Consumed:  false,
	}

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if inv.Consume() {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if successCount != 1 {
		t.Errorf("Concurrent Consume() succeeded %d times, want 1", successCount)
	}
}

func TestTokenToRoomBinding(t *testing.T) {
	m := NewManager()
	inv1, _ := m.Create("TR-ROOM1", 10*time.Minute)
	inv2, _ := m.Create("TR-ROOM2", 10*time.Minute)

	_, err := m.Validate(inv1.Token, "TR-ROOM2")
	if err == nil {
		t.Fatal("Validate() should have failed for room mismatch")
	}

	_, err = m.Validate(inv2.Token, "TR-ROOM1")
	if err == nil {
		t.Fatal("Validate() should have failed for room mismatch")
	}
}

func TestCleanupExpired(t *testing.T) {
	m := NewManager()
	m.Create("TR-ROOM1", -1*time.Minute)
	m.Create("TR-ROOM2", 10*time.Minute)

	if m.ActiveCount() != 2 {
		t.Fatalf("ActiveCount() = %d, want 2", m.ActiveCount())
	}

	m.Cleanup()

	if m.ActiveCount() != 1 {
		t.Errorf("ActiveCount() after Cleanup() = %d, want 1", m.ActiveCount())
	}
}

func TestRemoveByRoom(t *testing.T) {
	m := NewManager()
	m.Create("TR-ROOM1", 10*time.Minute)
	m.Create("TR-ROOM1", 10*time.Minute)
	m.Create("TR-ROOM2", 10*time.Minute)

	m.RemoveByRoom("TR-ROOM1")

	if m.ActiveCount() != 1 {
		t.Errorf("ActiveCount() after RemoveByRoom() = %d, want 1", m.ActiveCount())
	}
}

func TestInvitationIsValid(t *testing.T) {
	inv := &Invitation{
		Token:     "TRINV-test",
		RoomID:    "TR-TEST1",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Consumed:  false,
	}

	if !inv.IsValid() {
		t.Error("IsValid() should return true for valid invitation")
	}

	inv.Consume()

	if inv.IsValid() {
		t.Error("IsValid() should return false for consumed invitation")
	}
}

func TestInvitationIsExpired(t *testing.T) {
	inv := &Invitation{
		Token:     "TRINV-test",
		RoomID:    "TR-TEST1",
		CreatedAt: time.Now().Add(-20 * time.Minute),
		ExpiresAt: time.Now().Add(-10 * time.Minute),
		Consumed:  false,
	}

	if !inv.IsExpired() {
		t.Error("IsExpired() should return true for expired invitation")
	}
}

func TestConcurrentValidateByToken(t *testing.T) {
	m := NewManager()
	inv, err := m.Create("TR-TEST1", 10*time.Minute)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := m.ValidateByToken(inv.Token)
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if successCount != 1 {
		t.Errorf("Concurrent ValidateByToken() succeeded %d times, want 1", successCount)
	}
}
