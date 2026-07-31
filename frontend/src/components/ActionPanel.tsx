import { useEffect, useMemo, useState } from "react";
import type { PublicState } from "../types";

// The server sends the remaining seconds with each state; we tick locally so
// the timer bar is smooth and buttons disable at 0 (server auto-acts).
export function ActionPanel({ state, yourIdx, onAction, disabled }: {
  state: PublicState;
  yourIdx: number;
  onAction: (type: "fold" | "check" | "call" | "raise" | "all_in", amount?: number) => void;
  disabled: boolean;
}) {
  const me = state.players[yourIdx];
  const toCall = me ? state.current_bet - me.bet_this_round : 0;
  const [remaining, setRemaining] = useState(30);
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

  useEffect(() => {
    const iv = setInterval(() => setRemaining((r) => Math.max(0, r - 1)), 1000);
    return () => clearInterval(iv);
  }, [state.current_idx, state.phase]);

  if (!me) return null;
  const timeUp = remaining <= 0;

  const betAmount = Math.max(0, raiseTarget - me.bet_this_round);

  return (
    <div className="action-panel">
      <div className="timer-bar">
        <div className="timer-fill" style={{ width: `${Math.min(100, (remaining / 30) * 100)}%` }} />
      </div>
      <div className="action-row">
        <button className="btn danger" disabled={!canAct || timeUp} onClick={() => onAction("fold")}>
          Fold
        </button>
        {toCall <= 0 ? (
          <button className="btn neutral" disabled={!canAct || timeUp} onClick={() => onAction("check")}>
            Check
          </button>
        ) : (
          <button
            className="btn primary"
            disabled={!canAct || timeUp}
            onClick={() => onAction("call")}
          >
            Call {Math.min(toCall, me.chips)}
          </button>
        )}
        <button className="btn warn" disabled={!canAct || timeUp} onClick={() => onAction("all_in")}>
          All-in {me.chips}
        </button>
        <div className="raise-box">
          <input
            type="range"
            min={minRaiseTarget}
            max={maxRaiseTarget}
            step={state.big_blind}
            value={raiseTarget}
            disabled={!canAct || timeUp}
            onChange={(e) => setRaiseTarget(Number(e.target.value))}
          />
          <button
            className="btn primary"
            disabled={!canAct || timeUp}
            onClick={() => onAction("raise", raiseTarget)}
          >
            Raise {betAmount}
          </button>
        </div>
      </div>
      {!canAct && state.phase === 4 && <div className="action-note">Mão terminada — nova mão em breve…</div>}
      {canAct && timeUp && <div className="action-note">Tempo esgotado — o servidor vai agir por ti…</div>}
    </div>
  );
}
