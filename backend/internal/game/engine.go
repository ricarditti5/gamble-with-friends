package game

import (
	"errors"
	"sort"
)

var (
	ErrNotEnoughPlayers  = errors.New("not enough players (min 2)")
	ErrHandOver          = errors.New("hand is over")
	ErrNotYourTurn       = errors.New("not your turn")
	ErrNotActive         = errors.New("player is not active")
	ErrNothingToCall     = errors.New("nothing to call, use check")
	ErrCannotCheck       = errors.New("cannot check when facing a bet")
	ErrRaiseTooSmall     = errors.New("raise below minimum")
	ErrInsufficientChips = errors.New("not enough chips")
)

type PlayerStatus int

const (
	Active PlayerStatus = iota
	Folded
	AllIn
	Spectator
)

func (s PlayerStatus) String() string {
	switch s {
	case Active:
		return "active"
	case Folded:
		return "folded"
	case AllIn:
		return "all_in"
	case Spectator:
		return "spectator"
	}
	return "unknown"
}

type Phase int

const (
	PhasePreflop Phase = iota
	PhaseFlop
	PhaseTurn
	PhaseRiver
	PhaseHandOver
)

func (p Phase) String() string {
	switch p {
	case PhasePreflop:
		return "preflop"
	case PhaseFlop:
		return "flop"
	case PhaseTurn:
		return "turn"
	case PhaseRiver:
		return "river"
	case PhaseHandOver:
		return "hand_over"
	}
	return "unknown"
}

type ActionType int

const (
	ActionFold ActionType = iota
	ActionCheck
	ActionCall
	ActionRaise
	ActionAllIn
)

type Action struct {
	Type   ActionType `json:"type"`
	Amount int        `json:"amount"` // total bet-to-level for Raise
}

type Player struct {
	SessionID    string
	Nickname     string
	Chips        int
	HoleCards    []Card
	Status       PlayerStatus
	BetThisRound int // chips committed in the current street
	Contributed  int // chips committed in the whole hand
	HasActed     bool
	Disconnected bool
}

type PlayerConfig struct {
	SessionID string
	Nickname  string
	Chips     int
}

type SidePot struct {
	Amount   int   `json:"amount"`
	Eligible []int `json:"eligible"`
}

type PotWinner struct {
	PlayerIdx int       `json:"player_idx"`
	Nickname  string    `json:"nickname"`
	Amount    int       `json:"amount"`
	Hand      HandValue `json:"hand"`
	Cards     []Card    `json:"cards"`
}

type HandResult struct {
	Showdown  bool        `json:"showdown"`
	Pots      []SidePot   `json:"pots"`
	Winners   []PotWinner `json:"winners"`
	Community []Card      `json:"community"`
}

type PublicPlayer struct {
	SessionID    string       `json:"session_id"`
	Nickname     string       `json:"nickname"`
	Chips        int          `json:"chips"`
	Status       PlayerStatus `json:"status"`
	BetThisRound int          `json:"bet_this_round"`
	HasActed     bool         `json:"has_acted"`
	Disconnected bool         `json:"disconnected"`
}

type PublicState struct {
	HandNumber int            `json:"hand_number"`
	Phase      Phase          `json:"phase"`
	Community  []Card         `json:"community"`
	Pot        int            `json:"pot"`
	CurrentBet int            `json:"current_bet"`
	MinRaise   int            `json:"min_raise"`
	DealerIdx  int            `json:"dealer_idx"`
	CurrentIdx int            `json:"current_idx"`
	Players    []PublicPlayer `json:"players"`
	SmallBlind int            `json:"small_blind"`
	BigBlind   int            `json:"big_blind"`
	HandOver   bool           `json:"hand_over"`
}

// Engine is a pure, network-free Texas Hold'em state machine (RNF4.1).
type Engine struct {
	players    []*Player
	dealerIdx  int
	currentIdx int
	deck       []Card
	community  []Card
	pot        int
	sidePots   []SidePot
	phase      Phase
	currentBet int
	lastRaise  int
	handNumber int
	smallBlind int
	bigBlind   int
	handOver   bool
	result     *HandResult
}

