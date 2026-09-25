package server

import (
	"crypto/tls"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/terminalroom/terminalroom/internal/protocol"
	"github.com/terminalroom/terminalroom/internal/tlsutil"
)

func rawTLSConn(t *testing.T, addr, certDir string) net.Conn {
	t.Helper()
	cfg, err := tlsutil.ClientConfig(certDir)
	if err != nil {
		t.Fatalf("client config: %v", err)
	}
	conn, err := tls.Dial("tcp", addr, cfg)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return conn
}

func TestOversizedFrameDoesNotCrashServer(t *testing.T) {
	srv, dir := testServer(t)
	addr := srv.Addr().String()

	// Attacker sends oversized line directly (bypass Send validation)
	attacker := rawTLSConn(t, addr, dir)
	oversized := strings.Repeat("A", protocol.MaxLineSize+100) + "\n"
	attacker.Write([]byte(oversized))
	// Give server time to handle
	time.Sleep(100 * time.Millisecond)
	attacker.Close()

	// Server should still be usable
	conn, reader, writer := testClient(t, addr, dir)
	defer conn.Close()
	// Try CREATE
	protocol.Send(writer, protocol.Message{Type: protocol.MsgCreate})
	msg, err := protocol.Decode(reader)
	if err != nil || msg.Type != protocol.MsgRoomID {
		t.Fatalf("server should remain usable after oversized: %v %v", msg, err)
	}
}

func TestOversizedDoesNotAffectOtherRoom(t *testing.T) {
	srv, dir := testServer(t)
	addr := srv.Addr().String()

	// Create legitimate room via client API
	conn1, reader1, writer1 := testClient(t, addr, dir)
	defer conn1.Close()
	protocol.Send(writer1, protocol.Message{Type: protocol.MsgCreate})
	var roomID1 string
	for i := 0; i < 5; i++ {
		m, err := protocol.Decode(reader1)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if m.Type == protocol.MsgRoomID {
			roomID1 = m.Payload
			break
		}
	}
	if roomID1 == "" {
		t.Fatal("no roomID")
	}
	// Drain remaining for room1 creation
	for i := 0; i < 4; i++ {
		protocol.Decode(reader1)
	}

	// Attacker oversized on separate connection
	attacker := rawTLSConn(t, addr, dir)
	oversizedPayload := strings.Repeat("x", protocol.MaxPayloadSize+10)
	oversizedLine := "CHAT|" + oversizedPayload + "\n"
	attacker.Write([]byte(oversizedLine))
	time.Sleep(100 * time.Millisecond)
	attacker.Close()

	// Second legitimate room should still be creatable and isolated
	conn2, reader2, writer2 := testClient(t, addr, dir)
	defer conn2.Close()
	protocol.Send(writer2, protocol.Message{Type: protocol.MsgCreate})
	msg, err := protocol.Decode(reader2)
	if err != nil || msg.Type != protocol.MsgRoomID {
		t.Fatalf("second room should be creatable after oversized: %v %v", msg, err)
	}
	roomID2 := msg.Payload
	if roomID1 == roomID2 {
		t.Error("room IDs should differ (isolation)")
	}
	// Verify first room still exists
	if _, err := srv.rooms.GetRoom(roomID1); err != nil {
		t.Errorf("first room should remain after attacker: %v", err)
	}
	if _, err := srv.rooms.GetRoom(roomID2); err != nil {
		t.Errorf("second room missing: %v", err)
	}
}

func TestServerRejectsOversizedChatPayload(t *testing.T) {
	srv, dir := testServer(t)
	addr := srv.Addr().String()
	conn, reader, writer := testClient(t, addr, dir)
	defer conn.Close()
	// Create room
	protocol.Send(writer, protocol.Message{Type: protocol.MsgCreate})
	// Consume initial messages
	for i := 0; i < 5; i++ {
		protocol.Decode(reader)
	}
	// Try to send oversized chat via raw (bypass client Send validation)
	// This simulates malicious client bypassing Send
	oversizedChat := "CHAT|" + strings.Repeat("y", protocol.MaxPayloadSize+1) + "\n"
	// Direct write to tls conn (need to use raw conn, not writer)
	// Use the same conn's writer? We have tls conn, we can write raw
	conn.Write([]byte(oversizedChat))
	// Server's handleParticipant should detect oversized via Decode and close that participant
	time.Sleep(100 * time.Millisecond)
	// Server should not have crashed; new room still possible
	conn2, reader2, writer2 := testClient(t, addr, dir)
	defer conn2.Close()
	protocol.Send(writer2, protocol.Message{Type: protocol.MsgCreate})
	m, err := protocol.Decode(reader2)
	if err != nil || m.Type != protocol.MsgRoomID {
		t.Fatalf("server should handle oversized chat without crash: %v %v", m, err)
	}
}
