package game

import "fmt"

type Suit uint8

const (
	Spades Suit = iota
	Hearts
	Diamonds
	Clubs
)

type Rank uint8

const (
	Two Rank = iota + 2
	Three
	Four
	Five
	Six
	Seven
	Eight
	Nine
	Ten
	Jack
	Queen
	King
	Ace
)

type Card struct {
	Rank Rank `json:"rank"`
	Suit Suit `json:"suit"`
}

var rankChars = []byte("23456789TJQKA")
var suitChars = []byte("SHDC")
var suitSymbols = []byte("♠♥♦♣")

func (c Card) String() string {
	if c.Rank < Two || c.Rank > Ace || c.Suit > Clubs {
		return "??"
	}
	return fmt.Sprintf("%c%c", rankChars[c.Rank-Two], suitChars[c.Suit])
}

func (c Card) Symbol() string {
	if c.Suit > Clubs {
		return "?"
	}
	return string(suitSymbols[c.Suit])
}

func (c Card) Red() bool {
	return c.Suit == Hearts || c.Suit == Diamonds
}