func NewEngine(configs []PlayerConfig, smallBlind, bigBlind int) *Engine {
	if smallBlind < 1 {
		smallBlind = 1
	}
	if bigBlind < smallBlind*2 {
		bigBlind = smallBlind * 2
	}
	players := make([]*Player, len(configs))
	for i, c := range configs {
		players[i] = &Player{
			SessionID: c.SessionID,
			Nickname:  c.Nickname,
			Chips:     c.Chips,
			Status:    Active,
		}
	}
	return &Engine{
		players:    players,
		dealerIdx:  -1,
		currentIdx: -1,
		smallBlind: smallBlind,
		bigBlind:   bigBlind,
	}
}

func (e *Engine) Players() []*Player  { return e.players }
func (e *Engine) Phase() Phase        { return e.phase }
func (e *Engine) Pot() int            { return e.pot }
func (e *Engine) HandNumber() int     { return e.handNumber }
func (e *Engine) IsHandOver() bool    { return e.handOver }
func (e *Engine) Result() *HandResult { return e.result }
func (e *Engine) Community() []Card   { return e.community }
func (e *Engine) DealerIdx() int      { return e.dealerIdx }
func (e *Engine) CurrentIdx() int     { return e.currentIdx }
func (e *Engine) SmallBlind() int     { return e.smallBlind }
func (e *Engine) BigBlind() int       { return e.bigBlind }
func (e *Engine) HoleCards(idx int) []Card {
	if idx < 0 || idx >= len(e.players) {
		return nil
	}
	return e.players[idx].HoleCards
}

func (e *Engine) CurrentBet() int { return e.currentBet }

// MinRaise returns the minimum raise increment for the current street.
func (e *Engine) MinRaise() int { return e.lastRaise }

func (e *Engine) PlayerIndex(sessionID string) int {
	for i, p := range e.players {
		if p.SessionID == sessionID {
			return i
		}
	}
	return -1
}

// StartHand resets all hand state, rotates the dealer, posts blinds and deals.
func (e *Engine) StartHand() error {
	return e.startHand(-1)
}

// StartHandWithDealer starts a new hand with the given player as the dealer
// instead of rotating. Used to honor a chosen blind assignment in heads-up
// games (there the dealer is the small blind).
func (e *Engine) StartHandWithDealer(dealerIdx int) error {
	return e.startHand(dealerIdx)
}

func (e *Engine) startHand(dealerOverride int) error {
	e.handNumber++
	e.community = nil
	e.pot = 0
	e.sidePots = nil
	e.currentBet = 0
	e.lastRaise = 0
	e.phase = PhasePreflop
	e.handOver = false
	e.result = nil
	e.currentIdx = -1

	for _, p := range e.players {
		p.HoleCards = nil
		p.BetThisRound = 0
		p.Contributed = 0
		p.HasActed = false
		if p.Chips > 0 {
			p.Status = Active
		} else {
			p.Status = Spectator
		}
	}

	active := e.activeIndices()
	if len(active) < 2 {
		return ErrNotEnoughPlayers
	}

	n := len(e.players)
	if dealerOverride >= 0 {
		e.dealerIdx = dealerOverride % n
	} else if e.dealerIdx < 0 {
		e.dealerIdx = active[0]
	} else {
		for {
			e.dealerIdx = (e.dealerIdx + 1) % n
			if e.players[e.dealerIdx].Status != Spectator {
				break
			}
		}
	}

	deck := NewDeck()
	if err := SecureShuffle(deck); err != nil {
		return err
	}
	e.deck = deck

	// Deal 2 hole cards to every non-spectator.
	for _, idx := range active {
		hole := make([]Card, 2)
		for k := 0; k < 2; k++ {
			hole[k], e.deck = e.deck[0], e.deck[1:]
		}
		e.players[idx].HoleCards = hole
	}

	// Blinds: heads-up => dealer is SB; otherwise SB = dealer+1, BB = dealer+2.
	sbIdx, bbIdx := e.blindPositions(active)
	e.postBlind(sbIdx, e.smallBlind)
	e.postBlind(bbIdx, e.bigBlind)
	e.currentBet = e.players[bbIdx].BetThisRound
	e.lastRaise = e.bigBlind

	e.currentIdx = e.firstToAct(active, bbIdx)
	e.advanceIfRoundComplete()
	return nil
}

