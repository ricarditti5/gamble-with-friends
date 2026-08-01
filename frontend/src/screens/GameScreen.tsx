import { useEffect, useReducer, useRef, useState } from "react";
import { ActionPanel } from "../components/ActionPanel";
import { PlayersList } from "../components/PlayersList";
import { Table2D } from "../components/Table2D";
import { GameClient } from "../lib/ws";
import type { ConnStatus } from "../lib/ws";
import { clearRoom } from "../lib/session";
import type { Session } from "../lib/session";
import type {
  GameStateMsg,
  MatchOverMsg,
  ServerMsg,
  ShowdownMsg,
} from "../types";
import { CardView } from "../components/CardView";
import { HAND_NAMES } from "../types";

interface GameState {
  state: GameStateMsg | null;
  showdown: ShowdownMsg["payload"] | null;
  matchOver: MatchOverMsg["payload"] | null;
  conn: ConnStatus;
  kicked: boolean;
  lastError: string | null;
}

const initial: GameState = {
  state: null,
  showdown: null,
  matchOver: null,
  conn: "connecting",
  kicked: false,
  lastError: null,
};

type Action =
  | { type: "state"; msg: GameStateMsg }
  | { type: "showdown"; payload: ShowdownMsg["payload"] }
  | { type: "match"; payload: MatchOverMsg["payload"] }
  | { type: "conn"; status: ConnStatus }
  | { type: "kicked" }
  | { type: "error"; message: string }
  | { type: "reset" };

function reducer(g: GameState, a: Action): GameState {
  switch (a.type) {
    case "state":
      return {
        ...g,
        state: a.msg,
        showdown: a.msg.state.hand_over ? g.showdown : null,
        matchOver: a.msg.room.status === "finished" ? g.matchOver : null,
      };
    case "showdown":
      return { ...g, showdown: a.payload };
    case "match":
      return { ...g, matchOver: a.payload, showdown: null };
    case "conn":
      return { ...g, conn: a.status };
    case "kicked":
      return { ...g, kicked: true };
    case "error":
      return { ...g, lastError: a.message };
    case "reset":
      return initial;
  }
}

