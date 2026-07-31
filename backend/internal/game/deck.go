package game

import (
	"crypto/rand"
	"math/big"
)

// NewDeck returns a fresh, ordered 52-card deck.
func NewDeck() []Card {
	deck := make([]Card, 0, 52)
	for s := Suit(0); s <= Clubs; s++ {
		for r := Two; r <= Ace; r++ {
			deck = append(deck, Card{Rank: r, Suit: s})
		}
	}
	return deck
}

// SecureShuffle shuffles the deck in place using a cryptographically secure
// RNG (crypto/rand), never math/rand (RNF1.1).
func SecureShuffle(deck []Card) error {
	for i := len(deck) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return err
		}
		j := int(n.Int64())
		deck[i], deck[j] = deck[j], deck[i]
	}
	return nil
}
