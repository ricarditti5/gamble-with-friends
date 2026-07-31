package game

import "testing"

func C(r Rank, s Suit) Card { return Card{Rank: r, Suit: s} }

func TestEvaluate5Categories(t *testing.T) {
	cases := []struct {
		name  string
		cards [5]Card
		want  HandCategory
	}{
		{"royal flush", [5]Card{C(Ace, Spades), C(King, Spades), C(Queen, Spades), C(Jack, Spades), C(Ten, Spades)}, StraightFlush},
		{"straight flush", [5]Card{C(Nine, Hearts), C(Eight, Hearts), C(Seven, Hearts), C(Six, Hearts), C(Five, Hearts)}, StraightFlush},
		{"quads", [5]Card{C(Ten, Spades), C(Ten, Hearts), C(Ten, Diamonds), C(Ten, Clubs), C(Three, Spades)}, FourOfAKind},
		{"full house", [5]Card{C(King, Spades), C(King, Hearts), C(King, Diamonds), C(Four, Clubs), C(Four, Spades)}, FullHouse},
		{"flush", [5]Card{C(Ace, Clubs), C(Jack, Clubs), C(Eight, Clubs), C(Five, Clubs), C(Two, Clubs)}, Flush},
		{"straight", [5]Card{C(Nine, Clubs), C(Eight, Spades), C(Seven, Hearts), C(Six, Diamonds), C(Five, Clubs)}, Straight},
		{"wheel straight", [5]Card{C(Ace, Spades), C(Five, Hearts), C(Four, Diamonds), C(Three, Clubs), C(Two, Spades)}, Straight},
		{"trips", [5]Card{C(Five, Spades), C(Five, Hearts), C(Five, Diamonds), C(Nine, Clubs), C(Two, Spades)}, ThreeOfAKind},
		{"two pair", [5]Card{C(Jack, Spades), C(Jack, Hearts), C(Eight, Diamonds), C(Eight, Clubs), C(Two, Spades)}, TwoPair},
		{"pair", [5]Card{C(Queen, Spades), C(Queen, Hearts), C(Jack, Diamonds), C(Five, Clubs), C(Two, Spades)}, OnePair},
		{"high card", [5]Card{C(Ace, Spades), C(Jack, Hearts), C(Eight, Diamonds), C(Five, Clubs), C(Two, Spades)}, HighCard},
	}
	for _, tc := range cases {
		got := Evaluate5(tc.cards)
		if got.Category != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got.Category, tc.want)
		}
	}
}

func TestWheelTiebreak(t *testing.T) {
	// Wheel straight (A-5) must tiebreak as 5-high, losing to a 6-high straight.
	wheel := Evaluate5([5]Card{C(Ace, Spades), C(Five, Hearts), C(Four, Diamonds), C(Three, Clubs), C(Two, Spades)})
	six := Evaluate5([5]Card{C(Six, Spades), C(Five, Hearts), C(Four, Diamonds), C(Three, Clubs), C(Two, Spades)})
	if Compare(six, wheel) <= 0 {
		t.Errorf("6-high straight should beat 5-high wheel, got %v vs %v", six, wheel)
	}
	if wheel.Tie[0] != 5 {
		t.Errorf("wheel tie should be 5-high, got %v", wheel.Tie[0])
	}
}

func TestCompareKickers(t *testing.T) {
	a := Evaluate5([5]Card{C(Ace, Spades), C(King, Hearts), C(Nine, Diamonds), C(Five, Clubs), C(Two, Spades)})
	b := Evaluate5([5]Card{C(Ace, Hearts), C(King, Diamonds), C(Nine, Clubs), C(Four, Spades), C(Two, Hearts)})
	if Compare(a, b) <= 0 {
		t.Errorf("A-K-9-5-2 should beat A-K-9-4-2")
	}
	if Compare(a, a) != 0 {
		t.Errorf("identical hands should tie")
	}
}

func TestBestHandOfSeven(t *testing.T) {
	// Board: A K Q J 10 of spades + hole 2d 3c -> royal flush
	cards := []Card{
		C(Ace, Spades), C(King, Spades), C(Queen, Spades), C(Jack, Spades), C(Ten, Spades),
		C(Two, Diamonds), C(Three, Clubs),
	}
	hv := BestHand(cards)
	if hv.Category != StraightFlush || hv.Tie[0] != int(Ace) {
		t.Errorf("expected royal flush, got %v", hv)
	}
}

func TestBestHandPicksFullHouse(t *testing.T) {
	// Board: 9 9 9 K K + hole 2 3 -> full house (not just trips)
	cards := []Card{
		C(Nine, Spades), C(Nine, Hearts), C(Nine, Diamonds), C(King, Spades), C(King, Hearts),
		C(Two, Clubs), C(Three, Clubs),
	}
	hv := BestHand(cards)
	if hv.Category != FullHouse {
		t.Errorf("expected full house, got %v", hv)
	}
}

func TestDeckUniquenessAndShuffle(t *testing.T) {
	deck := NewDeck()
	seen := map[Card]bool{}
	for _, c := range deck {
		if seen[c] {
			t.Fatalf("duplicate card %v", c)
		}
		seen[c] = true
	}
	if len(deck) != 52 {
		t.Fatalf("expected 52 cards, got %d", len(deck))
	}
	if err := SecureShuffle(deck); err != nil {
		t.Fatal(err)
	}
	diff := 0
	ordered := NewDeck()
	for i := range deck {
		if deck[i] != ordered[i] {
			diff++
		}
	}
	if diff < 40 {
		t.Errorf("shuffle barely changed the deck (%d positions differ), suspect broken RNG", diff)
	}
}
