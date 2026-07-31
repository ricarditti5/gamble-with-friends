import { useState } from "react";
import { createRoom, lookupRoom } from "../lib/api";
import { loadSession, saveSession, updateNickname } from "../lib/session";
import type { Session } from "../lib/session";

interface Props {
  onEnter: (session: Session, roomCode: string) => void;
}

export function EntryScreen({ onEnter }: Props) {
  const existing = loadSession();
  const [nickname, setNickname] = useState(existing?.nickname ?? "");
  const [mode, setMode] = useState<"join" | "create">("join");
  const [code, setCode] = useState("");
  const [roomName, setRoomName] = useState("Noite de Poker");
  const [maxPlayers, setMaxPlayers] = useState(6);
  const [initialChips, setInitialChips] = useState(1000);
  const [sb, setSb] = useState(5);
  const [bb, setBb] = useState(10);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [roomFound, setRoomFound] = useState<{ name: string; players: number; max: number } | null>(null);

  const sessionFor = (): Session => {
    if (existing && existing.nickname === nickname.trim()) return existing;
    return nickname.trim() === existing?.nickname ? existing : saveSession(nickname);
  };

  const onJoin = async () => {
    setError(null);
    const clean = code.trim().toUpperCase();
    if (clean.length < 3) return setError("Introduz o código da sala");
    setBusy(true);
    try {
      const info = await lookupRoom(clean);
      if (!info.found) {
        setError("Sala não encontrada. Verifica o código.");
        return;
      }
      if (info.status === "finished") {
        setError("Essa sala já terminou.");
        return;
      }
      if (info.player_count! >= info.max_players!) {
        setError("Sala cheia.");
        return;
      }
      setRoomFound({ name: info.name ?? "Sala", players: info.player_count ?? 0, max: info.max_players ?? 9 });
      const session = sessionFor();
      onEnter(session, clean);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Erro ao entrar na sala");
    } finally {
      setBusy(false);
    }
  };

  const onCreate = async () => {
    setError(null);
    const session = sessionFor();
    setBusy(true);
    try {
      const { code: newCode } = await createRoom({
        name: roomName.trim() || "Noite de Poker",
        max_players: maxPlayers,
        initial_chips: initialChips,
        small_blind: sb,
        big_blind: bb,
        nickname: session.nickname,
        session_id: session.session_id,
      });
      onEnter(session, newCode);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Erro ao criar a sala");
    } finally {
      setBusy(false);
    }
  };

  const onNicknameBlur = () => {
    if (nickname.trim()) {
      const s = existing && existing.nickname === nickname.trim()
        ? existing
        : updateNickname(nickname);
      void s;
    }
  };

  return (
    <div className="entry">
      <div className="entry-card">
        <div className="logo">♠ <span>Gamble with</span> Friends</div>
        <p className="subtitle">Texas Hold'em em tempo real com amigos</p>

        <label className="field">
          <span>O teu nome</span>
          <input
            value={nickname}
            maxLength={20}
            placeholder="Ex: Rui"
            onChange={(e) => setNickname(e.target.value)}
            onBlur={onNicknameBlur}
          />
        </label>

        <div className="mode-tabs">
          <button className={mode === "join" ? "active" : ""} onClick={() => setMode("join")}>
            Entrar numa sala
          </button>
          <button className={mode === "create" ? "active" : ""} onClick={() => setMode("create")}>
            Criar sala
          </button>
        </div>

        {mode === "join" ? (
          <div className="join-box">
            <input
              className="code-input"
              value={code}
              maxLength={6}
              placeholder="CÓDIGO"
              onChange={(e) => setCode(e.target.value.toUpperCase())}
              onKeyDown={(e) => e.key === "Enter" && onJoin()}
            />
            <button className="btn primary big" disabled={busy} onClick={onJoin}>
              {busy ? "A entrar…" : "Entrar"}
            </button>
            {roomFound && (
              <div className="room-info">
                {roomFound.name} · {roomFound.players}/{roomFound.max} jogadores
              </div>
            )}
          </div>
        ) : (
          <div className="create-box">
            <label className="field">
              <span>Nome da sala</span>
              <input value={roomName} maxLength={40} onChange={(e) => setRoomName(e.target.value)} />
            </label>
            <div className="grid2">
              <label className="field">
                <span>Jogadores (2–9)</span>
                <select value={maxPlayers} onChange={(e) => setMaxPlayers(Number(e.target.value))}>
                  {[2, 3, 4, 5, 6, 7, 8, 9].map((n) => (
                    <option key={n} value={n}>{n}</option>
                  ))}
                </select>
              </label>
              <label className="field">
                <span>Fichas por jogador</span>
                <select value={initialChips} onChange={(e) => setInitialChips(Number(e.target.value))}>
                  {[500, 1000, 2000, 5000, 10000].map((n) => (
                    <option key={n} value={n}>{n.toLocaleString("pt-PT")}</option>
                  ))}
                </select>
              </label>
              <label className="field">
                <span>Small blind</span>
                <select value={sb} onChange={(e) => setSb(Number(e.target.value))}>
                  {[1, 2, 5, 10, 25, 50].map((n) => (
                    <option key={n} value={n}>{n}</option>
                  ))}
                </select>
              </label>
              <label className="field">
                <span>Big blind</span>
                <select value={bb} onChange={(e) => setBb(Number(e.target.value))}>
                  {[2, 5, 10, 20, 50, 100].map((n) => (
                    <option key={n} value={n}>{n}</option>
                  ))}
                </select>
              </label>
            </div>
            <button className="btn primary big" disabled={busy} onClick={onCreate}>
              {busy ? "A criar…" : "Criar sala"}
            </button>
          </div>
        )}

        {error && <div className="error">{error}</div>}
      </div>
    </div>
  );
}
