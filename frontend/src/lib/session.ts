// Player session: nickname + UUID stored in localStorage (RF2.2, RF2.5).
// No accounts — the session id is the only identity.

const KEY = "gwf.session";

export interface Session {
  session_id: string;
  nickname: string;
}

export function loadSession(): Session | null {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return null;
    const s = JSON.parse(raw) as Session;
    if (!s.session_id || !s.nickname) return null;
    return s;
  } catch {
    return null;
  }
}

// crypto.randomUUID só existe em contextos seguros (https/localhost). Em
// http://IP-da-LAN sem https falharia — usa-se um fallback UUID v4.
function newUUID(): string {
  try {
    if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
      return crypto.randomUUID();
    }
  } catch {
    // continua para o fallback
  }
  const b = new Uint8Array(16);
  for (let i = 0; i < 16; i++) b[i] = Math.floor(Math.random() * 256);
  b[6] = (b[6] & 0x0f) | 0x40;
  b[8] = (b[8] & 0x3f) | 0x80;
  const h = Array.from(b, (x) => x.toString(16).padStart(2, "0"));
  return `${h[0]}${h[1]}${h[2]}${h[3]}-${h[4]}${h[5]}-${h[6]}${h[7]}-${h[8]}${h[9]}-${h[10]}${h[11]}${h[12]}${h[13]}${h[14]}${h[15]}`;
}

export function saveSession(nickname: string): Session {
  const s: Session = {
    session_id: newUUID(),
    nickname: nickname.trim().slice(0, 20),
  };
  localStorage.setItem(KEY, JSON.stringify(s));
  return s;
}

export function updateNickname(nickname: string): Session {
  const cur = loadSession() ?? { session_id: newUUID() } as Session;
  const s: Session = { session_id: cur.session_id, nickname: nickname.trim().slice(0, 20) };
  localStorage.setItem(KEY, JSON.stringify(s));
  return s;
}