export function GameScreen({ session, roomCode, onLeave }: {
  session: Session;
  roomCode: string;
  onLeave: () => void;
}) {
  const [game, dispatch] = useReducer(reducer, initial);
  const clientRef = useRef<GameClient | null>(null);
  const [bbSession, setBbSession] = useState<string | null>(null);

  useEffect(() => {
    const client = new GameClient(session.session_id, session.nickname, roomCode);
    clientRef.current = client;
    client.onMessage = (msg: ServerMsg) => handleMessage(msg);
    client.onStatus = (status) => dispatch({ type: "conn", status });
    client.connect();
    return () => {
      client.close();
      clientRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [roomCode]);

  // Heads-up: default the blind pick to the player who isn't the host.
  useEffect(() => {
    const seats = game.state?.seats ?? [];
    if (
      seats.length === 2 &&
      (!bbSession || !seats.some((s) => s.session_id === bbSession))
    ) {
      const other = seats.find((s) => s.session_id !== session.session_id);
      setBbSession(other?.session_id ?? seats[0].session_id);
    }
  }, [game.state?.seats, bbSession, session.session_id]);

  const handleMessage = (msg: ServerMsg) => {
    switch (msg.type) {
      case "game_state":
        dispatch({ type: "state", msg });
        break;
      case "showdown":
        dispatch({ type: "showdown", payload: msg.payload });
        break;
      case "match_over":
        dispatch({ type: "match", payload: msg.payload });
        break;
      case "kicked":
        dispatch({ type: "kicked" });
        clientRef.current?.close();
        break;
      case "error":
        if (msg.payload.toLowerCase().includes("kicked")) {
          dispatch({ type: "kicked" });
          clientRef.current?.close();
          break;
        }
        dispatch({ type: "error", message: msg.payload });
        break;
    }
  };

  const send = (type: string, extra?: Record<string, unknown>) => {
    clientRef.current?.send({ type, ...extra });
  };

  // Explicit leave: tells the server to free the seat and clears the saved
  // room so a refresh won't rejoin it.
  const leave = () => {
    clientRef.current?.send({ type: "leave" });
    clientRef.current?.close();
    clearRoom();
    onLeave();
  };

  const isHost = game.state?.room.host_id === session.session_id;
  const state = game.state?.state;
  const yourIdx = game.state?.your_idx ?? -1;
  const waiting = game.state?.room.status === "waiting";
  const seats = game.state?.seats ?? [];
  const canStart = waiting && isHost && seats.length >= 2;
  const connected = game.conn === "open";
  // After a refresh on a finished room the match payload comes from the
  // game_state champion instead of the match_over event.
  const matchOver = game.matchOver ?? game.state?.champion ?? null;

  const copyCode = () => {
    navigator.clipboard?.writeText(roomCode).catch(() => {});
  };

  const startGame = () => {
    if (seats.length === 2) {
      send("start", { big_blind_session: bbSession ?? seats[1].session_id });
    } else {
      send("start");
    }
  };

  if (game.kicked) {
    return (
      <div className="entry">
        <div className="entry-card">
          <h2>Foste expulso da sala</h2>
          <button className="btn primary big" onClick={leave}>Voltar ao início</button>
        </div>
      </div>
    );
  }

  return (
    <div className="game">
      <header className="topbar">
        <div className="brand">♠ Gamble with Friends</div>
        <div className="room-title">
          <b>{game.state?.room.name}</b>
          <button className="code-chip" onClick={copyCode} title="Copiar código">
            {roomCode} ⧉
          </button>
          <span className={`status-dot ${game.state?.room.status ?? "waiting"}`} />
          {waiting && <span className="waiting-label">A aguardar jogadores… ({seats.length}/{game.state?.room.max_players ?? 9})</span>}
        </div>
        <div className="conn">
          {connected ? (
            <span className="conn-ok">● Ligado</span>
          ) : (
            <span className="conn-bad">● {game.conn === "reconnecting" ? "A reconectar…" : "A ligar…"}</span>
          )}
          <button className="btn subtle" onClick={leave}>Sair</button>
        </div>
      </header>

      <main className="main">
        <div className="table-col">
          {state && <Table2D state={state} yourIdx={yourIdx} yourCards={game.state?.your_cards ?? []} />}

          {waiting && (
            <div className="lobby-card">
              <p>
                Estás na sala <b>{roomCode}</b>. Partilha o código com os teus amigos!
              </p>
              <div className="lobby-seats">
                {seats.map((s) => (
                  <div key={s.session_id} className={`lobby-seat ${s.disconnected ? "offline" : ""}`}>
                    <span className="avatar">{s.nickname.slice(0, 1).toUpperCase()}</span>
                    {s.nickname} {s.is_host ? "(host)" : ""} {s.is_bot ? "(bot)" : ""}
                    {s.disconnected ? " (sem ligação)" : ""}
                    {isHost && !s.is_host && !s.is_bot && (
                      <button className="btn tiny danger" onClick={() => send("kick", { session_id: s.session_id })}>
                        Expulsar
                      </button>
                    )}
                    {isHost && s.is_bot && (
                      <button className="btn tiny danger" onClick={() => send("remove_bot", { session_id: s.session_id })}>
                        Remover
                      </button>
                    )}
                  </div>
                ))}
              </div>

              {isHost && seats.length < (game.state?.room.max_players ?? 9) && (
                <button className="btn subtle" onClick={() => send("add_bot")}>
                  + Adicionar bot
                </button>
              )}

              {isHost && seats.length === 2 && (
                <label className="field blind-pick">
                  <span>Quem fica de Big Blind? (o outro fica de Small Blind)</span>
                  <select value={bbSession ?? ""} onChange={(e) => setBbSession(e.target.value)}>
                    {seats.map((s) => (
                      <option key={s.session_id} value={s.session_id}>{s.nickname}</option>
                    ))}
                  </select>
                </label>
              )}

              {isHost ? (
                <button className="btn primary big" disabled={!canStart} onClick={startGame}>
                  {canStart ? "Começar partida" : "Espera por mais jogadores (mín. 2)"}
                </button>
              ) : (
                <p className="muted">O host vai iniciar a partida…</p>
              )}
            </div>
          )}

          {state && !waiting && (
            <ActionPanel
              state={state}
              yourIdx={yourIdx}
              disabled={!connected}
              onAction={(type, amount) =>
                type === "raise"
                  ? send("action", { action: type, amount })
                  : send("action", { action: type })
              }
            />
          )}
        </div>

        <aside className="sidebar">
          <h3>Jogadores</h3>
          <PlayersList seats={seats} state={state ?? null} />
        </aside>
      </main>

      {game.state && !waiting && (
        <div className="hand-info">
          Mão #{game.state.state.hand_number} ·{" "}
          {state?.pot.toLocaleString("pt-PT")} no pot
        </div>
      )}

      {game.showdown && (
        <div className="modal">
          <div className="modal-card">
            <h2>{game.showdown.showdown ? "Showdown!" : "Todos fizeram fold"}</h2>
            {game.showdown.winners.map((w) => (
              <div key={w.player_idx} className="winner-row">
                <div className="winner-cards">
                  {w.cards.map((c, i) => (
                    <CardView key={i} card={c} small />
                  ))}
                </div>
                <div>
                  <b>{w.nickname}</b> ganha {w.amount.toLocaleString("pt-PT")}
                  {w.hand.category !== undefined && (
                    <div className="hand-name">{HAND_NAMES[w.hand.category as keyof typeof HAND_NAMES]}</div>
                  )}
                </div>
              </div>
            ))}
            <p className="muted">Nova mão em breve…</p>
          </div>
        </div>
      )}

      {matchOver && (
        <div className="modal">
          <div className="modal-card">
            <h2>🏆 {matchOver.winner_name} ganhou a partida!</h2>
            <p>
              Fichas finais: {matchOver.final_chips.toLocaleString("pt-PT")} ·
              {matchOver.player_count} jogadores
            </p>
            {isHost && (
              <button className="btn primary big" onClick={startGame}>
                Jogar outra partida (fichas repostas)
              </button>
            )}
            <button className="btn subtle" onClick={leave}>Sair da sala</button>
          </div>
        </div>
      )}

      {game.lastError && (
        <div className="modal">
          <div className="modal-card">
            <h2>Erro</h2>
            <p>{game.lastError}</p>
            <button className="btn primary" onClick={() => dispatch({ type: "error", message: "" })}>OK</button>
          </div>
        </div>
      )}
    </div>
  );
}
