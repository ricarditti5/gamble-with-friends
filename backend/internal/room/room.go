package room

import (
	"crypto/rand"
	"errors"
	"math/big"
	"sync"
	"time"

	"gamblefriends/backend/internal/game"
)

var (
	ErrRoomNotFound     = errors.New("room not found")
	ErrRoomFull         = errors.New("room is full")
	ErrRoomClosed       = errors.New("room is closed")
	ErrNotHost          = errors.New("only the host can do that")
	ErrNotEnoughPlayers = errors.New("need at least 2 players to start")
	ErrInvalidConfig    = errors.New("invalid room config")
)

type RoomStatus string

const (
	StatusWaiting    RoomStatus = "waiting"
	StatusInProgress RoomStatus = "in_progress"
	StatusFinished   RoomStatus = "finished"
)

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

type Config struct {
	Name         string `json:"name"`
	MaxPlayers   int    `json:"max_players"`
	InitialChips int    `json:"initial_chips"`
	SmallBlind   int    `json:"small_blind"`
	BigBlind     int    `json:"big_blind"`
}

func (c Config) Validate() error {
	if c.Name == "" || len(c.Name) > 40 {
		return ErrInvalidConfig
	}
	if c.MaxPlayers < 2 || c.MaxPlayers > 9 {
		return ErrInvalidConfig
	}
	if c.InitialChips < 10 || c.InitialChips > 1_000_000 {
		return ErrInvalidConfig
	}
	if c.SmallBlind < 1 || c.BigBlind <= c.SmallBlind || c.BigBlind > c.InitialChips/2 {
		return ErrInvalidConfig
	}
	return nil
}

type Seat struct {
	SessionID string
	Nickname  string
	IsHost    bool
	Removed   bool // kicked mid-game; leaves after the current hand
}

// Client is a connected WebSocket session.
type Client struct {
	SessionID string
	Nickname  string
	Send      chan []byte
	Close     func()
}

func (c *Client) sendJSON(v any) {
	// Non-blocking; drop the message if the client is too slow. The write
	// loop will detect a full buffer and close the connection.
	b, err := jsonMarshal(v)
	if err != nil {
		return
	}
	select {
	case c.Send <- b:
	default:
	}
}

type MatchInfo struct {
	WinnerSession string `json:"winner_session"`
	WinnerName    string `json:"winner_name"`
	FinalChips    int    `json:"final_chips"`
	TotalPot      int    `json:"total_pot"`
	PlayerCount   int    `json:"player_count"`
}

type Room struct {
	code   string
	config Config
	status RoomStatus

	mu      sync.RWMutex
	seats   []*Seat
	clients map[string]*Client
	hostID  string

	engine   *game.Engine
	champion *MatchInfo

	commands chan Command
	done     chan struct{}
	wg       sync.WaitGroup

	timer         *time.Timer
	pendingNext   bool
	actionTimeout time.Duration
	nextHandDelay time.Duration
	log           []LogEntry
	lastActivity  time.Time
	onFinish      func(room *Room)
}

type LogEntry struct {
	Text string `json:"text"`
	Kind string `json:"kind"`
}

type Command struct {
	Kind      int
	Client    *Client
	SessionID string
	Target    string
	Action    game.Action
	Resp      chan error
}

const (
	CmdJoin = iota
	CmdLeave
	CmdAction
	CmdStart
	CmdKick
	CmdTick
	CmdShutdown
)

func NewRoom(code string, cfg Config, host *Seat, actionTimeout, nextHandDelay time.Duration, onFinish func(*Room)) *Room {
	r := &Room{
		code:          code,
		config:        cfg,
		status:        StatusWaiting,
		seats:         []*Seat{host},
		clients:       map[string]*Client{},
		hostID:        host.SessionID,
		commands:      make(chan Command, 64),
		done:          make(chan struct{}, 1),
		actionTimeout: actionTimeout,
		nextHandDelay: nextHandDelay,
		lastActivity:  time.Now(),
		onFinish:      onFinish,
	}
	r.wg.Add(1)
	go r.loop()
	return r
}

func (r *Room) Code() string   { return r.code }
func (r *Room) Config() Config { return r.config }

func (r *Room) Status() RoomStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.status
}

