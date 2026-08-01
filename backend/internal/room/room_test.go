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
	r := NewRoom("ABC123", cfg, host, 500*time.Millisecond, nil)
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
	waitForDur(t, 5*time.Second, cond, what)
}

func waitForDur(t *testing.T, timeout time.Duration, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
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

	// No action timer: the game waits for human input, so drive the hand
	// manually. Heads-up: the dealer (host, idx 0) acts first preflop.
	waitFor(t, func() bool { return r.engine.CurrentIdx() == 0 }, "host's turn preflop")
	mustCommand(t, r, Command{Kind: CmdAction, SessionID: "host-uuid", Action: game.Action{Type: game.ActionRaise, Amount: 30}})
	waitFor(t, func() bool { return r.engine.CurrentIdx() == 1 }, "p2's turn preflop")
	mustCommand(t, r, Command{Kind: CmdAction, SessionID: "p2-uuid", Action: game.Action{Type: game.ActionCall}})

	// The flop is dealt automatically when the betting round completes.
	// Postflop the dealer (host, idx 0) acts last, so p2 goes first.
	waitFor(t, func() bool { return r.engine.Phase() == game.PhaseFlop }, "flop dealt automatically")
	waitFor(t, func() bool { return r.engine.CurrentIdx() == 1 }, "p2's turn on the flop")
	mustCommand(t, r, Command{Kind: CmdAction, SessionID: "p2-uuid", Action: game.Action{Type: game.ActionCheck}})
	waitFor(t, func() bool { return r.engine.CurrentIdx() == 0 }, "host's turn on the flop")
	mustCommand(t, r, Command{Kind: CmdAction, SessionID: "host-uuid", Action: game.Action{Type: game.ActionFold}})

	waitFor(t, func() bool {
		_, handOver := r.EngineStatus()
		return handOver
	}, "hand to finish")

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
	r := NewRoom("ABC123", cfg, &Seat{SessionID: "host", Nickname: "host", IsHost: true}, time.Second, nil)
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
	r := NewRoom("ABC123", cfg, &Seat{SessionID: "host-uuid", Nickname: "host", IsHost: true}, 200*time.Millisecond, nil)
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

func TestRoomAddBotAndPlay(t *testing.T) {
	r := newTestRoom(t)
	testClient(t, r, "host-uuid", "host")

	mustCommand(t, r, Command{Kind: CmdAddBot, SessionID: "host-uuid"})
	mustCommand(t, r, Command{Kind: CmdAddBot, SessionID: "host-uuid"})
	if got := r.PlayerCount(); got != 3 {
		t.Fatalf("player count = %d, want 3", got)
	}
	// Non-host cannot add bots.
	testClient(t, r, "other-uuid", "other")
	resp := make(chan error, 1)
	r.Command(Command{Kind: CmdAddBot, SessionID: "other-uuid", Resp: resp})
	if err := <-resp; err == nil {
		t.Error("non-host add_bot should fail")
	}
	// Remove the extra human so the match runs with host + bots.
	resp = make(chan error, 1)
	r.Command(Command{Kind: CmdLeave, SessionID: "other-uuid", Immediate: true, Resp: resp})
	if err := <-resp; err != nil {
		t.Fatalf("leave failed: %v", err)
	}

	mustCommand(t, r, Command{Kind: CmdStart, SessionID: "host-uuid"})
	if r.Status() != StatusInProgress {
		t.Fatalf("status = %v, want in_progress", r.Status())
	}
	// Bots act automatically; when it is the host's turn, fold for them
	// (the host is the only human and there is no action timer anymore).
	// A full street-by-street hand can take ~15s with bot delays.
	foldResp := make(chan error, 1)
	waitForDur(t, 30*time.Second, func() bool {
		_, handOver := r.EngineStatus()
		if !handOver && r.engine.CurrentIdx() == 0 {
			r.Command(Command{Kind: CmdAction, SessionID: "host-uuid", Action: game.Action{Type: game.ActionFold}, Resp: foldResp})
			if err := <-foldResp; err != nil {
				t.Errorf("host fold failed: %v", err)
			}
		}
		_, handOver = r.EngineStatus()
		return handOver
	}, "hand to finish with bots playing")

	// Bots cannot be added or removed mid-game.
	resp = make(chan error, 1)
	r.Command(Command{Kind: CmdRemoveBot, SessionID: "host-uuid", Target: "bot-1", Resp: resp})
	if err := <-resp; err == nil {
		t.Error("removing a bot mid-game should fail")
	}
}

func TestRoomRemoveBotWaiting(t *testing.T) {
	r := newTestRoom(t)
	testClient(t, r, "host-uuid", "host")
	mustCommand(t, r, Command{Kind: CmdAddBot, SessionID: "host-uuid"})
	mustCommand(t, r, Command{Kind: CmdAddBot, SessionID: "host-uuid"})

	mustCommand(t, r, Command{Kind: CmdRemoveBot, SessionID: "host-uuid", Target: "bot-1"})
	if got := r.PlayerCount(); got != 2 {
		t.Fatalf("player count = %d, want 2 after removing a bot", got)
	}
	resp := make(chan error, 1)
	r.Command(Command{Kind: CmdRemoveBot, SessionID: "host-uuid", Target: "bot-1", Resp: resp})
	if err := <-resp; err == nil {
		t.Error("removing a bot twice should fail")
	}
}

func TestRoomHeadsUpBlindChoice(t *testing.T) {
	r := newTestRoom(t)
	testClient(t, r, "host-uuid", "host")
	testClient(t, r, "p2-uuid", "p2")

	// Host picks p2 as the big blind; host must become dealer (small blind).
	mustCommand(t, r, Command{Kind: CmdStart, SessionID: "host-uuid", Target: "p2-uuid"})

	if r.engine == nil {
		t.Fatal("engine not started")
	}
	players := r.engine.Players()
	if players[0].SessionID != "host-uuid" || players[1].SessionID != "p2-uuid" {
		t.Fatalf("unexpected player order: %s, %s", players[0].SessionID, players[1].SessionID)
	}
	if got := r.engine.DealerIdx(); got != 0 {
		t.Errorf("dealer = %d, want 0 (host as small blind)", got)
	}
	if got := players[1].BetThisRound; got != r.config.BigBlind {
		t.Errorf("p2 big blind = %d, want %d", got, r.config.BigBlind)
	}
	if got := players[0].BetThisRound; got != r.config.SmallBlind {
		t.Errorf("host small blind = %d, want %d", got, r.config.SmallBlind)
	}
}

func TestRoomWaitingDisconnectExpire(t *testing.T) {
	r := newTestRoom(t)
	r.disconnectGrace = 150 * time.Millisecond
	testClient(t, r, "host-uuid", "host")
	testClient(t, r, "p2-uuid", "p2")

	// Dropped connection (refresh): seat is kept, marked disconnected.
	resp := make(chan error, 1)
	r.Command(Command{Kind: CmdLeave, SessionID: "p2-uuid", Resp: resp})
	if err := <-resp; err != nil {
		t.Fatalf("leave failed: %v", err)
	}
	if r.PlayerCount() != 2 {
		t.Fatalf("player count = %d, want 2 (seat kept on disconnect)", r.PlayerCount())
	}

	// A refresh reconnects with the same session: seat is reclaimed.
	testClient(t, r, "p2-uuid", "p2")
	resp = make(chan error, 1)
	r.Command(Command{Kind: CmdLeave, SessionID: "p2-uuid", Resp: resp})
	<-resp

	// No reconnect within the grace period: seat expires.
	waitFor(t, func() bool { return r.PlayerCount() == 1 }, "disconnected seat to expire")
}

func TestRoomExplicitLeaveRemovesSeat(t *testing.T) {
	r := newTestRoom(t)
	r.disconnectGrace = time.Hour
	testClient(t, r, "host-uuid", "host")
	testClient(t, r, "p2-uuid", "p2")

	resp := make(chan error, 1)
	r.Command(Command{Kind: CmdLeave, SessionID: "p2-uuid", Immediate: true, Resp: resp})
	if err := <-resp; err != nil {
		t.Fatalf("leave failed: %v", err)
	}
	if r.PlayerCount() != 1 {
		t.Errorf("player count = %d, want 1 after explicit leave", r.PlayerCount())
	}
}
