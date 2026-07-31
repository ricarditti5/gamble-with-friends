import { useState } from "react";
import { EntryScreen } from "./screens/EntryScreen";
import { GameScreen } from "./screens/GameScreen";
import type { Session } from "./lib/session";

type View = { screen: "entry" } | { screen: "game"; session: Session; code: string };

export default function App() {
  const [view, setView] = useState<View>({ screen: "entry" });

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
  return <EntryScreen onEnter={(session, code) => setView({ screen: "game", session, code })} />;
}