func (r *Room) PlayerCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.seats)
}

func (r *Room) HostSession() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.hostID
}

// Seats returns a copy of the current seats (safe for concurrent readers).
func (r *Room) Seats() []Seat {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Seat, len(r.seats))
	for i, s := range r.seats {
		out[i] = *s
	}
	return out
}

// Champion returns the finished match info, or nil if the match is running.
func (r *Room) Champion() *MatchInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.champion
}

// EngineStatus returns the current hand number and whether the hand is over
// (safe for concurrent readers such as tests).
func (r *Room) EngineStatus() (handNumber int, handOver bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.engine == nil {
		return 0, true
	}
	return r.engine.HandNumber(), r.engine.IsHandOver()
}

// PlayerChips returns the current chips of a player in the running match,
// or -1 if the player is not in the match.
func (r *Room) PlayerChips(sessionID string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.engine == nil {
		return -1
	}
	idx := r.engine.PlayerIndex(sessionID)
	if idx < 0 {
		return -1
	}
	return r.engine.Players()[idx].Chips
}

// PlayerDisconnected reports whether a player lost connection (safe read).
func (r *Room) PlayerDisconnected(sessionID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.engine == nil {
		return false
	}
	idx := r.engine.PlayerIndex(sessionID)
	if idx < 0 {
		return false
	}
	return r.engine.Players()[idx].Disconnected
}

func (r *Room) IsIdleFor(d time.Duration) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return time.Since(r.lastActivity) > d
}

func (r *Room) IsEmpty() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.seats) == 0
}

// Shutdown stops the room goroutine (used by the manager on cleanup).
func (r *Room) Shutdown() {
	select {
	case r.commands <- Command{Kind: CmdShutdown}:
	case <-r.done:
	}
}

// Command enqueues a command for the room's goroutine (single-threaded
// per-room execution, RNF2.1).
func (r *Room) Command(cmd Command) {
	r.commands <- cmd
}

func (r *Room) loop() {
	defer r.wg.Done()
	r.scheduleTimer()
	for {
		select {
		case cmd := <-r.commands:
			switch cmd.Kind {
			case CmdJoin:
				r.handleJoin(cmd)
			case CmdLeave:
				r.handleLeave(cmd)
			case CmdAction:
				r.handleAction(cmd)
			case CmdStart:
				r.handleStart(cmd)
			case CmdKick:
				r.handleKick(cmd)
			case CmdTick:
				r.handleTick()
			case CmdShutdown:
				return
			}
			r.scheduleTimer()
		case <-r.done:
			return
		}
	}
}

func (r *Room) handleJoin(cmd Command) {
	r.mu.Lock()
	if r.status == StatusFinished {
		r.mu.Unlock()
		cmd.Resp <- ErrRoomClosed
		return
	}
	var seat *Seat
	for _, s := range r.seats {
		if s.SessionID == cmd.Client.SessionID {
			if s.Removed {
				r.mu.Unlock()
				cmd.Resp <- errors.New("you were kicked from this room")
				return
			}
			seat = s
			break
		}
	}
	if seat == nil {
		if r.status != StatusWaiting {
			r.mu.Unlock()
			cmd.Resp <- errors.New("game already started")
			return
		}
		if len(r.seats) >= r.config.MaxPlayers {
			r.mu.Unlock()
			cmd.Resp <- ErrRoomFull
			return
		}
		seat = &Seat{SessionID: cmd.Client.SessionID, Nickname: cmd.Client.Nickname}
		r.seats = append(r.seats, seat)
		r.broadcastJSON("log", LogEntry{Text: cmd.Client.Nickname + " entrou na sala", Kind: "info"})
		r.broadcastJSON("player_joined", map[string]any{"session_id": cmd.Client.SessionID, "nickname": cmd.Client.Nickname})
	}
	if r.engine != nil {
		if idx := r.engine.PlayerIndex(cmd.Client.SessionID); idx >= 0 {
			r.engine.Players()[idx].Disconnected = false
		}
	}
	r.clients[cmd.Client.SessionID] = cmd.Client
	r.mu.Unlock()
	r.lastActivity = time.Now()
	cmd.Resp <- nil
	r.sendGameState()
}

