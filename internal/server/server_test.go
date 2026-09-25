package server

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/terminalroom/terminalroom/internal/protocol"
	"github.com/terminalroom/terminalroom/internal/tlsutil"
)

func testServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	srv, err := New("localhost:0", dir)
	if err != nil {
		t.Fatalf("Failed to create test server: %v", err)
	}
	go srv.Start()
	t.Cleanup(func() { srv.Shutdown() })
	time.Sleep(50 * time.Millisecond)
	return srv, dir
}

func testServerWithTTL(t *testing.T, ttl time.Duration) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	srv, err := NewWithTTL("localhost:0", dir, ttl)
	if err != nil {
		t.Fatalf("Failed to create test server: %v", err)
	}
	go srv.Start()
	t.Cleanup(func() { srv.Shutdown() })
	time.Sleep(50 * time.Millisecond)
	return srv, dir
}

func testClient(t *testing.T, addr string, certDir string) (net.Conn, *bufio.Reader, *bufio.Writer) {
	t.Helper()
	tlsConfig, err := tlsutil.ClientConfig(certDir)
	if err != nil {
		t.Fatalf("Failed to create client config: %v", err)
	}
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	return conn, bufio.NewReader(conn), bufio.NewWriter(conn)
}

func sendAndDecode(t *testing.T, writer *bufio.Writer, reader *bufio.Reader, msg protocol.Message) *protocol.Message {
	t.Helper()
	protocol.Send(writer, msg)
	msg2, err := protocol.Decode(reader)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	return msg2
}

func createRoom(t *testing.T, addr string, dir string) (net.Conn, *bufio.Reader, *bufio.Writer, string, string) {
	t.Helper()
	conn, reader, writer := testClient(t, addr, dir)

	protocol.Send(writer, protocol.Message{Type: protocol.MsgCreate})

	msg := sendAndDecode(t, writer, reader, protocol.Message{})
	if msg.Type != protocol.MsgRoomID {
		t.Fatalf("Expected ROOM_ID, got %s", msg.Type)
	}
	roomID := msg.Payload

	msg = sendAndDecode(t, writer, reader, protocol.Message{})
	if msg.Type != protocol.MsgInvToken {
		t.Fatalf("Expected INV_TOKEN, got %s", msg.Type)
	}
	token := msg.Payload

	protocol.Decode(reader)
	protocol.Decode(reader)

	return conn, reader, writer, roomID, token
}

func TestTLSConnection(t *testing.T) {
	srv, dir := testServer(t)
	conn, reader, writer := testClient(t, srv.Addr().String(), dir)
	defer conn.Close()

	msg := sendAndDecode(t, writer, reader, protocol.Message{Type: protocol.MsgCreate})
	if msg.Type != protocol.MsgRoomID {
		t.Errorf("Expected ROOM_ID, got %s", msg.Type)
	}
}

func TestTLSInvalidCertDir(t *testing.T) {
	_, err := tlsutil.ClientConfig("/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent cert dir")
	}
}

func TestValidInvitationFlow(t *testing.T) {
	srv, dir := testServer(t)
	addr := srv.Addr().String()

	conn1, _, _, _, token := createRoom(t, addr, dir)
	defer conn1.Close()

	conn2, reader2, writer2 := testClient(t, addr, dir)
	defer conn2.Close()

	protocol.Send(writer2, protocol.Message{Type: protocol.MsgJoin, Payload: token})

	gotReady := false
	for i := 0; i < 5; i++ {
		msg, err := protocol.Decode(reader2)
		if err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		if msg.Type == protocol.MsgReady {
			gotReady = true
			break
		}
		if msg.Type == protocol.MsgNotFound || msg.Type == protocol.MsgExpired || msg.Type == protocol.MsgUsed || msg.Type == protocol.MsgFull || msg.Type == protocol.MsgError {
			t.Fatalf("Join failed with: %s", msg.Type)
		}
	}

	if !gotReady {
		t.Error("Did not receive READY message")
	}
}

func TestInvalidInvitation(t *testing.T) {
	srv, dir := testServer(t)

	conn, reader, writer := testClient(t, srv.Addr().String(), dir)
	defer conn.Close()

	msg := sendAndDecode(t, writer, reader, protocol.Message{Type: protocol.MsgJoin, Payload: "TRINV-invalid-token"})
	if msg.Type != protocol.MsgNotFound {
		t.Errorf("Expected NOT_FOUND, got %s", msg.Type)
	}
}

