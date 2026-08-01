import type { PublicState, SeatInfo } from "../types";

// Sidebar list of the players in the room: who is seated, their connection
// state, chips and position in the current hand.
export function PlayersList({ seats, state }: { seats: SeatInfo[]; state: PublicState | null }) {
  const bySession = new Map(state?.players.map((p) => [p.session_id, p]) ?? []);
  const dealerSession = state ? state.players[state.dealer_idx]?.session_id : null;
  const currentSession = state ? state.players[state.current_idx]?.session_id : null;

  return (
    <div className="players">
      {seats.map((s) => {
        const p = bySession.get(s.session_id);
        const cls = ["player-row"];
        if (p && p.session_id === currentSession && !state?.hand_over) cls.push("turn");
        if (p?.status === 1) cls.push("folded");
        if (p?.status === 3) cls.push("spectator");
        if (s.disconnected) cls.push("offline");
        return (
          <div key={s.session_id} className={cls.join(" ")}>
            <span className="avatar">
              {s.nickname.slice(0, 1).toUpperCase()}
              {s.disconnected && <span className="offline-dot" />}
            </span>
            <div className="player-info">
              <div className="player-name">
                {s.nickname}
                {s.is_host && <span className="badge host">host</span>}
                {s.is_bot && <span className="badge ai">bot</span>}
              </div>
              <div className="player-sub">
                {s.disconnected
                  ? "sem ligação"
                  : p
                    ? `${p.chips.toLocaleString("pt-PT")} fichas`
                    : "à espera…"}
              </div>
            </div>
            {p && p.session_id === dealerSession && <span className="badge dealer">D</span>}
          </div>
        );
      })}
    </div>
  );
}
