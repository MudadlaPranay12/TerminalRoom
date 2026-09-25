package room

import (
	"fmt"
	"sync"
	"time"
)

type Manager struct {
	mu    sync.RWMutex
	rooms map[string]*Room
}

func NewManager() *Manager {
	return &Manager{
		rooms: make(map[string]*Room),
	}
}

func (m *Manager) CreateRoom() (*Room, error) {
	return m.CreateRoomWithTTL(DefaultRoomTTL)
}

func (m *Manager) CreateRoomWithTTL(ttl time.Duration) (*Room, error) {
	id, err := GenerateRoomID()
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for m.rooms[id] != nil {
		id, err = GenerateRoomID()
		if err != nil {
			return nil, err
		}
	}

	r := NewRoomWithTTL(id, ttl)
	m.rooms[id] = r
	return r, nil
}

func (m *Manager) GetRoom(id string) (*Room, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	r, ok := m.rooms[id]
	if !ok {
		return nil, fmt.Errorf("room not found")
	}
	if r.IsDestroyed() {
		return nil, fmt.Errorf("room not found")
	}
	return r, nil
}

func (m *Manager) RemoveRoom(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.rooms, id)
}

func (m *Manager) ActiveRoomCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.rooms)
}