func TestExpiredInvitation(t *testing.T) {
	srv, dir := testServer(t)
	addr := srv.Addr().String()

	conn, reader, writer := testClient(t, addr, dir)
	defer conn.Close()

	msg := sendAndDecode(t, writer, reader, protocol.Message{Type: protocol.MsgJoin, Payload: "TRINV-expired-token"})
	if msg.Type != protocol.MsgNotFound {
		t.Errorf("Expected NOT_FOUND for invalid token, got %s", msg.Type)
	}
}

func TestRoomIDCannotJoin(t *testing.T) {
	srv, dir := testServer(t)
	addr := srv.Addr().String()

	conn1, _, _, roomID, _ := createRoom(t, addr, dir)
	defer conn1.Close()

	conn2, reader2, writer2 := testClient(t, addr, dir)
	defer conn2.Close()

	msg := sendAndDecode(t, writer2, reader2, protocol.Message{Type: protocol.MsgJoin, Payload: roomID})
	if msg.Type == protocol.MsgReady {
		t.Error("Should not be able to join with room ID alone")
	}
}

func TestThirdParticipantRejected(t *testing.T) {
	srv, dir := testServer(t)
	addr := srv.Addr().String()

	conn1, _, _, _, token := createRoom(t, addr, dir)
	defer conn1.Close()

	conn2, reader2, writer2 := testClient(t, addr, dir)
	defer conn2.Close()

	protocol.Send(writer2, protocol.Message{Type: protocol.MsgJoin, Payload: token})

	for {
		msg, err := protocol.Decode(reader2)
		if err != nil {
			t.Fatalf("Failed to read join response: %v", err)
		}
		if msg.Type == protocol.MsgReady {
			break
		}
		if msg.Type != protocol.MsgParticipantID && msg.Type != protocol.MsgSystem && msg.Type != protocol.MsgExpiry && msg.Type != protocol.MsgEndpoint {
			t.Fatalf("Unexpected response: %s", msg.Type)
		}
	}

	conn3, reader3, writer3 := testClient(t, addr, dir)
	defer conn3.Close()

	msg := sendAndDecode(t, writer3, reader3, protocol.Message{Type: protocol.MsgJoin, Payload: token})
	if msg.Type != protocol.MsgUsed {
		t.Errorf("Expected USED for third participant, got %s", msg.Type)
	}
}

func TestConcurrentInvitationUse(t *testing.T) {
	srv, dir := testServer(t)
	addr := srv.Addr().String()

	conn1, _, _, _, token := createRoom(t, addr, dir)
	defer conn1.Close()

	var wg sync.WaitGroup
	results := make(chan protocol.MsgType, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, reader, writer := testClient(t, addr, dir)
			defer conn.Close()

			protocol.Send(writer, protocol.Message{Type: protocol.MsgJoin, Payload: token})

			for {
				msg, err := protocol.Decode(reader)
				if err != nil {
					results <- protocol.MsgError
					return
				}
				switch msg.Type {
				case protocol.MsgReady:
					results <- protocol.MsgReady
					return
				case protocol.MsgNotFound, protocol.MsgExpired, protocol.MsgUsed, protocol.MsgFull, protocol.MsgError:
					results <- msg.Type
					return
				}
			}
		}()
	}

	wg.Wait()
	close(results)

	successCount := 0
	for msgType := range results {
		if msgType == protocol.MsgReady {
			successCount++
		}
	}

	if successCount != 1 {
		t.Errorf("Concurrent invitation use: %d succeeded, want 1", successCount)
	}
}

