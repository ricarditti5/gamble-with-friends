import type { Card, PublicState } from "../types";
import { PHASE_NAMES } from "../types";
import { CommunityCards } from "./CommunityCards";
import { PlayerSeat } from "./PlayerSeat";

// Mirrors the engine's blind/dealer rules for the badges.
function blindPositions(state: PublicState): { sb: number; bb: number } {
  const active: number[] = [];
  state.players.forEach((p, i) => {
    if (p.status !== 3) active.push(i);
  });
  const n = state.players.length;
  const next = (from: number) => {
    let cur = from;
    do {
      cur = (cur + 1) % n;
    } while (state.players[cur].status === 3);
    return cur;
  };
  if (active.length === 2) {
    return { sb: state.dealer_idx, bb: active.find((i) => i !== state.dealer_idx) ?? -1 };
  }
  const sb = next(state.dealer_idx);
  return { sb, bb: next(sb) };
}

// Seats are arranged on an ellipse; the viewer is always at the bottom.
export function Table2D({ state, yourIdx, yourCards }: {
  state: PublicState;
  yourIdx: number;
  yourCards: Card[];
}) {
  const n = state.players.length;
  const pos = (i: number) => {
    const angle = ((-90 + ((i - yourIdx) * 360) / n) * Math.PI) / 180;
    return {
      x: 50 + 40 * Math.cos(angle),
      y: 50 + 26 * Math.sin(angle),
    };
  };
  const { sb, bb } = blindPositions(state);

  return (
    <div className="table-wrap">
      <div className="poker-table">
        <div className="table-center">
          <CommunityCards community={state.community} />
          <div className="pot">
            Pot <b>{state.pot.toLocaleString("pt-PT")}</b>
          </div>
          <div className="blinds-line">
            Blinds {state.small_blind}/{state.big_blind} · {PHASE_NAMES[state.phase]}
          </div>
        </div>

        {state.players.map((p, i) => {
          const { x, y } = pos(i);
          return (
            <div key={p.session_id} className="seat-pos" style={{ left: `${x}%`, top: `${y}%` }}>
              <PlayerSeat
                player={p}
                isDealer={state.dealer_idx === i}
                isSB={sb === i}
                isBB={bb === i}
                isTurn={state.current_idx === i && !state.hand_over}
                isYou={i === yourIdx}
                holeCards={i === yourIdx ? yourCards : []}
              />
            </div>
          );
        })}
      </div>
    </div>
  );
}
