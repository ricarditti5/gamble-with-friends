import type { Card } from "../types";

const RANKS = "23456789TJQKA";
const SUITS = ["♠", "♥", "♦", "♣"];

export function rankChar(rank: number): string {
  return RANKS[rank - 2] ?? "?";
}

export function suitChar(suit: number): string {
  return SUITS[suit] ?? "?";
}

export function isRed(suit: number): boolean {
  return suit === 1 || suit === 2;
}

export function CardView({ card, faceDown, small }: { card: Card; faceDown?: boolean; small?: boolean }) {
  if (faceDown) {
    return <div className={`card card-back ${small ? "small" : ""}`} />;
  }
  const red = isRed(card.suit);
  return (
    <div className={`card ${red ? "red" : "black"} ${small ? "small" : ""}`}>
      <span className="card-rank">{rankChar(card.rank)}</span>
      <span className="card-suit">{suitChar(card.suit)}</span>
    </div>
  );
}
