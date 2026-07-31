import type { RoomLookup } from "../types";

// Tolerante a barra final em VITE_API_URL (ex: "https://x.com/").
const API = (import.meta.env.VITE_API_URL ?? "").replace(/\/+$/, "");

// Double-submit CSRF: the backend sets a `gwf_csrf` cookie on every response;
// unsafe methods must echo the token in X-CSRF-Token.
const CSRF_COOKIE = "gwf_csrf";
let csrfToken: string | null = null;

function readCookie(name: string): string | null {
  const m = document.cookie.match(new RegExp("(?:^|; )" + name + "=([^;]*)"));
  return m ? decodeURIComponent(m[1]) : null;
}

async function ensureCsrf(): Promise<string> {
  if (csrfToken) return csrfToken;
  const res = await fetch(`${API}/api/csrf`, { credentials: "include" });
  const body = await res.json().catch(() => ({}));
  csrfToken = (body as { token?: string }).token ?? readCookie(CSRF_COOKIE);
  return csrfToken ?? "";
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const method = init?.method ?? "GET";
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (method !== "GET" && method !== "HEAD") {
    headers["X-CSRF-Token"] = await ensureCsrf();
  }
  const res = await fetch(`${API}${path}`, {
    headers,
    credentials: "include",
    ...init,
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = (body as { error?: string }).error ?? `HTTP ${res.status}`;
    // Sem VITE_API_URL definido no build, os pedidos vão para o host do
    // frontend (que não tem /api) e falham com 404.
    if (!API && (res.status === 404 || res.status === 405)) {
      throw new Error(`${err} — backend não configurado. Define VITE_API_URL no build (frontend/.env) e volta a fazer deploy.`);
    }
    throw new Error(err);
  }
  return body as T;
}

export interface CreateRoomInput {
  name: string;
  max_players: number;
  initial_chips: number;
  small_blind: number;
  big_blind: number;
  nickname: string;
  session_id: string;
}

export function createRoom(input: CreateRoomInput): Promise<{ code: string }> {
  return req("/api/rooms", { method: "POST", body: JSON.stringify(input) });
}

export function lookupRoom(code: string): Promise<RoomLookup> {
  return req(`/api/rooms/${encodeURIComponent(code)}`);
}