func (r *Room) handleLeave(cmd Command) {
	r.mu.Lock()
	c, ok := r.clients[cmd.SessionID]
	if ok {
		delete(r.clients, cmd.SessionID)
		if c.Close != nil {
			c.Close()
		}
	}
	if r.status == StatusWaiting {
		for i, s := range r.seats {
			if s.SessionID == cmd.SessionID {
				r.seats = append(r.seats[:i], r.seats[i+1:]...)
				if s.IsHost {
					r.promoteHostLocked()
				}
				r.broadcastJSON("player_left", map[string]any{"session_id": cmd.SessionID})
				break
			}
		}
		if len(r.seats) == 0 {
			r.mu.Unlock()
			if r.onFinish != nil {
				r.onFinish(r)
			}
			select {
			case r.done <- struct{}{}:
			default:
			}
			return
		}
	} else if r.engine != nil {
		if idx := r.engine.PlayerIndex(cmd.SessionID); idx >= 0 {
			r.engine.Players()[idx].Disconnected = true
			r.broadcastJSON("log", LogEntry{Text: r.engine.Players()[idx].Nickname + " perdeu ligação", Kind: "info"})
		}
	}
	r.mu.Unlock()
	r.lastActivity = time.Now()
	cmd.Resp <- nil
	r.sendGameState()
}

func (r *Room) handleAction(cmd Command) {
	r.mu.Lock()
	if r.engine == nil || r.status != StatusInProgress {
		r.mu.Unlock()
		cmd.Resp <- errors.New("game not running")
		return
	}
	idx := r.engine.PlayerIndex(cmd.SessionID)
	if idx < 0 {
		r.mu.Unlock()
		cmd.Resp <- errors.New("not in this game")
		return
	}
	err := r.engine.Act(idx, cmd.Action)
	if err == nil && r.engine.IsHandOver() {
		r.pendingNext = true
	}
	r.mu.Unlock()
	cmd.Resp <- err
	if err == nil {
		r.logAction(cmd.SessionID, cmd.Action)
		if r.engine.IsHandOver() {
			r.broadcastJSON("showdown", r.engine.Result())
		}
		r.sendGameState()
	}
}

func (r *Room) handleStart(cmd Command) {
	if cmd.SessionID != r.hostID {
		cmd.Resp <- ErrNotHost
		return
	}
	r.mu.Lock()
	if r.status == StatusInProgress {
		r.mu.Unlock()
		cmd.Resp <- errors.New("game already running")
		return
	}
	if r.status == StatusWaiting {
		active := 0
		for _, s := range r.seats {
			if !s.Removed {
				active++
			}
		}
		if active < 2 {
			r.mu.Unlock()
			cmd.Resp <- ErrNotEnoughPlayers
			return
		}
	}
	// Restart (status finished) or first start (waiting): rebuild the engine
	// so everyone's chips reset (RF1.6).
	cfgs := make([]game.PlayerConfig, 0, len(r.seats))
	for _, s := range r.seats {
		if s.Removed {
			continue
		}
		cfgs = append(cfgs, game.PlayerConfig{SessionID: s.SessionID, Nickname: s.Nickname, Chips: r.config.InitialChips})
	}
	eng := game.NewEngine(cfgs, r.config.SmallBlind, r.config.BigBlind)
	if err := eng.StartHand(); err != nil {
		r.mu.Unlock()
		cmd.Resp <- err
		return
	}
	r.engine = eng
	r.status = StatusInProgress
	r.champion = nil
	r.log = nil
	r.mu.Unlock()
	cmd.Resp <- nil
	r.broadcastJSON("log", LogEntry{Text: "A partida começou!", Kind: "game"})
	r.sendGameState()
}