func (e *Engine) blindPositions(active []int) (int, int) {
	if len(active) == 2 {
		// Heads-up: dealer is SB, other player is BB.
		return e.dealerIdx, e.nextSeat(e.dealerIdx)
	}
	sb := e.nextSeat(e.dealerIdx)
	return sb, e.nextSeat(sb)
}

func (e *Engine) nextSeat(idx int) int {
	n := len(e.players)
	cur := idx
	for {
		cur = (cur + 1) % n
		if cur == idx {
			return idx
		}
		if e.players[cur].Status != Spectator {
			return cur
		}
	}
}

func (e *Engine) firstToAct(active []int, bbIdx int) int {
	if len(active) == 2 && e.phase == PhasePreflop {
		// Heads-up preflop: dealer (SB) acts first.
		if e.players[e.dealerIdx].Status == Active {
			return e.dealerIdx
		}
	}
	start := bbIdx
	if e.phase != PhasePreflop {
		start = e.dealerIdx
	}
	return e.nextActiveSeat(start)
}

func (e *Engine) nextActiveSeat(idx int) int {
	n := len(e.players)
	cur := idx
	for {
		cur = (cur + 1) % n
		if e.players[cur].Status == Active {
			return cur
		}
		if cur == idx {
			return idx
		}
	}
}

func (e *Engine) postBlind(idx, amount int) {
	p := e.players[idx]
	if p.Chips <= amount {
		amount = p.Chips
	}
	p.Chips -= amount
	p.BetThisRound += amount
	p.Contributed += amount
	e.pot += amount
	p.HasActed = true
	if p.Chips == 0 {
		p.Status = AllIn
	}
}

func (e *Engine) activeIndices() []int {
	var out []int
	for i, p := range e.players {
		if p.Status != Spectator {
			out = append(out, i)
		}
	}
	return out
}

func (e *Engine) inHandIndices() []int {
	var out []int
	for i, p := range e.players {
		if p.Status != Folded && p.Status != Spectator {
			out = append(out, i)
		}
	}
	return out
}

// Act applies a player action with full server-side validation (RNF1.4).
func (e *Engine) Act(idx int, a Action) error {
	if e.handOver {
		return ErrHandOver
	}
	if idx < 0 || idx >= len(e.players) {
		return errors.New("player not found")
	}
	p := e.players[idx]
	if p.Status != Active {
		return ErrNotActive
	}
	if idx != e.currentIdx {
		return ErrNotYourTurn
	}

	switch a.Type {
	case ActionFold:
		p.Status = Folded
		p.HasActed = true

	case ActionCheck:
		if p.BetThisRound < e.currentBet {
			return ErrCannotCheck
		}
		p.HasActed = true

	case ActionCall:
		toCall := e.currentBet - p.BetThisRound
		if toCall <= 0 {
			return ErrNothingToCall
		}
		amount := toCall
		if amount > p.Chips {
			amount = p.Chips
		}
		p.Chips -= amount
		p.BetThisRound += amount
		p.Contributed += amount
		e.pot += amount
		if p.Chips == 0 {
			p.Status = AllIn
		}
		p.HasActed = true

	case ActionRaise:
		target := a.Amount
		if target <= e.currentBet {
			return ErrRaiseTooSmall
		}
		if target-p.BetThisRound > p.Chips {
			return ErrInsufficientChips
		}
		isAllIn := target-p.BetThisRound == p.Chips
		if target-e.currentBet < e.lastRaise && !isAllIn {
			return ErrRaiseTooSmall
		}
		amount := target - p.BetThisRound
		p.Chips -= amount
		p.BetThisRound += amount
		p.Contributed += amount
		e.pot += amount
		if p.Chips == 0 {
			p.Status = AllIn
		}
		e.lastRaise = target - e.currentBet
		e.currentBet = target
		p.HasActed = true

	case ActionAllIn:
		if p.Chips <= 0 {
			return errors.New("no chips to go all-in with")
		}
		amount := p.Chips
		p.Chips = 0
		p.BetThisRound += amount
		p.Contributed += amount
		e.pot += amount
		if p.BetThisRound > e.currentBet {
			inc := p.BetThisRound - e.currentBet
			if inc >= e.lastRaise {
				e.lastRaise = inc
			}
			e.currentBet = p.BetThisRound
		}
		p.Status = AllIn
		p.HasActed = true

	default:
		return errors.New("unknown action")
	}

	e.afterAction()
	return nil
}

