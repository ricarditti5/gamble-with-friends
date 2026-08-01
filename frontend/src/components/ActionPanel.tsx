import { useEffect, useMemo, useState } from "react";
import type { PublicState } from "../types";

// No action timer: the game waits for the player to act. Bots and
// disconnected players are handled server-side.
export function ActionPanel({ state, yourIdx, onAction, disabled }: {
  state: PublicState;
  yourIdx: number;
  onAction: (type: "fold" | "check" | "call" | "raise" | "all_in", amount?: number) => void;
  disabled: boolean;
}) {
  const me = state.players[yourIdx];
  const toCall = me ? state.current_bet - me.bet_this_round : 0;
  const [raiseTarget, setRaiseTarget] = useState(0);

  const canAct = !disabled && me?.status === 0 && state.current_idx === yourIdx && !state.hand_over;

  const minRaiseTarget = useMemo(() => {
    if (!me) return 0;
    const base = Math.max(state.current_bet, me.bet_this_round);
    const inc = state.min_raise > 0 ? state.min_raise : state.big_blind;
    return base + inc;
  }, [me, state.current_bet, state.min_raise, state.big_blind]);

  const maxRaiseTarget = useMemo(() => (me ? me.chips + me.bet_this_round : 0), [me]);

  useEffect(() => {
    if (me) setRaiseTarget(Math.min(Math.max(minRaiseTarget, state.current_bet + 1), maxRaiseTarget));
  }, [minRaiseTarget, maxRaiseTarget, me, state.current_bet]);

  if (!me) return null;

  const betAmount = Math.max(0, raiseTarget - me.bet_this_round);

  return (
    <div className="action-panel">
      <div className="action-row">
        <button className="btn danger" disabled={!canAct} onClick={() => onAction("fold")}>
          Fold
        </button>
        {toCall <= 0 ? (
          <button className="btn neutral" disabled={!canAct} onClick={() => onAction("check")}>
            Check
          </button>
        ) : (
          <button
            className="btn primary"
            disabled={!canAct}
            onClick={() => onAction("call")}
          >
            Call {Math.min(toCall, me.chips)}
          </button>
        )}
        <button className="btn warn" disabled={!canAct} onClick={() => onAction("all_in")}>
          All-in {me.chips}
        </button>
        <div className="raise-box">
          <input
            type="range"
            min={minRaiseTarget}
            max={maxRaiseTarget}
            step={state.big_blind}
            value={raiseTarget}
            disabled={!canAct}
            onChange={(e) => setRaiseTarget(Number(e.target.value))}
          />
          <button
            className="btn primary"
            disabled={!canAct}
            onClick={() => onAction("raise", raiseTarget)}
          >
            Raise {betAmount}
          </button>
        </div>
      </div>
      {!canAct && state.phase === 4 && <div className="action-note">Mão terminada — nova mão em breve…</div>}
    </div>
  );
}