func (r *Room) handleKick(cmd Command) {
	if cmd.SessionID != r.hostID {
		cmd.Resp <- ErrNotHost
		return
	}
	if cmd.SessionID == cmd.Target {
		cmd.Resp <- errors.New("cannot kick yourself")
		return
	}
	cmd.Resp <- nil
	r.mu.Lock()
	for _, s := range r.seats {
		if s.SessionID == cmd.Target {
			s.Removed = true
			break
		}
	}
	if r.status == StatusWaiting {
		for i, s := range r.seats {
			if s.SessionID == cmd.Target {
				r.seats = append(r.seats[:i], r.seats[i+1:]...)
				break
			}
		}
	}
	if c, ok := r.clients[cmd.Target]; ok {
		delete(r.clients, cmd.Target)
		c.sendJSON(map[string]any{"type": "kicked"})
		if c.Close != nil {
			c.Close()
		}
	}
	kickedName := ""
	if r.engine != nil {
		if idx := r.engine.PlayerIndex(cmd.Target); idx >= 0 {
			r.engine.FoldForce(idx)
			kickedName = r.engine.Players()[idx].Nickname
		}
	}
	r.mu.Unlock()
	r.lastActivity = time.Now()
	if kickedName != "" {
		r.broadcastJSON("log", LogEntry{Text: kickedName + " foi expulso", Kind: "info"})
	}
	r.broadcastJSON("player_left", map[string]any{"session_id": cmd.Target})
	r.sendGameState()
}

func (r *Room) handleTick() {
	if r.pendingNext {
		r.pendingNext = false
		if r.engine != nil {
			r.nextHand()
		}
		return
	}
	r.mu.Lock()
	if r.engine == nil || r.status != StatusInProgress || r.engine.IsHandOver() {
		r.mu.Unlock()
		return
	}
	// Timeout: auto fold/check for the current player (RF3.10).
	acted := false
	handOver := false
	if idx := r.engine.CurrentIdx(); idx >= 0 {
		a := r.engine.AutoAction()
		if err := r.engine.Act(idx, a); err == nil {
			acted = true
			handOver = r.engine.IsHandOver()
			if handOver {
				r.pendingNext = true
			}
			r.logActionLocked(r.engine.Players()[idx].SessionID, a)
		}
	}
	r.mu.Unlock()
	if acted {
		if handOver {
			r.broadcastJSON("showdown", r.engine.Result())
		}
		r.sendGameState()
	}
}

// scheduleTimer arms the action timer when a hand is waiting for input,
// or the next-hand delay when a hand just ended.
func (r *Room) scheduleTimer() {
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
	r.mu.RLock()
	active := r.status == StatusInProgress && r.engine != nil
	var handOver, pendingNext bool
	if active {
		handOver = r.engine.IsHandOver()
		pendingNext = r.pendingNext
	}
	r.mu.RUnlock()
	if !active {
		return
	}
	if handOver {
		if pendingNext {
			r.timer = time.AfterFunc(r.nextHandDelay, func() {
				r.commands <- Command{Kind: CmdTick}
			})
		}
		return
	}
	r.timer = time.AfterFunc(r.actionTimeout, func() {
		r.commands <- Command{Kind: CmdTick}
	})
}

func (r *Room) nextHand() {
	r.mu.Lock()
	r.engine.MarkSpectators()
	var champion *MatchInfo
	if r.engine.MatchOver() {
		r.status = StatusFinished
		champion = &MatchInfo{
			WinnerSession: r.engine.Players()[r.engine.ChampionIdx()].SessionID,
			WinnerName:    r.engine.Players()[r.engine.ChampionIdx()].Nickname,
			FinalChips:    r.engine.Players()[r.engine.ChampionIdx()].Chips,
			TotalPot:      r.engine.Pot(),
			PlayerCount:   len(r.seats),
		}
		r.champion = champion
	} else {
		// Remove kicked players' seats and forfeit their chips.
		var kept []*Seat
		for _, s := range r.seats {
			if !s.Removed {
				kept = append(kept, s)
			}
		}
		r.seats = kept
		keptIDs := map[string]bool{}
		for _, s := range kept {
			keptIDs[s.SessionID] = true
		}
		for _, p := range r.engine.Players() {
			if !keptIDs[p.SessionID] {
				p.Chips = 0
				p.Status = game.Spectator
			}
		}
	}
	r.mu.Unlock()

	if champion != nil {
		r.broadcastJSON("match_over", champion)
		r.logAction("", game.Action{Type: game.ActionFold}, "Fim da partida!")
		if r.onFinish != nil {
			r.onFinish(r)
		}
		return
	}
	r.mu.Lock()
	err := r.engine.StartHand()
	r.mu.Unlock()
	if err != nil {
		return
	}
	r.sendGameState()
}

