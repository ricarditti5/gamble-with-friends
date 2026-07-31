import type { Card } from "../types";
import { CardView } from "./CardView";

export function CommunityCards({ community }: { community: Card[] }) {
  const slots = 5;
  const cards: (Card | null)[] = [];
  for (let i = 0; i < slots; i++) {
    cards.push(community[i] ?? null);
  }
  return (
    <div className="community">
      {cards.map((c, i) =>
        c ? <CardView key={i} card={c} /> : <div key={i} className="card card-slot" />
      )}
    </div>
  );
}
