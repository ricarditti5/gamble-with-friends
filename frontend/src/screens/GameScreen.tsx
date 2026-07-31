import { useEffect, useReducer, useRef } from "react";
import { ActionPanel } from "../components/ActionPanel";
import { HistoryLog } from "../components/HistoryLog";
import { Table2D } from "../components/Table2D";
import { GameClient } from "../lib/ws";
import type { ConnStatus } from "../lib/ws";
import type { Session } from "../lib/session";
import type {
  GameStateMsg,
  LogEntry,
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
  log: LogEntry[];
  conn: ConnStatus;
  kicked: boolean;
  lastError: string | null;
}

const initial: GameState = {
  state: null,
  showdown: null,
  matchOver: null,
  log: [],
  conn: "connecting",
  kicked: false,
  lastError: null,
};

type Action =
  | { type: "state"; msg: GameStateMsg }
  | { type: "showdown"; payload: ShowdownMsg["payload"] }
  | { type: "match"; payload: MatchOverMsg["payload"] }
  | { type: "log"; entry: LogEntry }
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
        log: a.msg.log.length ? a.msg.log : g.log,
        showdown: a.msg.state.hand_over ? g.showdown : null,
        matchOver: a.msg.room.status === "finished" ? g.matchOver : null,
      };
    case "showdown":
      return { ...g, showdown: a.payload };
    case "match":
      return { ...g, matchOver: a.payload, showdown: null };
    case "log":
      return { ...g, log: [...g.log, a.entry].slice(-60) };
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
      case "log":
        dispatch({ type: "log", entry: msg.payload });
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

  const isHost = game.state?.room.host_id === session.session_id;
  const state = game.state?.state;
  const yourIdx = game.state?.your_idx ?? -1;
  const waiting = game.state?.room.status === "waiting";
  const canStart = waiting && isHost && (game.state?.seats.length ?? 0) >= 2;
  const connected = game.conn === "open";

  const copyCode = () => {
    navigator.clipboard?.writeText(roomCode).catch(() => {});
  };

  if (game.kicked) {
    return (
      <div className="entry">
        <div className="entry-card">
          <h2>Foste expulso da sala</h2>
          <button className="btn primary big" onClick={onLeave}>Voltar ao início</button>
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
          {waiting && <span className="waiting-label">A aguardar jogadores… ({game.state?.seats.length ?? 0}/{game.state?.room.max_players ?? 9})</span>}
        </div>
        <div className="conn">
          {connected ? (
            <span className="conn-ok">● Ligado</span>
          ) : (
            <span className="conn-bad">● {game.conn === "reconnecting" ? "A reconectar…" : "A ligar…"}</span>
          )}
          <button className="btn subtle" onClick={onLeave}>Sair</button>
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
                {game.state?.seats.map((s) => (
                  <div key={s.session_id} className="lobby-seat">
                    <span className="avatar">{s.nickname.slice(0, 1).toUpperCase()}</span>
                    {s.nickname} {s.is_host ? "(host)" : ""}
                    {isHost && !s.is_host && (
                      <button className="btn tiny danger" onClick={() => send("kick", { session_id: s.session_id })}>
                        Expulsar
                      </button>
                    )}
                  </div>
                ))}
              </div>
              {isHost ? (
                <button className="btn primary big" disabled={!canStart} onClick={() => send("start")}>
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
          <h3>Histórico</h3>
          <HistoryLog log={game.log} />
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

      {game.matchOver && (
        <div className="modal">
          <div className="modal-card">
            <h2>🏆 {game.matchOver.winner_name} ganhou a partida!</h2>
            <p>
              Fichas finais: {game.matchOver.final_chips.toLocaleString("pt-PT")} ·
              {game.matchOver.player_count} jogadores
            </p>
            {isHost && (
              <button className="btn primary big" onClick={() => send("start")}>
                Jogar outra partida (fichas repostas)
              </button>
            )}
            <button className="btn subtle" onClick={onLeave}>Sair da sala</button>
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
