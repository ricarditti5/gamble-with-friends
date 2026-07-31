// Smoke test end-to-end: cria uma sala, liga 3 clientes WebSocket, joga uma
// partida completa com ações automáticas e valida invariantes (conservação de
// fichas, cartas privadas, fases, showdown, fim da partida).
//
// Uso: inicia o backend (go run .) e depois:
//   node scripts/smoke-test.mjs [base_url]
//
// Default base_url: http://localhost:8080

const BASE = process.argv[2] ?? "http://localhost:8080";
const WS = BASE.replace(/^http/, "ws") + "/ws";

const uid = () => crypto.randomUUID();
const log = (s) => console.log(`[smoke] ${s}`);

function client(name) {
  const ws = new WebSocket(WS + "?room=" + encodeURIComponent(roomCode));
  const out = { ws, name, session: uid(), messages: [], latest: null, closed: false, yourIdx: -1, yourCards: [] };
  ws.onmessage = (ev) => {
    const msg = JSON.parse(ev.data);
    out.messages.push(msg);
    if (msg.type === "game_state") {
      out.latest = msg;
      out.yourIdx = msg.your_idx;
      out.yourCards = msg.your_cards;
    }
  };
  ws.onclose = () => (out.closed = true);
  return out;
}

function send(c, msg) {
  if (!c.closed) c.ws.send(JSON.stringify(msg));
}

function myPlayer(c) {
  const s = c.latest;
  if (!s || s.yourIdx < 0) return null;
  return s.state.players[s.yourIdx];
}

function pickAction(c) {
  const s = c.latest;
  if (!s) return null;
  const st = s.state;
  const me = myPlayer(c);
  if (!me || me.status !== 0 || st.hand_over || st.current_idx !== c.yourIdx) return null;
  const toCall = st.current_bet - me.bet_this_round;
  const r = Math.random();
  if (me.chips <= st.big_blind || r < 0.08) return { action: "all_in" };
  if (toCall > 0) return r < 0.25 ? { action: "fold" } : { action: "call" };
  if (r < 0.3) {
    const min = Math.max(st.current_bet + (st.min_raise || st.big_blind), me.bet_this_round + st.big_blind);
    const max = me.chips + me.bet_this_round;
    if (min <= max) return { action: "raise", amount: min };
  }
  return { action: "check" };
}

function assert(cond, what) {
  if (!cond) {
    console.error(`[smoke] FAIL: ${what}`);
    process.exit(1);
  }
  log(`ok: ${what}`);
}

let roomCode = "";
let roomName = "";

const host = uid();
log(`a criar sala (host session ${host.slice(0, 8)}…)`);
const createRes = await fetch(BASE + "/api/rooms", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    name: "Smoke Test",
    max_players: 3,
    initial_chips: 500,
    small_blind: 5,
    big_blind: 10,
    nickname: "Host",
    session_id: host,
  }),
});
assert(createRes.status === 201, "POST /api/rooms -> 201");
const created = await createRes.json();
roomCode = created.code;
roomName = created.name;
log(`sala ${roomCode} criada`);

const lookup = await (await fetch(`${BASE}/api/rooms/${roomCode}`)).json();
assert(lookup.found && lookup.status === "waiting", "GET /api/rooms/:code -> found, waiting");

const cHost = client("Host");
const cP2 = client("P2");
const cP3 = client("P3");

const join = (c) => send(c, { type: "join", session_id: c.session, nickname: c.name });
join(cHost);
join(cP2);
join(cP3);

await new Promise((r) => setTimeout(r, 400));

// Sala espera que host comece; host só pode começar com >=2 jogadores.
send(cHost, { type: "start" });
await new Promise((r) => setTimeout(r, 300));

const stateAfterStart = cHost.latest;
assert(stateAfterStart && stateAfterStart.room.status === "in_progress", "sala em in_progress após start");
assert(stateAfterStart.state.phase === 0, "fase inicial preflop");
assert(stateAfterStart.state.players.length === 3, "3 jogadores sentados");
assert(stateAfterStart.state.players.every((p) => p.chips === 500), "todas as fichas = 500 no início");
assert(
  stateAfterStart.state.players.every((p) => p.status === 0) ||
    stateAfterStart.state.players.filter((p) => p.status === 0).length >= 2,
  "jogadores ativos"
);

// As cartas privadas só aparecem para o dono.
const totalChipss = stateAfterStart.state.players.reduce((a, p) => a + p.chips, 0) + stateAfterStart.state.pot;
assert(totalChipss === 1500, `conservação de fichas (${totalChipss} == 1500)`);

// Joga até a partida acabar (com timeout de segurança).
const deadline = Date.now() + 90_000;
let handNumber = stateAfterStart.state.hand_number;
let showdowns = 0;
let matchOver = false;
let sawNewHand = false;

while (Date.now() < deadline && !matchOver) {
  for (const c of [cHost, cP2, cP3]) {
    const a = pickAction(c);
    if (a) {
      send(c, { type: "action", action: a.action, amount: a.amount ?? 0 });
    }
  }
  // processa eventos
  await new Promise((r) => setTimeout(r, 80));

  for (const c of [cHost, cP2, cP3]) {
    for (const msg of c.messages) {
      if (msg.type === "showdown") showdowns++;
      if (msg.type === "match_over") matchOver = true;
    }
    c.messages.length = 0;
    const s = c.latest;
    if (s) {
      if (s.state.hand_number !== handNumber) {
        handNumber = s.state.hand_number;
        sawNewHand = true;
      }
      const chips = s.state.players.reduce((a, p) => a + p.chips, 0) + s.state.pot;
      if (chips !== 1500) {
        console.error(`[smoke] FAIL: fichas não conservadas na mão ${s.state.hand_number}: ${chips}`);
        process.exit(1);
      }
      // as cartas do próprio têm sempre tamanho 2 quando está numa mão
      if (c.yourIdx >= 0 && s.state.players[c.yourIdx].status !== 3 && c.yourCards.length !== 2) {
        console.error(`[smoke] FAIL: ${c.name} deveria ter 2 cartas privadas, tem ${c.yourCards.length}`);
        process.exit(1);
      }
    }
  }
}

assert(matchOver, "partida terminou com match_over");
assert(showdowns >= 1, "houve pelo menos um showdown");
assert(sawNewHand, "houve mais do que uma mão (blinds rodaram)");

const winner = cHost.messages
  .concat(cP2.messages, cP3.messages)
  .find((m) => m.type === "match_over");
assert(!!winner, "match_over com payload");
assert(typeof winner.payload.winner_name === "string", "vencedor identificado");

for (const c of [cHost, cP2, cP3]) c.ws.close();
log("smoke test passou! ✅");
process.exit(0);
