package room

import (
	"sync"
	"testing"
	"time"

	"github.com/terminalroom/terminalroom/internal/protocol"
)

func TestGenerateRoomID(t *testing.T) {
	id, err := GenerateRoomID()
	if err != nil {
		t.Fatalf("GenerateRoomID() error = %v", err)
	}

	if len(id) == 0 {
		t.Fatal("GenerateRoomID() returned empty ID")
	}

	if len(id) != len(RoomIDPrefix)+RoomIDLength {
		t.Fatalf("GenerateRoomID() returned ID of unexpected length: %d", len(id))
	}
}

func TestRoomIDUniqueness(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id, err := GenerateRoomID()
		if err != nil {
			t.Fatalf("GenerateRoomID() error = %v", err)
		}
		if ids[id] {
			t.Fatalf("GenerateRoomID() produced duplicate ID: %s", id)
		}
		ids[id] = true
	}
}

func TestNewRoom(t *testing.T) {
	r := NewRoom("TR-TEST1")

	if r.ID != "TR-TEST1" {
		t.Errorf("NewRoom() ID = %s, want TR-TEST1", r.ID)
	}

	if r.State != StateWaiting {
		t.Errorf("NewRoom() State = %v, want StateWaiting", r.State)
	}

	if r.ParticipantCount() != 0 {
		t.Errorf("NewRoom() ParticipantCount() = %d, want 0", r.ParticipantCount())
	}

	if r.TimeRemaining() <= 0 {
		t.Error("NewRoom() should have positive TTL")
	}
}

func TestNewRoomWithTTL(t *testing.T) {
	r := NewRoomWithTTL("TR-TEST1", 5*time.Second)

	if r.TimeRemaining() > 5*time.Second {
		t.Error("NewRoomWithTTL() TTL should not exceed specified duration")
	}
}

func TestRoomJoin(t *testing.T) {
	r := NewRoom("TR-TEST1")

	p, err := r.Join("user1")
	if err != nil {
		t.Fatalf("Join() error = %v", err)
	}

	if p.ID != 1 {
		t.Errorf("Join() participant ID = %d, want 1", p.ID)
	}

	if r.ParticipantCount() != 1 {
		t.Errorf("Join() ParticipantCount() = %d, want 1", r.ParticipantCount())
	}
}

func TestRoomJoinFull(t *testing.T) {
	r := NewRoom("TR-TEST1")
	r.Join("user1")
	r.Join("user2")

	_, err := r.Join("user3")
	if err == nil {
		t.Fatal("Join() should have failed for full room")
	}

	if err.Error() != "room is full" {
		t.Errorf("Join() error = %v, want 'room is full'", err)
	}
}

func TestRoomJoinDestroyed(t *testing.T) {
	r := NewRoom("TR-TEST1")
	r.Leave(1)

	_, err := r.Join("user2")
	if err == nil {
		t.Fatal("Join() should have failed for destroyed room")
	}

	if err.Error() != "room is not available" {
		t.Errorf("Join() error = %v, want 'room is not available'", err)
	}
}

func TestRoomJoinExpiring(t *testing.T) {
	r := NewRoom("TR-TEST1")
	r.Join("user1")

	r.BeginExpiring()

	_, err := r.Join("user2")
	if err == nil {
		t.Fatal("Join() should have failed for expiring room")
	}

	if err.Error() != "room is not available" {
		t.Errorf("Join() error = %v, want 'room is not available'", err)
	}
}

func TestRoomLeave(t *testing.T) {
	r := NewRoom("TR-TEST1")
	p, _ := r.Join("user1")

	r.Leave(p.ID)

	if r.ParticipantCount() != 0 {
		t.Errorf("Leave() ParticipantCount() = %d, want 0", r.ParticipantCount())
	}

	if !r.IsDestroyed() {
		t.Error("Leave() room should be destroyed when empty")
	}
}

func TestRoomLeaveDoesNotDestroyClosing(t *testing.T) {
	r := NewRoom("TR-TEST1")
	p1, _ := r.Join("user1")
	p2, _ := r.Join("user2")

	r.BeginExpiring()
	r.BeginClosing()

	r.Leave(p1.ID)
	if r.IsDestroyed() {
		t.Error("Leave() should not destroy room in closing state")
	}

	r.Leave(p2.ID)
	if r.IsDestroyed() {
		t.Error("Leave() should not destroy room in closing state")
	}
}

func TestRoomActive(t *testing.T) {
	r := NewRoom("TR-TEST1")
	r.Join("user1")

	if r.IsActive() {
		t.Error("Room with 1 participant should not be active")
	}

	r.Join("user2")

	if !r.IsActive() {
		t.Error("Room with 2 participants should be active")
	}
}

