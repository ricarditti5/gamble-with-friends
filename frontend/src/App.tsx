import { useEffect, useState } from "react";
import { EntryScreen } from "./screens/EntryScreen";
import { GameScreen } from "./screens/GameScreen";
import { lookupRoom } from "./lib/api";
import {
  clearRoom,
  loadRoom,
  loadSession,
  markLeft,
  saveRoom,
  wasClosedLongAgo,
} from "./lib/session";
import type { Session } from "./lib/session";

type View = { screen: "entry" } | { screen: "game"; session: Session; code: string };

export default function App() {
  const [view, setView] = useState<View>({ screen: "entry" });
  const [ready, setReady] = useState(false);

  // Refresh keeps the player in their room: the room code is persisted and a
  // quick reload resumes it automatically. Only an explicit Sair or a long
  // closed tab clears it.
  useEffect(() => {
    const session = loadSession();
    const code = loadRoom();
    if (!session || !code || wasClosedLongAgo()) {
      setReady(true);
      return;
    }
    lookupRoom(code)
      .then((info) => {
        if (info.found) {
          setView({ screen: "game", session, code });
        } else {
          clearRoom();
        }
      })
      .catch(() => clearRoom())
      .finally(() => setReady(true));
  }, []);

  useEffect(() => {
    const onHide = () => markLeft();
    window.addEventListener("pagehide", onHide);
    return () => window.removeEventListener("pagehide", onHide);
  }, []);

  if (!ready) return null;

  if (view.screen === "game") {
    return (
      <GameScreen
        key={view.code}
        session={view.session}
        roomCode={view.code}
        onLeave={() => setView({ screen: "entry" })}
      />
    );
  }
  return (
    <EntryScreen
      onEnter={(session, code) => {
        saveRoom(code);
        setView({ screen: "game", session, code });
      }}
    />
  );
}
