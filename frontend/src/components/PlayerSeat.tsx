import type { PublicPlayer } from "../types";
import { CardView } from "./CardView";
import type { Card } from "../types";

const STATUS_LABEL: Record<number, string | null> = {
  0: null,
  1: "Fold",
  2: "All-in",
  3: "Espectador",
};

export function PlayerSeat({ player, isDealer, isSB, isBB, isTurn, isYou, holeCards }: {
  player: PublicPlayer;
  isDealer: boolean;
  isSB: boolean;
  isBB: boolean;
  isTurn: boolean;
  isYou: boolean;
  holeCards: Card[];
}) {
  const folded = player.status === 1;
  const allIn = player.status === 2;
  const spectating = player.status === 3;
  const label = STATUS_LABEL[player.status];

  return (
    <div
      className={`seat ${isYou ? "you" : ""} ${folded ? "folded" : ""} ${allIn ? "all-in" : ""} ${spectating ? "spectator" : ""} ${isTurn ? "turn" : ""}`}
    >
      {isTurn && <div className="turn-indicator">A jogar…</div>}
      <div className="avatar">
        {player.nickname.slice(0, 1).toUpperCase()}
        {player.disconnected && <span className="offline-dot" title="Sem ligação" />}
      </div>
      <div className="seat-name" title={player.nickname}>
        {player.nickname}
      </div>
      <div className="seat-chips">🪙 {player.chips.toLocaleString("pt-PT")}</div>
      <div className="seat-badges">
        {isDealer && <span className="badge dealer" title="Dealer">D</span>}
        {isSB && <span className="badge sb" title="Small Blind">SB</span>}
        {isBB && <span className="badge bb" title="Big Blind">BB</span>}
        {allIn && <span className="badge ai">All-in</span>}
      </div>
      <div className="seat-cards">
        {holeCards.map((c, i) => (
          <CardView key={i} card={c} faceDown={!isYou} small />
        ))}
      </div>
      {label && <div className="seat-status">{label}</div>}
      {player.bet_this_round > 0 && (
        <div className="seat-bet">+{player.bet_this_round}</div>
      )}
    </div>
  );
}
