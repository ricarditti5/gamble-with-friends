package game

import "testing"

func newTestEngine(stacks []int, sb, bb int) *Engine {
	cfgs := make([]PlayerConfig, len(stacks))
	for i, s := range stacks {
		cfgs[i] = PlayerConfig{SessionID: string(rune('A' + i)), Nickname: string(rune('A' + i)), Chips: s}
	}
	return NewEngine(cfgs, sb, bb)
}

// rigDeck forces the next streets to deal the given community cards.
func rigDeck(e *Engine, community []Card) {
	used := map[Card]bool{}
	for _, c := range community {
		used[c] = true
	}
	for _, p := range e.Players() {
		for _, c := range p.HoleCards {
			used[c] = true
		}
	}
	var rest []Card
	for _, c := range NewDeck() {
		if !used[c] {
			rest = append(rest, c)
		}
	}
	e.deck = append(append([]Card{}, community...), rest...)
}

func runOutWithChecks(e *Engine) {
	for !e.IsHandOver() {
		e.Act(e.CurrentIdx(), Action{Type: ActionCheck})
	}
}

func TestBlindsAndTurnOrder(t *testing.T) {
	e := newTestEngine([]int{1000, 1000, 1000}, 5, 10)
	if err := e.StartHand(); err != nil {
		t.Fatal(err)
	}
	// dealer=0, SB=1, BB=2, preflop action starts left of BB => dealer (0)
	if e.DealerIdx() != 0 {
		t.Errorf("dealer = %d, want 0", e.DealerIdx())
	}
	if got := e.Players()[1].BetThisRound; got != 5 {
		t.Errorf("SB bet = %d, want 5", got)
	}
	if got := e.Players()[2].BetThisRound; got != 10 {
		t.Errorf("BB bet = %d, want 10", got)
	}
	if got := e.CurrentIdx(); got != 0 {
		t.Errorf("first to act = %d, want 0", got)
	}
	if len(e.Players()[0].HoleCards) != 2 || len(e.Players()[1].HoleCards) != 2 {
		t.Errorf("expected 2 hole cards each")
	}
}

func TestHeadsUpTurnOrder(t *testing.T) {
	e := newTestEngine([]int{1000, 1000}, 5, 10)
	e.StartHand()
	// heads-up: dealer=0 is SB, BB=1, dealer acts first preflop
	if e.DealerIdx() != 0 || e.CurrentIdx() != 0 {
		t.Errorf("heads-up: dealer=%d current=%d, want 0,0", e.DealerIdx(), e.CurrentIdx())
	}
	// BB posts 10, SB posts 5 -> current bet is 10
	if e.Pot() != 15 {
		t.Errorf("pot = %d, want 15", e.Pot())
	}
}

func TestFoldEveryone(t *testing.T) {
	e := newTestEngine([]int{1000, 1000, 1000}, 5, 10)
	e.StartHand()
	// dealer(0) folds, SB(1) folds, BB(2) wins without showdown
	e.Act(0, Action{Type: ActionFold})
	e.Act(1, Action{Type: ActionFold})
	if !e.IsHandOver() {
		t.Fatal("hand should be over")
	}
	res := e.Result()
	if res.Showdown {
		t.Fatal("should not be a showdown")
	}
	if len(res.Winners) != 1 || res.Winners[0].PlayerIdx != 2 {
		t.Errorf("winner should be BB(2), got %+v", res.Winners)
	}
	if got := e.Players()[2].Chips; got != 1005 {
		t.Errorf("BB chips = %d, want 1005", got)
	}
}

func TestCheckRoundAdvancesToFlop(t *testing.T) {
	e := newTestEngine([]int{1000, 1000, 1000}, 5, 10)
	e.StartHand()
	// preflop: 0 calls 10, 1 calls 5 more, 2 checks
	e.Act(0, Action{Type: ActionCall})
	e.Act(1, Action{Type: ActionCall})
	e.Act(2, Action{Type: ActionCheck})
	if e.Phase() != PhaseFlop {
		t.Fatalf("phase = %v, want flop", e.Phase())
	}
	if len(e.Community()) != 3 {
		t.Fatalf("community = %d cards, want 3", len(e.Community()))
	}
	// currentBet resets for the new street; first to act = left of dealer (1)
	if e.CurrentBet() != 0 {
		t.Errorf("currentBet = %d, want 0", e.CurrentBet())
	}
	if e.CurrentIdx() != 1 {
		t.Errorf("first to act on flop = %d, want 1", e.CurrentIdx())
	}
	if e.Pot() != 30 {
		t.Errorf("pot = %d, want 30", e.Pot())
	}
}