func TestRoomOtherParticipant(t *testing.T) {
	r := NewRoom("TR-TEST1")
	p1, _ := r.Join("user1")
	p2, _ := r.Join("user2")

	other := r.OtherParticipant(p1.ID)
	if other == nil || other.ID != p2.ID {
		t.Error("OtherParticipant() returned wrong participant")
	}

	other = r.OtherParticipant(p2.ID)
	if other == nil || other.ID != p1.ID {
		t.Error("OtherParticipant() returned wrong participant")
	}
}

func TestBroadcastExcept(t *testing.T) {
	r := NewRoom("TR-TEST1")
	p1, _ := r.Join("user1")
	p2, _ := r.Join("user2")

	ch1 := r.RegisterWriter(p1.ID)
	ch2 := r.RegisterWriter(p2.ID)
	defer r.UnregisterWriter(p1.ID)
	defer r.UnregisterWriter(p2.ID)

	r.BroadcastExcept(p1.ID, protocol.Message{
		Type:    protocol.MsgChat,
		Payload: "from-p1",
	})

	select {
	case msg := <-ch1:
		t.Errorf("Sender should not receive own message, got: %s", msg.Payload)
	case <-time.After(100 * time.Millisecond):
	}

	select {
	case msg := <-ch2:
		if msg.Payload != "from-p1" {
			t.Errorf("Receiver got wrong payload: %s, want 'from-p1'", msg.Payload)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("TestBroadcastExcept timed out waiting for receiver")
	}
}

func TestBroadcastAll(t *testing.T) {
	r := NewRoom("TR-TEST1")
	p1, _ := r.Join("user1")
	p2, _ := r.Join("user2")

	ch1 := r.RegisterWriter(p1.ID)
	ch2 := r.RegisterWriter(p2.ID)
	defer r.UnregisterWriter(p1.ID)
	defer r.UnregisterWriter(p2.ID)

	r.BroadcastAll(protocol.Message{
		Type:    protocol.MsgSystem,
		Payload: "system message",
	})

	for _, ch := range []chan protocol.Message{ch1, ch2} {
		select {
		case msg := <-ch:
			if msg.Payload != "system message" {
				t.Errorf("BroadcastAll wrong payload: %s", msg.Payload)
			}
		case <-time.After(1 * time.Second):
			t.Fatal("TestBroadcastAll timed out")
		}
	}
}

func TestRoomIsolation(t *testing.T) {
	r1 := NewRoom("TR-ROOM1")
	r2 := NewRoom("TR-ROOM2")

	p1, _ := r1.Join("user1")
	p2, _ := r2.Join("user2")

	ch1 := r1.RegisterWriter(p1.ID)
	ch2 := r2.RegisterWriter(p2.ID)
	defer r1.UnregisterWriter(p1.ID)
	defer r2.UnregisterWriter(p2.ID)

	r1.BroadcastExcept(0, protocol.Message{
		Type:    protocol.MsgChat,
		Payload: "message1",
	})
	r2.BroadcastExcept(0, protocol.Message{
		Type:    protocol.MsgChat,
		Payload: "message2",
	})

	msg1 := <-ch1
	msg2 := <-ch2

	if msg1.Payload == msg2.Payload {
		t.Error("Rooms should be isolated")
	}
	if msg1.Payload != "message1" {
		t.Errorf("Room 1 got wrong message: %s", msg1.Payload)
	}
	if msg2.Payload != "message2" {
		t.Errorf("Room 2 got wrong message: %s", msg2.Payload)
	}
}

func TestBroadcastExceptDestroyedRoom(t *testing.T) {
	r := NewRoom("TR-TEST1")
	p1, _ := r.Join("user1")
	ch1 := r.RegisterWriter(p1.ID)
	defer r.UnregisterWriter(p1.ID)

	r.Leave(p1.ID)

	r.BroadcastExcept(0, protocol.Message{
		Type:    protocol.MsgChat,
		Payload: "should not arrive",
	})

	select {
	case msg := <-ch1:
		t.Errorf("Should not receive message in destroyed room, got: %s", msg.Payload)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRegisterWriterIdempotent(t *testing.T) {
	r := NewRoom("TR-TEST1")
	p1, _ := r.Join("user1")

	ch1 := r.RegisterWriter(p1.ID)
	ch2 := r.RegisterWriter(p1.ID)

	if ch1 == ch2 {
		t.Error("RegisterWriter should return new channel on re-registration")
	}

	// Ownership: Room never closes channel on re-registration; it just replaces the entry.
	// Old channel stays open (owner closes), new channel is the one in the map.
	select {
	case <-ch1:
		t.Error("First channel should NOT be closed by Room on re-registration (owner closes after delete)")
	default:
		// expected: not closed
	}
	// Verify broadcast goes to new channel, not old
	r.BroadcastAll(protocol.Message{Type: protocol.MsgSystem, Payload: "test"})
	select {
	case msg := <-ch2:
		if msg.Payload != "test" {
			t.Errorf("new channel should receive broadcast, got %q", msg.Payload)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("new channel should receive broadcast")
	}
	select {
	case <-ch1:
		t.Error("old channel should not receive broadcast after re-registration")
	default:
	}
	// Cleanup: owner closes old channel
	close(ch1)
}

func TestConcurrentJoin(t *testing.T) {
	r := NewRoom("TR-TEST1")

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.Join("user")
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if successCount != MaxCapacity {
		t.Errorf("Concurrent Join() succeeded %d times, want %d", successCount, MaxCapacity)
	}
}

func TestConcurrentFinalSlot(t *testing.T) {
	r := NewRoom("TR-TEST1")
	r.Join("user1")

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.Join("user")
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if successCount != 1 {
		t.Errorf("Concurrent final slot Join() succeeded %d times, want 1", successCount)
	}
}

func TestStateTransitions(t *testing.T) {
	r := NewRoom("TR-TEST1")

	if r.State != StateWaiting {
		t.Errorf("Initial state = %v, want StateWaiting", r.State)
	}

	r.Join("user1")
	if r.State != StateWaiting {
		t.Errorf("After 1 join = %v, want StateWaiting", r.State)
	}

	r.Join("user2")
	if r.State != StateActive {
		t.Errorf("After 2 joins = %v, want StateActive", r.State)
	}

	if !r.BeginExpiring() {
		t.Error("BeginExpiring() should succeed from Active")
	}
	if r.State != StateExpiring {
		t.Errorf("After BeginExpiring = %v, want StateExpiring", r.State)
	}

	if !r.BeginClosing() {
		t.Error("BeginClosing() should succeed from Expiring")
	}
	if r.State != StateClosing {
		t.Errorf("After BeginClosing = %v, want StateClosing", r.State)
	}

	r.FinishDestroying()
	if r.State != StateDestroyed {
		t.Errorf("After FinishDestroying = %v, want StateDestroyed", r.State)
	}
}

func TestInvalidStateTransitions(t *testing.T) {
	r := NewRoom("TR-TEST1")
	r.Join("user1")
	r.Join("user2")

	if r.BeginExpiring() {
		r.FinishDestroying()
	}

	r2 := NewRoom("TR-TEST2")
	if r2.BeginClosing() {
		t.Error("BeginClosing() should fail from Waiting")
	}
	r2.FinishDestroying()

	r3 := NewRoom("TR-TEST3")
	if r3.BeginExpiring() {
		r3.BeginClosing()
	}
	r3.FinishDestroying()

	if r3.BeginExpiring() {
		t.Error("BeginExpiring() should fail from Destroyed")
	}
}

func TestExpirationTimer(t *testing.T) {
	r := NewRoomWithTTL("TR-TEST1", 100*time.Millisecond)

	expired := make(chan struct{})
	r.SetOnExpire(func() {
		close(expired)
	})

	r.StartExpirationTimer()

	select {
	case <-expired:
	case <-time.After(1 * time.Second):
		t.Fatal("Expiration timer did not fire")
	}

	if r.State != StateWaiting {
		t.Errorf("State after timer = %v, want StateWaiting (server handles transition)", r.State)
	}
}

func TestExpirationTimerStoppedOnDestroy(t *testing.T) {
	r := NewRoomWithTTL("TR-TEST1", 100*time.Millisecond)

	expired := make(chan struct{})
	r.SetOnExpire(func() {
		close(expired)
	})

	r.StartExpirationTimer()
	r.StopExpirationTimer()

	select {
	case <-expired:
		t.Error("Expiration timer should not fire after stop")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestTimeRemaining(t *testing.T) {
	r := NewRoomWithTTL("TR-TEST1", 5*time.Second)

	remaining := r.TimeRemaining()
	if remaining <= 0 || remaining > 5*time.Second {
		t.Errorf("TimeRemaining() = %v, want 0-5s", remaining)
	}

	r.FinishDestroying()
	if r.TimeRemaining() != 0 {
		t.Errorf("TimeRemaining() after destroy = %v, want 0", r.TimeRemaining())
	}
}

func TestBroadcastExceptDuringClosing(t *testing.T) {
	r := NewRoom("TR-TEST1")
	p1, _ := r.Join("user1")
	ch1 := r.RegisterWriter(p1.ID)
	defer r.UnregisterWriter(p1.ID)

	r.BeginExpiring()
	r.BeginClosing()

	r.BroadcastExcept(0, protocol.Message{
		Type:    protocol.MsgChat,
		Payload: "should not arrive",
	})

	select {
	case msg := <-ch1:
		t.Errorf("Should not receive message during closing, got: %s", msg.Payload)
	case <-time.After(100 * time.Millisecond):
	}
}
