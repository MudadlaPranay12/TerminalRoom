package room

import (
	"sync"
	"testing"

	"github.com/terminalroom/terminalroom/internal/protocol"
)

// helper to create room with two participants and writers
func setupRoomWithWriters(t *testing.T) (*Room, int, int, chan protocol.Message, chan protocol.Message) {
	t.Helper()
	r := NewRoom("TR-TEST")
	p1, _ := r.Join("user1")
	p2, _ := r.Join("user2")
	ch1 := r.RegisterWriter(p1.ID)
	ch2 := r.RegisterWriter(p2.ID)
	return r, p1.ID, p2.ID, ch1, ch2
}

func TestBroadcastUnregisterConcurrent(t *testing.T) {
	r, id1, id2, _, _ := setupRoomWithWriters(t)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			r.BroadcastExcept(id1, protocol.Message{Type: protocol.MsgChat, Payload: "hello"})
		}()
		go func() {
			defer wg.Done()
			// Unregister and re-register to stress close/delete race
			r.UnregisterWriter(id2)
			// Immediately re-register; owner would close, but new channel
			_ = r.RegisterWriter(id2)
		}()
	}
	wg.Wait()
	// Should not panic, should not have send-on-closed
}

func TestBroadcastDestroyConcurrent(t *testing.T) {
	for iter := 0; iter < 20; iter++ {
		r := NewRoom("TR-DESTROY")
		p1, _ := r.Join("user1")
		p2, _ := r.Join("user2")
		_ = r.RegisterWriter(p1.ID)
		_ = r.RegisterWriter(p2.ID)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				r.BroadcastAll(protocol.Message{Type: protocol.MsgChat, Payload: "msg"})
			}
		}()
		go func() {
			defer wg.Done()
			r.FinishDestroying()
		}()
		wg.Wait()
		// After destroy, broadcasts should be no-ops, not panic
		r.BroadcastAll(protocol.Message{Type: protocol.MsgChat, Payload: "after"})
	}
}

func TestConcurrentBroadcasts(t *testing.T) {
	r, id1, _, _, _ := setupRoomWithWriters(t)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			r.BroadcastExcept(id1, protocol.Message{Type: protocol.MsgChat, Payload: "a"})
		}()
		go func() {
			defer wg.Done()
			r.BroadcastAll(protocol.Message{Type: protocol.MsgSystem, Payload: "sys"})
		}()
	}
	wg.Wait()
}

func TestConcurrentUnregister(t *testing.T) {
	r := NewRoom("TR-UNREG")
	p1, _ := r.Join("user1")
	p2, _ := r.Join("user2")
	_ = r.RegisterWriter(p1.ID)
	_ = r.RegisterWriter(p2.ID)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); r.UnregisterWriter(p1.ID); _ = r.RegisterWriter(p1.ID) }()
		go func() { defer wg.Done(); r.UnregisterWriter(p2.ID); _ = r.RegisterWriter(p2.ID) }()
	}
	wg.Wait()
}

func TestRegisterWriterBroadcastConcurrent(t *testing.T) {
	r := NewRoom("TR-REG")
	p1, _ := r.Join("user1")
	_ = r.RegisterWriter(p1.ID)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			ch := r.RegisterWriter(p1.ID)
			// Drain new channel briefly
			select {
			case <-ch:
			default:
			}
			_ = n
		}(i)
		go func() {
			defer wg.Done()
			r.BroadcastAll(protocol.Message{Type: protocol.MsgChat, Payload: "concurrent"})
		}()
	}
	wg.Wait()
}

func TestBroadcastWithExpirationDestroy(t *testing.T) {
	r := NewRoom("TR-EXP")
	p1, _ := r.Join("user1")
	p2, _ := r.Join("user2")
	_ = r.RegisterWriter(p1.ID)
	_ = r.RegisterWriter(p2.ID)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			r.BroadcastExcept(p1.ID, protocol.Message{Type: protocol.MsgChat, Payload: "chat"})
		}
	}()
	go func() {
		defer wg.Done()
		// Simulate expiration path
		r.BeginExpiring()
		r.BeginClosing()
		r.FinishDestroying()
	}()
	wg.Wait()
}

func TestRepeatedBroadcastDestroyCycles(t *testing.T) {
	for cycle := 0; cycle < 20; cycle++ {
		r := NewRoom("TR-CYCLE")
		p1, _ := r.Join("user1")
		_ = r.RegisterWriter(p1.ID)
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r.BroadcastAll(protocol.Message{Type: protocol.MsgChat, Payload: "x"})
			}()
		}
		wg.Wait()
		r.FinishDestroying()
		// After destroy, new room same ID should be isolatable
		r2 := NewRoom("TR-CYCLE")
		p2, _ := r2.Join("user1")
		ch2 := r2.RegisterWriter(p2.ID)
		r2.BroadcastAll(protocol.Message{Type: protocol.MsgChat, Payload: "y"})
		select {
		case <-ch2:
		default:
		}
		_ = p2
		// Ensure old room's broadcasts don't affect new room
		if r.IsDestroyed() == false {
			t.Error("old room should be destroyed")
		}
	}
}