func TestRaiseValidation(t *testing.T) {
	e := newTestEngine([]int{1000, 1000, 1000}, 5, 10)
	e.StartHand()
	// player 0 raises to 30 (raise of 20 >= BB)
	if err := e.Act(0, Action{Type: ActionRaise, Amount: 30}); err != nil {
		t.Fatal(err)
	}
	// player 1 tries to raise to 35: increment of 5 < min raise 20 -> error
	if err := e.Act(1, Action{Type: ActionRaise, Amount: 35}); err != ErrRaiseTooSmall {
		t.Errorf("expected ErrRaiseTooSmall, got %v", err)
	}
	// player 1 min raise to 50 (increment 20) works
	if err := e.Act(1, Action{Type: ActionRaise, Amount: 50}); err != nil {
		t.Fatal(err)
	}
	// check not allowed when facing a bet
	if err := e.Act(2, Action{Type: ActionCheck}); err != ErrCannotCheck {
		t.Errorf("expected ErrCannotCheck, got %v", err)
	}
	// raise above chips rejected
	if err := e.Act(2, Action{Type: ActionRaise, Amount: 5000}); err != ErrInsufficientChips {
		t.Errorf("expected ErrInsufficientChips, got %v", err)
	}
	// wrong turn rejected
	if err := e.Act(0, Action{Type: ActionFold}); err != ErrNotYourTurn {
		t.Errorf("expected ErrNotYourTurn, got %v", err)
	}
}

func TestShortAllInCreateSidePot(t *testing.T) {
	e := newTestEngine([]int{100, 500, 500}, 5, 10)
	e.StartHand()
	// dealer 0: all-in raise to 100
	if err := e.Act(0, Action{Type: ActionRaise, Amount: 100}); err != nil {
		t.Fatal(err)
	}
	// 1 calls 95 more, 2 (BB) calls 90 more
	if err := e.Act(1, Action{Type: ActionCall}); err != nil {
		t.Fatal(err)
	}
	if err := e.Act(2, Action{Type: ActionCall}); err != nil {
		t.Fatal(err)
	}
	if e.Phase() != PhaseFlop {
		t.Fatalf("phase = %v, want flop (0 is all-in, 1 and 2 still play)", e.Phase())
	}
	// 1 and 2 bet the flop for a side pot
	if err := e.Act(1, Action{Type: ActionRaise, Amount: 200}); err != nil {
		t.Fatal(err)
	}
	if err := e.Act(2, Action{Type: ActionCall}); err != nil {
		t.Fatal(err)
	}
	// all-in run-out for the remaining streets
	runOutWithChecks(e)
	if !e.IsHandOver() {
		t.Fatal("hand should be over")
	}
	res := e.Result()
	// main pot: 300 (100 each), side pot: 400 (200 extra each from 1 and 2)
	if len(res.Pots) != 2 {
		t.Fatalf("expected 2 pots, got %+v", res.Pots)
	}
	if res.Pots[0].Amount != 300 || res.Pots[1].Amount != 400 {
		t.Errorf("pots = %d/%d, want 300/400", res.Pots[0].Amount, res.Pots[1].Amount)
	}
	// total chips must be conserved: 100+500+500 = 1100
	total := 0
	for _, p := range e.Players() {
		total += p.Chips
	}
	if total != 1100 {
		t.Errorf("chips not conserved: total = %d, want 1100", total)
	}
}

func TestShowdownWinnerTakesPot(t *testing.T) {
	e := newTestEngine([]int{1000, 1000}, 5, 10)
	e.StartHand()
	// force specific hands: player 0 gets A♠ A♥, player 1 gets 2♠ 3♥
	e.Players()[0].HoleCards = []Card{C(Ace, Spades), C(Ace, Hearts)}
	e.Players()[1].HoleCards = []Card{C(Two, Spades), C(Three, Hearts)}
	// community: K Q J 9 8 mixed suits (no pair for player 1)
	rigDeck(e, []Card{C(King, Clubs), C(Queen, Diamonds), C(Jack, Spades), C(Nine, Hearts), C(Eight, Clubs)})

	// dealer (0, SB) calls 5, BB checks, then run the board out
	e.Act(0, Action{Type: ActionCall})
	e.Act(1, Action{Type: ActionCheck})
	runOutWithChecks(e)
	res := e.Result()
	if !res.Showdown {
		t.Fatal("expected showdown")
	}
	if len(res.Winners) != 1 || res.Winners[0].PlayerIdx != 0 {
		t.Errorf("player 0 (pair of aces) should win, got %+v", res.Winners)
	}
	if got := e.Players()[0].Chips; got != 1010 {
		t.Errorf("winner chips = %d, want 1010", got)
	}
}

