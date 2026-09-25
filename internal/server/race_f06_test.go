package server

import (
	"sync"
	"testing"
	"time"

	"github.com/terminalroom/terminalroom/internal/protocol"
)

func TestServerShutdownWhileBroadcastF06(t *testing.T) {
	srv, _ := testServer(t)
	r, _ := srv.rooms.CreateRoom()
	_ = r.RegisterWriter(1)
	_ = r.RegisterWriter(2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			r.BroadcastAll(protocol.Message{Type: protocol.MsgChat, Payload: "stress"})
			r.BroadcastExcept(1, protocol.Message{Type: protocol.MsgChat, Payload: "except"})
		}
	}()
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond)
		srv.Shutdown()
	}()
	wg.Wait()
	srv.Shutdown()
}

func TestServerBroadcastDuringExpirationF06(t *testing.T) {
	srv, _ := testServerWithTTL(t, 300*time.Millisecond)
	r, _ := srv.rooms.CreateRoom()
	r.SetOnExpire(func() {
		r.BeginExpiring()
		r.BroadcastAll(protocol.Message{Type: protocol.MsgSystem, Payload: "expiring"})
		r.BeginClosing()
		r.FinishDestroying()
	})
	r.StartExpirationTimer()
	_ = r.RegisterWriter(1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			r.BroadcastAll(protocol.Message{Type: protocol.MsgChat, Payload: "chat"})
			time.Sleep(5 * time.Millisecond)
		}
	}()
	time.Sleep(400 * time.Millisecond)
	wg.Wait()
	_ = srv
}