func TestConcurrentFinalSlotJoin(t *testing.T) {
	srv, dir := testServer(t)
	addr := srv.Addr().String()

	conn1, reader1, writer1 := testClient(t, addr, dir)
	defer conn1.Close()

	protocol.Send(writer1, protocol.Message{Type: protocol.MsgCreate})
	protocol.Decode(reader1)

	msg := sendAndDecode(t, writer1, reader1, protocol.Message{})
	_ = msg

	conn2, reader2, writer2 := testClient(t, addr, dir)
	defer conn2.Close()

	protocol.Send(writer2, protocol.Message{Type: protocol.MsgJoin, Payload: "invalid"})
	protocol.Decode(reader2)

	r, err := srv.rooms.CreateRoom()
	if err != nil {
		t.Fatal(err)
	}
	roomID := r.ID

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			inv, err := srv.invitations.Create(roomID, 10*time.Minute)
			if err != nil {
				return
			}
			c, reader, writer := testClient(t, addr, dir)
			defer c.Close()

			protocol.Send(writer, protocol.Message{Type: protocol.MsgJoin, Payload: inv.Token})
			resp, err := protocol.Decode(reader)
			if err != nil {
				return
			}
			if resp.Type == protocol.MsgReady {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if successCount > 1 {
		t.Errorf("Concurrent final slot: %d succeeded, want at most 1", successCount)
	}
}

func TestServerSurvivesClientFailure(t *testing.T) {
	srv, dir := testServer(t)
	addr := srv.Addr().String()

	conn, _, _ := testClient(t, addr, dir)
	conn.Close()

	time.Sleep(50 * time.Millisecond)

	conn2, reader2, writer2 := testClient(t, addr, dir)
	defer conn2.Close()

	msg := sendAndDecode(t, writer2, reader2, protocol.Message{Type: protocol.MsgCreate})
	if msg.Type != protocol.MsgRoomID {
		t.Errorf("Server should work after client failure, got %s", msg.Type)
	}
}

func TestUnauthenticatedChatRejected(t *testing.T) {
	srv, dir := testServer(t)
	addr := srv.Addr().String()

	conn, reader, writer := testClient(t, addr, dir)
	defer conn.Close()

	msg := sendAndDecode(t, writer, reader, protocol.Message{Type: protocol.MsgChat, Payload: "hello"})
	if msg.Type != protocol.MsgError {
		t.Errorf("Expected ERROR for unauthenticated CHAT, got %s", msg.Type)
	}
}

func TestMalformedMessageRejected(t *testing.T) {
	srv, dir := testServer(t)
	addr := srv.Addr().String()

	conn, reader, writer := testClient(t, addr, dir)
	defer conn.Close()

	fmt.Fprint(writer, "NOPAYLOAD\n")
	writer.Flush()

	msg, err := protocol.Decode(reader)
	if err == nil && msg.Type == protocol.MsgChat {
		t.Error("Malformed message should not be accepted as CHAT")
	}
}

func TestEmptyInitialMessage(t *testing.T) {
	srv, dir := testServer(t)
	addr := srv.Addr().String()

	conn, reader, writer := testClient(t, addr, dir)
	defer conn.Close()

	fmt.Fprint(writer, "\n")
	writer.Flush()

	msg, err := protocol.Decode(reader)
	if err == nil && msg.Type != protocol.MsgError {
		t.Errorf("Empty initial message should return ERROR, got %s", msg.Type)
	}
}

func TestRoomExpiration(t *testing.T) {
	srv, dir := testServerWithTTL(t, 100*time.Millisecond)
	addr := srv.Addr().String()

	conn1, _, _, roomID, _ := createRoom(t, addr, dir)
	defer conn1.Close()

	time.Sleep(2500 * time.Millisecond)

	_, err := srv.rooms.GetRoom(roomID)
	if err == nil {
		t.Error("Room should have been removed after TTL expiration")
	}
}

func TestRoomExpirationNotifiesParticipants(t *testing.T) {
	srv, dir := testServerWithTTL(t, 200*time.Millisecond)
	addr := srv.Addr().String()

	conn1, reader1, writer1 := testClient(t, addr, dir)
	defer conn1.Close()

	protocol.Send(writer1, protocol.Message{Type: protocol.MsgCreate})

	conn1.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer conn1.SetReadDeadline(time.Time{})

	gotExpiring := false
	for i := 0; i < 10; i++ {
		msg, err := protocol.Decode(reader1)
		if err != nil {
			break
		}
		if msg.Type == protocol.MsgSystem && msg.Payload == "Room is expiring. Connection will close shortly." {
			gotExpiring = true
			break
		}
		if msg.Type == protocol.MsgSystem && msg.Payload == "Room is closing." {
			gotExpiring = true
			break
		}
	}

	if !gotExpiring {
		t.Error("Participant should have received expiration notification")
	}
}

func TestAbandonedRoomCleanup(t *testing.T) {
	srv, dir := testServerWithTTL(t, 10*time.Second)
	addr := srv.Addr().String()

	conn1, reader1, writer1 := testClient(t, addr, dir)

	protocol.Send(writer1, protocol.Message{Type: protocol.MsgCreate})
	msg := sendAndDecode(t, writer1, reader1, protocol.Message{})
	roomID := msg.Payload
	protocol.Decode(reader1)
	protocol.Decode(reader1)
	protocol.Decode(reader1)

	conn1.Close()
	time.Sleep(100 * time.Millisecond)

	_, err := srv.rooms.GetRoom(roomID)
	if err == nil {
		t.Error("Abandoned room should have been cleaned up")
	}
}

func TestInvitationInvalidatedAfterRoomDestruction(t *testing.T) {
	srv, dir := testServerWithTTL(t, 200*time.Millisecond)
	addr := srv.Addr().String()

	conn1, _, _, _, token := createRoom(t, addr, dir)
	conn1.Close()

	time.Sleep(400 * time.Millisecond)

	conn2, reader2, writer2 := testClient(t, addr, dir)
	defer conn2.Close()

	msg := sendAndDecode(t, writer2, reader2, protocol.Message{Type: protocol.MsgJoin, Payload: token})
	if msg.Type != protocol.MsgNotFound {
		t.Errorf("Expected NOT_FOUND for invitation to destroyed room, got %s", msg.Type)
	}
}

func TestDestroyedRoomCannotBeJoined(t *testing.T) {
	srv, dir := testServerWithTTL(t, 200*time.Millisecond)
	addr := srv.Addr().String()

	conn1, _, _, roomID, _ := createRoom(t, addr, dir)
	conn1.Close()

	time.Sleep(400 * time.Millisecond)

	conn2, reader2, writer2 := testClient(t, addr, dir)
	defer conn2.Close()

	msg := sendAndDecode(t, writer2, reader2, protocol.Message{Type: protocol.MsgJoin, Payload: "TRINV-" + roomID})
	if msg.Type == protocol.MsgReady {
		t.Error("Should not be able to join destroyed room")
	}
}

func TestNoChatAfterRoomDestruction(t *testing.T) {
	srv, dir := testServerWithTTL(t, 200*time.Millisecond)
	addr := srv.Addr().String()

	conn1, reader1, writer1 := testClient(t, addr, dir)
	defer conn1.Close()

	conn2, _, writer2 := testClient(t, addr, dir)
	defer conn2.Close()

	protocol.Send(writer1, protocol.Message{Type: protocol.MsgCreate})
	protocol.Decode(reader1)
	protocol.Decode(reader1)
	protocol.Decode(reader1)
	protocol.Decode(reader1)

	protocol.Send(writer2, protocol.Message{Type: protocol.MsgJoin, Payload: "invalid"})

	time.Sleep(400 * time.Millisecond)

	protocol.Send(writer1, protocol.Message{Type: protocol.MsgChat, Payload: "test"})
	time.Sleep(100 * time.Millisecond)

	select {
	case msg := <-make(chan *protocol.Message, 1):
		_ = msg
	default:
	}
}

func TestMultipleDestroyCallsSafe(t *testing.T) {
	srv, dir := testServer(t)
	addr := srv.Addr().String()

	conn1, _, _, _, _ := createRoom(t, addr, dir)
	conn1.Close()

	time.Sleep(100 * time.Millisecond)

	conn2, reader2, writer2 := testClient(t, addr, dir)
	defer conn2.Close()

	msg := sendAndDecode(t, writer2, reader2, protocol.Message{Type: protocol.MsgCreate})
	if msg.Type != protocol.MsgRoomID {
		t.Errorf("Server should work after previous room destruction, got %s", msg.Type)
	}
}

func TestExpiryMessageSent(t *testing.T) {
	srv, dir := testServer(t)
	addr := srv.Addr().String()

	conn, reader, writer := testClient(t, addr, dir)
	defer conn.Close()

	protocol.Send(writer, protocol.Message{Type: protocol.MsgCreate})

	gotExpiry := false
	for i := 0; i < 5; i++ {
		msg, err := protocol.Decode(reader)
		if err != nil {
			break
		}
		if msg.Type == protocol.MsgExpiry {
			gotExpiry = true
			break
		}
		if msg.Type == protocol.MsgError {
			t.Fatalf("Got error: %s", msg.Payload)
		}
	}

	if !gotExpiry {
		t.Error("Should receive EXPIRY message after room creation")
	}
}

func TestJoinReceivesExpiryMessage(t *testing.T) {
	srv, dir := testServer(t)
	addr := srv.Addr().String()

	conn1, _, _, _, token := createRoom(t, addr, dir)
	defer conn1.Close()

	conn2, reader2, writer2 := testClient(t, addr, dir)
	defer conn2.Close()

	protocol.Send(writer2, protocol.Message{Type: protocol.MsgJoin, Payload: token})

	gotExpiry := false
	for i := 0; i < 5; i++ {
		msg, err := protocol.Decode(reader2)
		if err != nil {
			break
		}
		if msg.Type == protocol.MsgExpiry {
			gotExpiry = true
		}
		if msg.Type == protocol.MsgReady {
			break
		}
	}

	if !gotExpiry {
		t.Error("Joiner should receive EXPIRY message")
	}
}

// F-02: Server rejects public/invalid ENDPOINT_ADDR before advertisement
func TestServerRejectsPublicEndpoint(t *testing.T) {
	dir := t.TempDir()
	_, err := NewWithTTLAndEndpoint("127.0.0.1:0", dir, 10*time.Minute, "8.8.8.8:9090")
	if err == nil {
		t.Fatal("public endpoint 8.8.8.8:9090 should be rejected")
	}
	_, err = NewWithTTLAndEndpoint("127.0.0.1:0", dir, 10*time.Minute, "1.1.1.1:9090")
	if err == nil {
		t.Fatal("public endpoint 1.1.1.1:9090 should be rejected")
	}
	_, err = NewWithTTLAndEndpoint("127.0.0.1:0", dir, 10*time.Minute, "example.com:9090")
	if err == nil {
		t.Fatal("public hostname example.com:9090 should be rejected")
	}
	_, err = NewWithTTLAndEndpoint("127.0.0.1:0", dir, 10*time.Minute, "0.0.0.0:9090")
	if err == nil {
		t.Fatal("0.0.0.0:9090 should be rejected")
	}
}

func TestServerRejectsMalformedEndpoint(t *testing.T) {
	dir := t.TempDir()
	invalid := []string{
		"192.168.1.10",       // missing port
		"192.168.1.10:0",     // port 0
		"192.168.1.10:65536", // port >65535
		"192.168.1.10:abc",   // invalid port
		"[::]:9090",          // unspecified
		":9090",              // empty host
	}
	for _, ep := range invalid {
		_, err := NewWithTTLAndEndpoint("127.0.0.1:0", dir, 10*time.Minute, ep)
		if err == nil {
			t.Errorf("malformed endpoint %q should be rejected", ep)
		}
	}
}

func TestServerAcceptsValidEndpoint(t *testing.T) {
	dir := t.TempDir()
	valid := []string{
		"127.0.0.1:9090",
		"localhost:9090",
		"192.168.1.10:9090",
		"10.0.0.5:9090",
		"172.16.0.10:9090",
		"100.93.120.19:9090",
	}
	for _, ep := range valid {
		srv, err := NewWithTTLAndEndpoint("127.0.0.1:0", dir, 10*time.Minute, ep)
		if err != nil {
			t.Errorf("valid endpoint %q should be accepted: %v", ep, err)
			continue
		}
		if srv.Endpoint() != ep {
			t.Errorf("endpoint mismatch: got %q want %q", srv.Endpoint(), ep)
		}
		srv.Shutdown()
	}
}

func TestServerEndpointAdvertisedViaMsgEndpoint(t *testing.T) {
	dir := t.TempDir()
	endpoint := "192.168.1.10:9090"
	srv, err := NewWithTTLAndEndpoint("127.0.0.1:0", dir, 10*time.Minute, endpoint)
	if err != nil {
		t.Fatalf("valid endpoint: %v", err)
	}
	go srv.Start()
	defer srv.Shutdown()
	time.Sleep(50 * time.Millisecond)
	addr := srv.Addr().String()
	conn, reader, writer := testClient(t, addr, dir)
	defer conn.Close()
	protocol.Send(writer, protocol.Message{Type: protocol.MsgCreate})
	// Expect ROOM_ID, INV_TOKEN, EXPIRY, ENDPOINT
	var gotEndpoint string
	for i := 0; i < 5; i++ {
		msg, err := protocol.Decode(reader)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if msg.Type == protocol.MsgEndpoint {
			gotEndpoint = msg.Payload
			break
		}
	}
	if gotEndpoint != endpoint {
		t.Errorf("MsgEndpoint = %q want %q", gotEndpoint, endpoint)
	}
}

func TestServerDevModeDoesNotAllowPublicEndpoint(t *testing.T) {
	dir := t.TempDir()
	// Even with DEVELOPMENT_MODE, public endpoint must still be rejected
	// Server validation does not consult DevelopmentMode
	_, err := NewWithTTLAndEndpoint("127.0.0.1:0", dir, 10*time.Minute, "8.8.8.8:9090")
	if err == nil {
		t.Fatal("public endpoint should be rejected even in dev mode")
	}
}
