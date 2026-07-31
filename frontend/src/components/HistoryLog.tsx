import { useEffect, useRef } from "react";
import type { LogEntry } from "../types";

export function HistoryLog({ log }: { log: LogEntry[] }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    ref.current?.scrollTo({ top: ref.current.scrollHeight, behavior: "smooth" });
  }, [log.length]);

  return (
    <div className="history" ref={ref}>
      {log.map((e, i) => (
        <div key={i} className={`log-entry ${e.kind}`}>
          {e.text}
        </div>
      ))}
    </div>
  );
}
