package game

import "sort"

// HandCategory ranks hands from worst (HighCard) to best (StraightFlush).
type HandCategory int

const (
	HighCard HandCategory = iota
	OnePair
	TwoPair
	ThreeOfAKind
	Straight
	Flush
	FullHouse
	FourOfAKind
	StraightFlush
)

func (c HandCategory) String() string {
	switch c {
	case HighCard:
		return "High Card"
	case OnePair:
		return "One Pair"
	case TwoPair:
		return "Two Pair"
	case ThreeOfAKind:
		return "Three of a Kind"
	case Straight:
		return "Straight"
	case Flush:
		return "Flush"
	case FullHouse:
		return "Full House"
	case FourOfAKind:
		return "Four of a Kind"
	case StraightFlush:
		return "Straight Flush"
	}
	return "Unknown"
}

// HandValue is a comparable evaluation of a 5-card poker hand.
// Tie holds tiebreaker ranks in order of importance (descending).
type HandValue struct {
	Category HandCategory `json:"category"`
	Tie      [5]int       `json:"tie"`
}

// Evaluate5 evaluates exactly 5 cards.
func Evaluate5(cards [5]Card) HandValue {
	ranks := [5]int{}
	for i, c := range cards {
		ranks[i] = int(c.Rank)
	}
	rs := ranks[:]
	sort.Sort(sort.Reverse(sort.IntSlice(rs)))

	flush := true
	for i := 1; i < 5; i++ {
		if cards[i].Suit != cards[0].Suit {
			flush = false
			break
		}
	}

	straight := true
	for i := 1; i < 5; i++ {
		if ranks[i] != ranks[i-1]-1 {
			straight = false
			break
		}
	}
	wheel := ranks[0] == int(Ace) && ranks[1] == 5 && ranks[2] == 4 && ranks[3] == 3 && ranks[4] == 2
	if wheel {
		straight = true
		rs = []int{5, 4, 3, 2, 1}
	}

	counts := map[int]int{}
	for _, r := range ranks {
		counts[r]++
	}

	four, three, pair1, pair2 := 0, 0, 0, 0
	var kickers []int
	for _, r := range ranks {
		switch counts[r] {
		case 4:
			four = r
		case 3:
			three = r
		case 2:
			if pair1 == 0 {
				pair1 = r
			} else if pair2 == 0 && r != pair1 {
				pair2 = r
			}
		default:
			kickers = append(kickers, r)
		}
	}

	hv := HandValue{}
	if flush && straight {
		hv.Category = StraightFlush
		hv.Tie[0] = rs[0]
		return hv
	}
	if four > 0 {
		hv.Category = FourOfAKind
		hv.Tie[0] = four
		hv.Tie[1] = kickers[0]
		return hv
	}
	if three > 0 && pair1 > 0 {
		hv.Category = FullHouse
		hv.Tie[0] = three
		hv.Tie[1] = pair1
		return hv
	}
	if flush {
		hv.Category = Flush
		copy(hv.Tie[:], rs[:5])
		return hv
	}
	if straight {
		hv.Category = Straight
		hv.Tie[0] = rs[0]
		return hv
	}
	if three > 0 {
		hv.Category = ThreeOfAKind
		hv.Tie[0] = three
		hv.Tie[1] = kickers[0]
		hv.Tie[2] = kickers[1]
		return hv
	}
	if pair1 > 0 && pair2 > 0 {
		hv.Category = TwoPair
		hv.Tie[0] = pair1
		hv.Tie[1] = pair2
		hv.Tie[2] = kickers[0]
		return hv
	}
	if pair1 > 0 {
		hv.Category = OnePair
		hv.Tie[0] = pair1
		hv.Tie[1] = kickers[0]
		hv.Tie[2] = kickers[1]
		hv.Tie[3] = kickers[2]
		return hv
	}
	hv.Category = HighCard
	copy(hv.Tie[:], rs[:5])
	return hv
}

// BestHand evaluates the best 5-card hand out of 5..7 cards.
func BestHand(cards []Card) HandValue {
	n := len(cards)
	if n < 5 || n > 7 {
		panic("BestHand expects 5 to 7 cards")
	}
	best := HandValue{Category: HighCard - 1}
	idx := make([]int, 5)
	var choose func(start, depth int)
	choose = func(start, depth int) {
		if depth == 5 {
			var five [5]Card
			for i, k := range idx {
				five[i] = cards[k]
			}
			hv := Evaluate5(five)
			if Compare(hv, best) > 0 {
				best = hv
			}
			return
		}
		for i := start; i <= n-(5-depth); i++ {
			idx[depth] = i
			choose(i+1, depth+1)
		}
	}
	choose(0, 0)
	return best
}

// Compare returns 1 if a > b, -1 if a < b, 0 if equal.
func Compare(a, b HandValue) int {
	if a.Category != b.Category {
		if a.Category > b.Category {
			return 1
		}
		return -1
	}
	for i := 0; i < 5; i++ {
		if a.Tie[i] != b.Tie[i] {
			if a.Tie[i] > b.Tie[i] {
				return 1
			}
			return -1
		}
	}
	return 0
}
