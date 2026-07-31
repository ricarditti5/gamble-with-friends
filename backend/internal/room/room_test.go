package room

import (
	"testing"
	"time"

	"gamblefriends/backend/internal/game"
)

func newTestRoom(t *testing.T) *Room {
	t.Helper()
	cfg := Config{Name: "test", MaxPlayers: 4, InitialChips: 1000, SmallBlind: 5, BigBlind: 10}
	host := &Seat{SessionID: "host-uuid", Nickname: "host", IsHost: true}
	r := NewRoom("ABC123", cfg, host, 2*time.Second, 500*time.Millisecond, nil)
	t.Cleanup(r.Shutdown)
	return r
}

func testClient(t *testing.T, r *Room, sessionID, nickname string) *Client {
	t.Helper()
	c := &Client{SessionID: sessionID, Nickname: nickname, Send: make(chan []byte, 64)}
	resp := make(chan error, 1)
	r.Command(Command{Kind: CmdJoin, Client: c, Resp: resp})
	if err := <-resp; err != nil {
		t.Fatalf("join %s: %v", nickname, err)
	}
	return c
}

func mustCommand(t *testing.T, r *Room, cmd Command) {
	t.Helper()
	resp := make(chan error, 1)
	cmd.Resp = resp
	r.Command(cmd)
	if err := <-resp; err != nil {
		t.Fatalf("command failed: %v", err)
	}
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func TestRoomJoinStartAndPlay(t *testing.T) {
	r := newTestRoom(t)
	testClient(t, r, "host-uuid", "host")
	testClient(t, r, "p2-uuid", "p2")

	mustCommand(t, r, Command{Kind: CmdStart, SessionID: "host-uuid"})
	if r.Status() != StatusInProgress {
		t.Fatalf("status = %v, want in_progress", r.Status())
	}

	// The engine auto-advances; let the timer do auto-actions until a hand
	// finishes (test room uses a 2s action timeout).
	waitFor(t, func() bool {
		_, handOver := r.EngineStatus()
		return handOver
	}, "hand to finish via auto-actions")

	// After the next-hand delay the room should have started a new hand.
	waitFor(t, func() bool {
		handNumber, handOver := r.EngineStatus()
		return handNumber >= 2 && !handOver
	}, "second hand to start")
}

func TestRoomActionValidation(t *testing.T) {
	r := newTestRoom(t)
	testClient(t, r, "host-uuid", "host")
	testClient(t, r, "p2-uuid", "p2")
	mustCommand(t, r, Command{Kind: CmdStart, SessionID: "host-uuid"})

	resp := make(chan error, 1)
	r.Command(Command{Kind: CmdAction, SessionID: "p2-uuid", Action: game.Action{Type: game.ActionFold}, Resp: resp})
	if err := <-resp; err == nil {
		t.Error("p2 should not be able to act out of turn")
	}
}

func TestRoomKickInWaiting(t *testing.T) {
	r := newTestRoom(t)
	testClient(t, r, "host-uuid", "host")
	c2 := testClient(t, r, "p2-uuid", "p2")

	mustCommand(t, r, Command{Kind: CmdKick, SessionID: "host-uuid", Target: "p2-uuid"})

	// Kicked player's client should receive the kicked message.
	select {
	case msg := <-c2.Send:
		if string(msg) == "" {
			t.Error("expected kicked message")
		}
	case <-time.After(1 * time.Second):
		t.Error("kicked player did not receive a message")
	}
	if r.PlayerCount() != 1 {
		t.Errorf("player count = %d, want 1", r.PlayerCount())
	}

	// Non-host cannot kick.
	resp := make(chan error, 1)
	r.Command(Command{Kind: CmdKick, SessionID: "p2-uuid", Target: "host-uuid", Resp: resp})
	if err := <-resp; err == nil {
		t.Error("non-host kick should fail")
	}
}

func TestRoomFullRejected(t *testing.T) {
	cfg := Config{Name: "x", MaxPlayers: 2, InitialChips: 1000, SmallBlind: 5, BigBlind: 10}
	r := NewRoom("ABC123", cfg, &Seat{SessionID: "host", Nickname: "host", IsHost: true}, time.Second, time.Second, nil)
	t.Cleanup(r.Shutdown)
	testClient(t, r, "host", "host")
	testClient(t, r, "p2", "p2")
	c3 := &Client{SessionID: "p3", Nickname: "p3", Send: make(chan []byte, 64)}
	resp := make(chan error, 1)
	r.Command(Command{Kind: CmdJoin, Client: c3, Resp: resp})
	if err := <-resp; err == nil {
		t.Error("third player should be rejected in a 2-max room")
	}
}

func TestRoomReconnectKeepsSeat(t *testing.T) {
	r := newTestRoom(t)
	testClient(t, r, "host-uuid", "host")
	testClient(t, r, "p2-uuid", "p2")
	mustCommand(t, r, Command{Kind: CmdStart, SessionID: "host-uuid"})

	// Simulate disconnect + reconnect with the same session ID.
	resp := make(chan error, 1)
	r.Command(Command{Kind: CmdLeave, SessionID: "p2-uuid", Resp: resp})
	<-resp

	if !r.PlayerDisconnected("p2-uuid") {
		t.Error("p2 should be marked disconnected")
	}

	c2 := testClient(t, r, "p2-uuid", "p2")
	if c2 == nil {
		t.Fatal("reconnect failed")
	}
	if r.PlayerDisconnected("p2-uuid") {
		t.Error("p2 should be reconnected")
	}
}

func TestRoomSecondMatchResetsChips(t *testing.T) {
	cfg := Config{Name: "test", MaxPlayers: 4, InitialChips: 1000, SmallBlind: 5, BigBlind: 10}
	r := NewRoom("ABC123", cfg, &Seat{SessionID: "host-uuid", Nickname: "host", IsHost: true}, 10*time.Second, 200*time.Millisecond, nil)
	t.Cleanup(r.Shutdown)
	testClient(t, r, "host-uuid", "host")
	testClient(t, r, "p2-uuid", "p2")
	mustCommand(t, r, Command{Kind: CmdStart, SessionID: "host-uuid"})

	// Heads-up: host (dealer/SB) acts first preflop. All-in vs call settles
	// the whole match in one hand.
	mustCommand(t, r, Command{Kind: CmdAction, SessionID: "host-uuid", Action: game.Action{Type: game.ActionRaise, Amount: 1000}})
	mustCommand(t, r, Command{Kind: CmdAction, SessionID: "p2-uuid", Action: game.Action{Type: game.ActionCall}})

	waitFor(t, func() bool { return r.Status() == StatusFinished }, "match to finish")

	mustCommand(t, r, Command{Kind: CmdStart, SessionID: "host-uuid"})
	// Chips reset to initial (RF1.6); blinds are posted right after start,
	// so every player must hold at least initial - big_blind.
	min := r.config.InitialChips - r.config.BigBlind
	for _, s := range r.Seats() {
		chips := r.PlayerChips(s.SessionID)
		if chips < min {
			t.Errorf("%s chips = %d, want >= %d (reset)", s.Nickname, chips, min)
		}
	}
	if r.Status() != StatusInProgress {
		t.Fatalf("status = %v, want in_progress", r.Status())
	}
}
