package room

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/terminalroom/terminalroom/internal/protocol"
)

const (
	MaxCapacity  = 2
	RoomIDLength = 6
	RoomIDPrefix = "TR-"
	allowedChars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

	DefaultRoomTTL = 10 * time.Minute
	MinRoomTTL     = 10 * time.Minute
	MaxRoomTTL     = 60 * time.Minute
)

// AllowedRoomTTLs is the user-facing selection set. Keep UI simple — 5 options.
var AllowedRoomTTLs = []time.Duration{
	10 * time.Minute,
	20 * time.Minute,
	30 * time.Minute,
	45 * time.Minute,
	60 * time.Minute,
}

// IsValidRoomTTL reports whether d is within the server-enforced bounds [Min,Max].
func IsValidRoomTTL(d time.Duration) bool {
	return d >= MinRoomTTL && d <= MaxRoomTTL
}

type State int

const (
	StateWaiting   State = iota
	StateActive
	StateExpiring
	StateClosing
	StateDestroyed
)

func (s State) String() string {
	switch s {
	case StateWaiting:
		return "waiting"
	case StateActive:
		return "active"
	case StateExpiring:
		return "expiring"
	case StateClosing:
		return "closing"
	case StateDestroyed:
		return "destroyed"
	default:
		return "unknown"
	}
}

func (s State) allowsJoin() bool {
	return s == StateWaiting || s == StateActive
}

type Participant struct {
	ID       int
	Name     string
	JoinedAt time.Time
}

type Room struct {
	mu           sync.RWMutex
	ID           string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	State        State
	participants map[int]*Participant
	nextSlot     int
	// writers holds per-participant channels for message delivery.
	// Ownership: Room owns the map, the participant goroutine (handleParticipant)
	// owns its channel and is the only closer. Room never closes a channel
	// while a broadcast may be sending; UnregisterWriter/FinishDestroying only
	// remove the entry, the owner closes afterwards. This makes send-on-closed
	// structurally impossible (close happens after delete, broadcast holds RLock
	// during send, delete holds Lock).
	writers     map[int]chan protocol.Message
	done        chan struct{}
	destroyOnce sync.Once
	expireTimer *time.Timer
	onExpire    func()
}

func GenerateRoomID() (string, error) {
	b := make([]byte, RoomIDLength)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate room ID: %w", err)
	}
	for i, v := range b {
		b[i] = allowedChars[v%byte(len(allowedChars))]
	}
	return RoomIDPrefix + string(b), nil
}

func NewRoom(id string) *Room {
	return &Room{
		ID:           id,
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(DefaultRoomTTL),
		State:        StateWaiting,
		participants: make(map[int]*Participant),
		nextSlot:     1,
		writers:      make(map[int]chan protocol.Message),
		done:         make(chan struct{}),
	}
}

func NewRoomWithTTL(id string, ttl time.Duration) *Room {
	return &Room{
		ID:           id,
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(ttl),
		State:        StateWaiting,
		participants: make(map[int]*Participant),
		nextSlot:     1,
		writers:      make(map[int]chan protocol.Message),
		done:         make(chan struct{}),
	}
}

func (r *Room) StartExpirationTimer() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.State == StateDestroyed {
		return
	}

	remaining := time.Until(r.ExpiresAt)
	if remaining <= 0 {
		if r.onExpire != nil {
			go r.onExpire()
		}
		return
	}

	r.expireTimer = time.AfterFunc(remaining, func() {
		r.mu.RLock()
		cb := r.onExpire
		r.mu.RUnlock()
		if cb != nil {
			cb()
		}
	})
}

func (r *Room) SetOnExpire(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onExpire = fn
}

func (r *Room) StopExpirationTimer() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.expireTimer != nil {
		r.expireTimer.Stop()
		r.expireTimer = nil
	}
}

func (r *Room) TimeRemaining() time.Duration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.State == StateDestroyed {
		return 0
	}
	remaining := time.Until(r.ExpiresAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (r *Room) Join(name string) (*Participant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.State.allowsJoin() {
		return nil, fmt.Errorf("room is not available")
	}

	if len(r.participants) >= MaxCapacity {
		return nil, fmt.Errorf("room is full")
	}

	p := &Participant{
		ID:       r.nextSlot,
		Name:     name,
		JoinedAt: time.Now(),
	}
	r.participants[p.ID] = p
	r.nextSlot++

	if len(r.participants) == MaxCapacity {
		r.State = StateActive
	}

	return p, nil
}

func (r *Room) Leave(id int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.participants, id)

	if len(r.participants) == 0 && r.State != StateDestroyed && r.State != StateClosing {
		r.destroyLocked()
	}
}

func (r *Room) destroyLocked() {
	r.State = StateDestroyed
	r.destroyOnce.Do(func() {
		close(r.done)
	})
}

func (r *Room) Destroy() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.destroyLocked()
}

func (r *Room) BeginExpiring() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.State != StateWaiting && r.State != StateActive {
		return false
	}

	r.State = StateExpiring
	if r.expireTimer != nil {
		r.expireTimer.Stop()
		r.expireTimer = nil
	}
	return true
}

func (r *Room) BeginClosing() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.State != StateExpiring {
		return false
	}

	r.State = StateClosing
	return true
}

func (r *Room) FinishDestroying() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.State == StateDestroyed {
		return
	}

	// Do not close channels here; owner goroutine closes its own channel after
	// UnregisterWriter deletes it. Just clear the map so no new broadcasts target them.
	for id := range r.writers {
		delete(r.writers, id)
	}

	r.destroyLocked()
}

func (r *Room) ParticipantCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.participants)
}

func (r *Room) IsActive() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.State == StateActive
}

func (r *Room) IsDestroyed() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.State == StateDestroyed
}

func (r *Room) IsExpiring() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.State == StateExpiring || r.State == StateClosing
}

func (r *Room) Done() chan struct{} {
	return r.done
}

func (r *Room) RegisterWriter(id int) chan protocol.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	// If re-registering same ID, just remove old entry; old channel's owner
	// already closed it (or will via Done). Do not close here to avoid
	// send-on-closed.
	if _, ok := r.writers[id]; ok {
		delete(r.writers, id)
	}
	ch := make(chan protocol.Message, 64)
	r.writers[id] = ch
	return ch
}

func (r *Room) UnregisterWriter(id int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Ownership: do not close here. Caller (handleParticipant) owns the channel
	// and closes it after this delete, when no new broadcasts can find it.
	if _, ok := r.writers[id]; ok {
		delete(r.writers, id)
	}
}

func (r *Room) BroadcastExcept(senderID int, msg protocol.Message) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.State == StateDestroyed || r.State == StateClosing {
		return
	}

	for id, ch := range r.writers {
		if id == senderID {
			continue
		}
		select {
		case ch <- msg:
		default:
		}
	}
}

func (r *Room) BroadcastAll(msg protocol.Message) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.State == StateDestroyed || r.State == StateClosing {
		return
	}

	for _, ch := range r.writers {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (r *Room) GetParticipantName(id int) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p, ok := r.participants[id]; ok {
		return p.Name
	}
	return ""
}

func (r *Room) OtherParticipant(id int) *Participant {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.participants {
		if p.ID != id {
			return p
		}
	}
	return nil
}