// FoldForce folds a player regardless of whose turn it is (used for kicks).
func (e *Engine) FoldForce(idx int) {
	if idx < 0 || idx >= len(e.players) {
		return
	}
	p := e.players[idx]
	if p.Status != Active {
		return
	}
	p.Status = Folded
	p.HasActed = true
	if idx == e.currentIdx {
		e.afterAction()
	}
}

// AutoAction returns the action the current player would take on timeout
// (RF3.10): fold when facing a bet, otherwise check.
func (e *Engine) AutoAction() Action {
	if e.currentIdx < 0 || e.handOver {
		return Action{Type: ActionFold}
	}
	p := e.players[e.currentIdx]
	if p.BetThisRound < e.currentBet {
		return Action{Type: ActionFold}
	}
	return Action{Type: ActionCheck}
}

func (e *Engine) afterAction() {
	if e.handOver {
		return
	}
	if len(e.inHandIndices()) <= 1 {
		e.endHand(false)
		return
	}
	advanced := e.advanceIfRoundComplete()
	if e.handOver {
		return
	}
	if !advanced {
		e.currentIdx = e.nextActive(e.currentIdx)
	}
}

func (e *Engine) nextActive(idx int) int {
	n := len(e.players)
	cur := idx
	for {
		cur = (cur + 1) % n
		if e.players[cur].Status == Active {
			return cur
		}
		if cur == idx {
			return idx
		}
	}
}

func (e *Engine) roundComplete() bool {
	for _, p := range e.players {
		if p.Status == Active {
			if p.BetThisRound < e.currentBet || !p.HasActed {
				return false
			}
		}
	}
	// With no active players left (all all-in/folded) the round is complete.
	return true
}

// advanceIfRoundComplete deals the next street, or ends the hand, as long as
// the betting round is complete (e.g. everyone all-in runs the board out).
func (e *Engine) advanceIfRoundComplete() bool {
	progressed := false
	for !e.handOver && e.roundComplete() {
		switch e.phase {
		case PhasePreflop:
			e.dealCommunity(3)
			e.phase = PhaseFlop
		case PhaseFlop:
			e.dealCommunity(1)
			e.phase = PhaseTurn
		case PhaseTurn:
			e.dealCommunity(1)
			e.phase = PhaseRiver
		case PhaseRiver:
			e.endHand(true)
		default:
			return progressed
		}
		if e.handOver {
			return true
		}
		e.beginStreet()
		progressed = true
	}
	return progressed
}

func (e *Engine) beginStreet() {
	for _, p := range e.players {
		p.BetThisRound = 0
		p.HasActed = false
	}
	e.currentBet = 0
	e.lastRaise = 0
	active := e.activeIndices()
	if len(active) == 0 {
		return
	}
	e.currentIdx = e.firstToAct(active, e.dealerIdx)
}

func (e *Engine) dealCommunity(count int) {
	cards := make([]Card, count)
	for i := 0; i < count; i++ {
		cards[i], e.deck = e.deck[0], e.deck[1:]
	}
	e.community = append(e.community, cards...)
}

// computeSidePots builds main + side pots from total contributions.
// Folded players' chips stay in the pot (they count toward amounts) but they
// are not eligible to win anything (RF3.6).
func (e *Engine) computeSidePots() []SidePot {
	var levels []int
	seen := map[int]bool{}
	for _, p := range e.players {
		if p.Contributed > 0 && !seen[p.Contributed] {
			seen[p.Contributed] = true
			levels = append(levels, p.Contributed)
		}
	}
	sort.Ints(levels)

	var pots []SidePot
	prev := 0
	for _, l := range levels {
		amount, contributors := 0, 0
		var eligible []int
		for i, p := range e.players {
			if p.Contributed >= l {
				contributors++
				amount += l - prev
				if p.Status != Folded {
					eligible = append(eligible, i)
				}
			}
		}
		if len(eligible) == 0 {
			// Dead money (e.g. a folded player who bet more than anyone
			// matched): goes to whoever wins the hand.
			eligible = e.inHandIndices()
		}
		if amount > 0 {
			pots = append(pots, SidePot{Amount: amount, Eligible: eligible})
		}
		prev = l
	}
	return pots
}