// logActionLocked appends to the log; the caller must hold r.mu.
func (r *Room) logActionLocked(sessionID string, a game.Action, forced ...string) {
	label := ""
	if len(forced) > 0 {
		label = forced[0]
	}
	if label == "" {
		var nick string
		if idx := r.engine.PlayerIndex(sessionID); idx >= 0 {
			nick = r.engine.Players()[idx].Nickname
		}
		verb := map[game.ActionType]string{
			game.ActionFold:  "fez fold",
			game.ActionCheck: "fez check",
			game.ActionCall:  "fez call",
			game.ActionRaise: "apostou para " + itoa(a.Amount),
			game.ActionAllIn: "foi all-in",
		}[a.Type]
		label = nick + " " + verb
	}
	r.log = append(r.log, LogEntry{Text: label, Kind: "action"})
	if len(r.log) > 50 {
		r.log = r.log[len(r.log)-50:]
	}
	r.broadcastJSON("log", r.log[len(r.log)-1])
}

// logAction is a lock-safe wrapper around logActionLocked.
func (r *Room) logAction(sessionID string, a game.Action, forced ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logActionLocked(sessionID, a, forced...)
}

func (r *Room) promoteHostLocked() {
	for _, s := range r.seats {
		if !s.IsHost {
			s.IsHost = true
			r.hostID = s.SessionID
			r.broadcastJSON("host_changed", map[string]any{"session_id": s.SessionID})
			return
		}
	}
	r.hostID = ""
}

func (r *Room) broadcastJSON(msgType string, payload any) {
	r.broadcast(map[string]any{"type": msgType, "payload": payload})
}

func (r *Room) broadcast(base map[string]any) {
	if len(r.clients) == 0 {
		return
	}
	b, err := jsonMarshal(base)
	if err != nil {
		return
	}
	for _, c := range r.clients {
		select {
		case c.Send <- b:
		default:
		}
	}
}

func (r *Room) sendGameState() {
	r.mu.RLock()
	roomInfo := map[string]any{
		"code":        r.code,
		"name":        r.config.Name,
		"status":      r.status,
		"host_id":     r.hostID,
		"max_players": r.config.MaxPlayers,
	}
	seats := make([]map[string]any, 0, len(r.seats))
	for _, s := range r.seats {
		if s.Removed {
			continue
		}
		seats = append(seats, map[string]any{
			"session_id": s.SessionID,
			"nickname":   s.Nickname,
			"is_host":    s.IsHost,
		})
	}
	log := append([]LogEntry{}, r.log...)
	r.mu.RUnlock()

	var remaining int
	if r.engine != nil && !r.engine.IsHandOver() && r.status == StatusInProgress {
		remaining = int(r.actionTimeout.Seconds())
	}

	for _, c := range r.clients {
		msg := map[string]any{
			"type":      "game_state",
			"room":      roomInfo,
			"seats":     seats,
			"log":       log,
			"remaining": remaining,
		}
		if r.engine != nil {
			ps := r.engine.PublicState()
			msg["state"] = ps
			if idx := r.engine.PlayerIndex(c.SessionID); idx >= 0 {
				msg["your_cards"] = r.engine.HoleCards(idx)
				msg["your_idx"] = idx
			} else {
				msg["your_cards"] = []game.Card{}
				msg["your_idx"] = -1
			}
		}
		b, err := jsonMarshal(msg)
		if err != nil {
			continue
		}
		select {
		case c.Send <- b:
		default:
		}
	}
}

func ActionFromString(s string) game.Action {
	switch s {
	case "fold":
		return game.Action{Type: game.ActionFold}
	case "check":
		return game.Action{Type: game.ActionCheck}
	case "call":
		return game.Action{Type: game.ActionCall}
	case "raise":
		return game.Action{Type: game.ActionRaise}
	case "all_in":
		return game.Action{Type: game.ActionAllIn}
	}
	return game.Action{Type: game.ActionFold}
}

func GenerateCode() (string, error) {
	b := make([]byte, 6)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(codeAlphabet))))
		if err != nil {
			return "", err
		}
		b[i] = codeAlphabet[n.Int64()]
	}
	return string(b), nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
