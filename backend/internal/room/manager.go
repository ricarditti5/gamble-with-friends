package room

import (
	"context"
	"sync"
	"time"
)

// Manager owns all rooms (RNF2.2): isolated rooms, no shared mutable state.
type Manager struct {
	mu    sync.RWMutex
	rooms map[string]*Room

	ActionTimeout time.Duration
	NextHandDelay time.Duration
	IdleTTL       time.Duration
	FinishedTTL   time.Duration
	OnRoomFinish  func(*Room)
}

func NewManager() *Manager {
	return &Manager{
		rooms:         map[string]*Room{},
		ActionTimeout: 25 * time.Second,
		NextHandDelay: 4 * time.Second,
		IdleTTL:       30 * time.Minute,
		FinishedTTL:   1 * time.Hour,
	}
}

func (m *Manager) CreateRoom(cfg Config, host *Seat) (*Room, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	code, err := GenerateCode()
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	for {
		if _, exists := m.rooms[code]; !exists {
			break
		}
		code, err = GenerateCode()
		if err != nil {
			m.mu.Unlock()
			return nil, err
		}
	}
	r := NewRoom(code, cfg, host, m.ActionTimeout, m.NextHandDelay, func(r *Room) {
		if m.OnRoomFinish != nil {
			m.OnRoomFinish(r)
		}
	})
	m.rooms[code] = r
	m.mu.Unlock()
	return r, nil
}

func (m *Manager) GetRoom(code string) (*Room, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.rooms[code]
	return r, ok
}

func (m *Manager) RemoveRoom(code string) {
	m.mu.Lock()
	r, ok := m.rooms[code]
	if ok {
		delete(m.rooms, code)
	}
	m.mu.Unlock()
	if ok {
		r.Shutdown()
	}
}

// SweepLoop periodically removes abandoned waiting rooms and stale finished
// rooms so the process does not accumulate state forever.
func (m *Manager) SweepLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.sweep()
		}
	}
}

func (m *Manager) sweep() {
	m.mu.RLock()
	rooms := make([]*Room, 0, len(m.rooms))
	for _, r := range m.rooms {
		rooms = append(rooms, r)
	}
	m.mu.RUnlock()
	now := time.Now()
	for _, r := range rooms {
		remove := false
		if r.Status() == StatusWaiting && r.IsIdleFor(m.IdleTTL) {
			remove = true
		}
		if r.Status() == StatusFinished && r.IsIdleFor(m.FinishedTTL) {
			remove = true
		}
		if r.IsEmpty() {
			remove = true
		}
		if remove {
			_ = now
			m.RemoveRoom(r.Code())
		}
	}
}
