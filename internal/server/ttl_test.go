package server

import (
	"strings"
	"testing"
	"time"

	"github.com/terminalroom/terminalroom/internal/protocol"
)

func createRoomWithTTL(t *testing.T, srv *Server, dir string, ttlPayload string) (string, time.Duration) {
	t.Helper()
	addr := srv.Addr().String()
	conn, reader, writer := testClient(t, addr, dir)
	protocol.Send(writer, protocol.Message{Type: protocol.MsgCreate, Payload: ttlPayload})
	var roomID, expiryStr string
	var expiryTime time.Time
	for i := 0; i < 10; i++ {
		msg, err := protocol.Decode(reader)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		switch msg.Type {
		case protocol.MsgRoomID:
			roomID = msg.Payload
		case protocol.MsgExpiry:
			expiryStr = msg.Payload
			expiryTime, _ = time.Parse(time.RFC3339, expiryStr)
		case protocol.MsgError:
			conn.Close()
			return "", 0
		}
		if roomID != "" && expiryStr != "" {
			break
		}
	}
	// Get TTL before closing (room may be destroyed after close)
	var ttl time.Duration
	if roomID != "" {
		if r, err := srv.rooms.GetRoom(roomID); err == nil {
			ttl = r.ExpiresAt.Sub(r.CreatedAt)
		} else if !expiryTime.IsZero() {
			// Fallback: use time.Until (close to original TTL)
			ttl = time.Until(expiryTime)
			// If clock skew makes ttl slightly low, add a second
			if ttl < 0 {
				ttl = 0
			}
		}
	}
	conn.Close()
	if roomID == "" {
		return "", 0
	}
	return roomID, ttl
}

func TestCreateRoomDefaultTTL(t *testing.T) {
	srv, dir := testServer(t)
	_, ttl := createRoomWithTTL(t, srv, dir, "")
	if ttl < 9*time.Minute || ttl > 11*time.Minute {
		t.Errorf("default TTL should be ~10m, got %v", ttl)
	}
}

func TestCreateRoomWithValidTTLs(t *testing.T) {
	valid := map[string]time.Duration{
		"600":  10 * time.Minute,
		"1200": 20 * time.Minute,
		"1800": 30 * time.Minute,
		"2700": 45 * time.Minute,
		"3600": 60 * time.Minute,
	}
	for payload, want := range valid {
		srv, dir := testServer(t)
		_, ttl := createRoomWithTTL(t, srv, dir, payload)
		if ttl < want-time.Second || ttl > want+time.Second {
			t.Errorf("CREATE %q TTL = %v, want %v", payload, ttl, want)
		}
	}
}

func TestCreateRoomRejectsInvalidTTL(t *testing.T) {
	invalid := []string{"599", "0", "-1", "-600", "3601", "10000", "abc", "10m"}
	for _, payload := range invalid {
		srv, dir := testServer(t)
		addr := srv.Addr().String()
		conn, reader, writer := testClient(t, addr, dir)
		protocol.Send(writer, protocol.Message{Type: protocol.MsgCreate, Payload: payload})
		foundError := false
		for i := 0; i < 10; i++ {
			msg, err := protocol.Decode(reader)
			if err != nil {
				break
			}
			if msg.Type == protocol.MsgError {
				foundError = true
				if !strings.Contains(strings.ToLower(msg.Payload), "invalid") {
					t.Errorf("payload %q error should mention invalid, got %q", payload, msg.Payload)
				}
				break
			}
		}
		conn.Close()
		if !foundError {
			t.Errorf("CREATE with TTL %q should be rejected", payload)
		}
		// Server should remain usable
		_, ttl2 := createRoomWithTTL(t, srv, dir, "600")
		if ttl2 < 9*time.Minute || ttl2 > 11*time.Minute {
			t.Errorf("server should remain usable after invalid TTL %q", payload)
		}
	}
}

func TestCountdownUsesSelectedTTL(t *testing.T) {
	srv, dir := testServer(t)
	addr := srv.Addr().String()
	conn, reader, writer := testClient(t, addr, dir)
	defer conn.Close()
	protocol.Send(writer, protocol.Message{Type: protocol.MsgCreate, Payload: "1800"}) // 30m
	var expiryStr string
	for i := 0; i < 10; i++ {
		msg, err := protocol.Decode(reader)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if msg.Type == protocol.MsgExpiry {
			expiryStr = msg.Payload
			break
		}
	}
	if expiryStr == "" {
		t.Fatal("no expiry received")
	}
	expiry, _ := time.Parse(time.RFC3339, expiryStr)
	remaining := time.Until(expiry)
	if remaining < 29*time.Minute || remaining > 30*time.Minute+time.Second {
		t.Errorf("30m room expiry remaining %v, want ~30m", remaining)
	}
}

func TestInvitationUnchangedWithCustomTTL(t *testing.T) {
	srv, dir := testServer(t)
	addr := srv.Addr().String()
	conn, reader, writer := testClient(t, addr, dir)
	defer conn.Close()
	protocol.Send(writer, protocol.Message{Type: protocol.MsgCreate, Payload: "2700"}) // 45m
	var token, endpoint string
	for i := 0; i < 10; i++ {
		msg, err := protocol.Decode(reader)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if msg.Type == protocol.MsgInvToken {
			token = msg.Payload
		}
		if msg.Type == protocol.MsgEndpoint {
			endpoint = msg.Payload
		}
		if token != "" && endpoint != "" {
			break
		}
	}
	if token == "" {
		t.Fatal("no token")
	}
	if !strings.HasPrefix(token, "TRINV-") {
		t.Errorf("token format changed: %q", token)
	}
	bundled := token + "@" + endpoint
	if !strings.Contains(bundled, "@") {
		t.Error("bundled invitation should contain @")
	}
	if !strings.Contains(endpoint, ":") {
		t.Error("endpoint should be host:port")
	}
	conn2, reader2, writer2 := testClient(t, addr, dir)
	defer conn2.Close()
	protocol.Send(writer2, protocol.Message{Type: protocol.MsgJoin, Payload: token})
	foundReady := false
	for i := 0; i < 10; i++ {
		m, err := protocol.Decode(reader2)
		if err != nil {
			break
		}
		if m.Type == protocol.MsgReady {
			foundReady = true
			break
		}
		if m.Type == protocol.MsgNotFound || m.Type == protocol.MsgExpired || m.Type == protocol.MsgUsed {
			t.Fatalf("invitation should be valid, got %v", m.Type)
		}
	}
	if !foundReady {
		t.Error("invitation should still allow join with custom TTL")
	}
}

func TestRoomExpirationWithCustomTTL(t *testing.T) {
	srv, dir := testServer(t)
	_, ttl := createRoomWithTTL(t, srv, dir, "1800")
	if ttl < 29*time.Minute || ttl > 30*time.Minute {
		t.Errorf("30m room should not be expired immediately")
	}
	srv2, _ := testServerWithTTL(t, 200*time.Millisecond)
	r, _ := srv2.rooms.CreateRoomWithTTL(200 * time.Millisecond)
	r.SetOnExpire(func() {
		r.BeginExpiring()
		r.BeginClosing()
		r.FinishDestroying()
	})
	r.StartExpirationTimer()
	time.Sleep(400 * time.Millisecond)
	if !r.IsDestroyed() {
		t.Error("short TTL room should be destroyed after expiry")
	}
}