func (e *Engine) endHand(showdown bool) {
	e.phase = PhaseHandOver
	e.handOver = true
	pots := e.computeSidePots()
	res := &HandResult{Showdown: showdown, Pots: pots, Community: append([]Card{}, e.community...)}

	bestHand := map[int]HandValue{}
	if showdown {
		for i, p := range e.players {
			if p.Status != Folded && p.Status != Spectator {
				seven := append(append([]Card{}, p.HoleCards...), e.community...)
				bestHand[i] = BestHand(seven)
			}
		}
	}
	winnersByPlayer := map[int]*PotWinner{}
	var order []int

	for _, pot := range pots {
		best := HandValue{Category: HighCard - 1}
		var winners []int
		for _, idx := range pot.Eligible {
			hv := bestHand[idx]
			switch Compare(hv, best) {
			case 1:
				best = hv
				winners = []int{idx}
			case 0:
				winners = append(winners, idx)
			}
		}
		// Odd chips go to the winner closest to the dealer button's left.
		sort.SliceStable(winners, func(i, j int) bool {
			return e.distanceFromDealer(winners[i]) < e.distanceFromDealer(winners[j])
		})
		base, rem := pot.Amount/len(winners), pot.Amount%len(winners)
		for i, w := range winners {
			amt := base
			if i < rem {
				amt++
			}
			e.players[w].Chips += amt
			if winnersByPlayer[w] == nil {
				winnersByPlayer[w] = &PotWinner{PlayerIdx: w, Nickname: e.players[w].Nickname}
				order = append(order, w)
			}
			winnersByPlayer[w].Amount += amt
			if showdown && len(winnersByPlayer[w].Cards) == 0 {
				winnersByPlayer[w].Cards = append([]Card{}, e.players[w].HoleCards...)
				winnersByPlayer[w].Hand = best
			}
		}
	}

	for _, w := range order {
		res.Winners = append(res.Winners, *winnersByPlayer[w])
	}
	e.result = res
}

func (e *Engine) distanceFromDealer(idx int) int {
	return (idx - e.dealerIdx + len(e.players)) % len(e.players)
}

// MarkSpectators flags players with 0 chips as spectators (RF3.9).
func (e *Engine) MarkSpectators() {
	for _, p := range e.players {
		if p.Chips <= 0 {
			p.Status = Spectator
		}
	}
}

// MatchOver reports whether a match has ended (0 or 1 players left with chips).
func (e *Engine) MatchOver() bool {
	withChips := 0
	for _, p := range e.players {
		if p.Status != Spectator && p.Chips > 0 {
			withChips++
		}
	}
	return withChips <= 1
}

func (e *Engine) ChampionIdx() int {
	for i, p := range e.players {
		if p.Status != Spectator && p.Chips > 0 {
			return i
		}
	}
	return -1
}

func (e *Engine) PublicState() *PublicState {
	ps := &PublicState{
		HandNumber: e.handNumber,
		Phase:      e.phase,
		Community:  append([]Card{}, e.community...),
		Pot:        e.pot,
		CurrentBet: e.currentBet,
		MinRaise:   e.lastRaise,
		DealerIdx:  e.dealerIdx,
		CurrentIdx: e.currentIdx,
		SmallBlind: e.smallBlind,
		BigBlind:   e.bigBlind,
		HandOver:   e.handOver,
	}
	for _, p := range e.players {
		ps.Players = append(ps.Players, PublicPlayer{
			SessionID:    p.SessionID,
			Nickname:     p.Nickname,
			Chips:        p.Chips,
			Status:       p.Status,
			BetThisRound: p.BetThisRound,
			HasActed:     p.HasActed,
			Disconnected: p.Disconnected,
		})
	}
	return ps
}