func TestTieSplitsPot(t *testing.T) {
	e := newTestEngine([]int{1000, 1000}, 5, 10)
	e.StartHand()
	// identical hands: both play the board
	e.Players()[0].HoleCards = []Card{C(Ace, Spades), C(Two, Clubs)}
	e.Players()[1].HoleCards = []Card{C(Ace, Hearts), C(Three, Clubs)}
	// board is a straight, both split
	rigDeck(e, []Card{C(Nine, Diamonds), C(Eight, Hearts), C(Seven, Spades), C(Six, Clubs), C(Five, Hearts)})

	e.Act(0, Action{Type: ActionCall})
	e.Act(1, Action{Type: ActionCheck})
	runOutWithChecks(e)
	res := e.Result()
	if len(res.Winners) != 2 {
		t.Fatalf("expected 2 winners (split), got %+v", res.Winners)
	}
	// Split pot: each player gets their own 10 back, net unchanged.
	if e.Players()[0].Chips != 1000 || e.Players()[1].Chips != 1000 {
		t.Errorf("chips = %d/%d, want 1000/1000", e.Players()[0].Chips, e.Players()[1].Chips)
	}
}

func TestMatchOverAndSpectator(t *testing.T) {
	e := newTestEngine([]int{500, 500}, 5, 10)
	e.StartHand()
	e.Players()[0].HoleCards = []Card{C(Ace, Spades), C(Ace, Hearts)}
	e.Players()[1].HoleCards = []Card{C(Two, Spades), C(Three, Hearts)}
	rigDeck(e, []Card{C(King, Clubs), C(Queen, Diamonds), C(Jack, Spades), C(Nine, Hearts), C(Eight, Clubs)})

	// heads-up: SB (dealer) goes all-in, BB calls -> all-in run-out, player 0 wins everything
	e.Act(0, Action{Type: ActionRaise, Amount: 500})
	e.Act(1, Action{Type: ActionCall})
	runOutWithChecks(e)
	// player 0 has 1000, player 1 has 0
	e.MarkSpectators()
	if e.Players()[1].Status != Spectator {
		t.Errorf("player 1 should be spectator, got %v", e.Players()[1].Status)
	}
	if !e.MatchOver() {
		t.Error("match should be over")
	}
	if e.ChampionIdx() != 0 {
		t.Errorf("champion should be 0, got %d", e.ChampionIdx())
	}
}

func TestBlindRotation(t *testing.T) {
	e := newTestEngine([]int{1000, 1000, 1000}, 5, 10)
	e.StartHand()
	if e.DealerIdx() != 0 {
		t.Fatalf("hand 1 dealer = %d", e.DealerIdx())
	}
	// end the hand with folds
	e.Act(0, Action{Type: ActionFold})
	e.Act(1, Action{Type: ActionFold})
	if err := e.StartHand(); err != nil {
		t.Fatal(err)
	}
	if e.DealerIdx() != 1 {
		t.Errorf("hand 2 dealer = %d, want 1", e.DealerIdx())
	}
}

func TestAutoAction(t *testing.T) {
	e := newTestEngine([]int{1000, 1000, 1000}, 5, 10)
	e.StartHand()
	// player 0 is dealer, facing BB bet of 10 -> auto fold
	if a := e.AutoAction(); a.Type != ActionFold {
		t.Errorf("auto action facing bet = %v, want fold", a.Type)
	}
	// player 0 calls, player 1 calls, player 2 checks -> flop, nobody facing a bet
	e.Act(0, Action{Type: ActionCall})
	e.Act(1, Action{Type: ActionCall})
	e.Act(2, Action{Type: ActionCheck})
	if a := e.AutoAction(); a.Type != ActionCheck {
		t.Errorf("auto action with no bet = %v, want check", a.Type)
	}
}

func TestRaisesReopenBetting(t *testing.T) {
	e := newTestEngine([]int{1000, 1000, 1000}, 5, 10)
	e.StartHand()
	e.Act(0, Action{Type: ActionRaise, Amount: 30}) // to 30
	e.Act(1, Action{Type: ActionRaise, Amount: 50}) // to 50
	// player 2 calls 40 more
	e.Act(2, Action{Type: ActionCall})
	// player 0 faces 20 more -> must act again (round not complete)
	if e.IsHandOver() {
		t.Fatal("hand over too early")
	}
	if got := e.CurrentIdx(); got != 0 {
		t.Errorf("turn = %d, want 0 (betting reopened)", got)
	}
	e.Act(0, Action{Type: ActionCall})
	// betting reopens for 1 and 2 only if they face a raise; here 1 and 2
	// both matched 50, so the preflop round completes -> flop, first to act is 1
	if e.Phase() != PhaseFlop {
		t.Fatalf("phase = %v, want flop", e.Phase())
	}
	if e.CurrentIdx() != 1 || e.CurrentBet() != 0 {
		t.Errorf("after calls: turn=%d bet=%d, want turn=1 bet=0", e.CurrentIdx(), e.CurrentBet())
	}
	if e.Pot() != 150 {
		t.Errorf("pot = %d, want 150", e.Pot())
	}
}
