# Gamble with Friends — Poker Texas Hold'em online

Aplicação web estilo **Kahoot** para jogar Texas Hold'em com amigos: cria-se uma
sala privada com um código de 6 caracteres, cada amigo entra só com um nickname
(sem contas), e joga-se em tempo real com fichas virtuais repostas a cada partida.

- **Backend:** Go + Fiber + WebSocket (goroutine por sala, estado em memória)
- **Frontend:** React + TypeScript + Vite (mesa 2D; 3D com react-three-fiber como passo futuro)
- **BD opcional:** PostgreSQL para histórico de partidas (`migrations/*.sql` up/down)

## Estrutura

```
backend/
  main.go                 # entrada: env, DB opcional, manager, servidor
  migrations/             # ficheiros .sql (up/down) — aplicas tu
  internal/
    game/                 # motor de jogo PURO (sem rede) — testado
      card.go deck.go     # cartas + shuffle criptográfico (crypto/rand)
      eval.go             # avaliação de mãos (best-of-5 de 7 cartas)
      engine.go           # rondas, blinds, side pots, showdown, timers
    room/                 # salas: goroutine por sala, códigos, reconexão
    server/               # HTTP (Fiber) + WebSocket
    db/                   # camada opcional de histórico PostgreSQL
frontend/
  src/screens/            # EntryScreen (Kahoot-style) + GameScreen
  src/components/         # mesa 2D, cartas, painel de ações, histórico
  src/lib/                # session (UUID local), api, ws (reconexão)
scripts/
  smoke-test.mjs          # teste end-to-end via WebSocket (3 bots)
```

## Correr o backend

```bash
cd backend
go mod tidy
go run .          # ou: go build -o bin/server . && ./bin/server
```

Variáveis de ambiente (opcionais):

| Variável | Descrição | Default |
|---|---|---|
| `PORT` | porta HTTP/WS | `8080` |
| `CORS_ORIGINS` | origens permitidas (`*` ou lista separada por vírgulas; com lista explícita as credenciais são permitidas) | `*` |
| `DATABASE_URL` | PostgreSQL (histórico de partidas) | desligado |
| `AUTO_MIGRATE` | aplica as migrations no arranque | `false` |
| `GWF_LOG_FORMAT` | formato dos logs estruturados: `text` ou `json` | `text` |
| `GWF_CSRF_SECRET` | segredo (hex) para assinar tokens CSRF; se vazio, é gerado no arranque (tokens reiniciam) | gerado |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | endpoint OTLP/HTTP (ex: `http://collector:4318`) — liga tracing OpenTelemetry | desligado |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | endpoint OTLP específico de traces (substitui o anterior) | — |
| `OTEL_TRACES_EXPORTER` | define `none` para desligar explicitamente o tracing | — |

## Observabilidade

- Logs estruturados com `log/slog` (JSON com `GWF_LOG_FORMAT=json`) — erros de
  criação de sala, joins/actions/start/kick no WebSocket, CSRF e arranque.
- Tracing OpenTelemetry (OTLP/HTTP) por request HTTP e ligação WebSocket:
  basta definir `OTEL_EXPORTER_OTLP_ENDPOINT` (o serviço aparece como
  `gamblefriends-backend`).

## Segurança

- **CSRF:** token assinado (HMAC-SHA256) emitido por `GET /api/csrf` e enviado
  no header `X-CSRF-Token` em métodos que alteram estado. Sem cookies — funciona
  entre sites (Vercel → API) mesmo com third-party cookies bloqueados. Define
  `GWF_CSRF_SECRET` (hex) para tokens estáveis entre reinícios.
- **CORS:** com `CORS_ORIGINS` explícito (ex: `https://frontend.meusite.com`) o
  backend permite credenciais e o preflight aceita `X-CSRF-Token`.

## Migrations (PostgreSQL)

Os ficheiros estão em `backend/migrations/` como pares `NNNN_nome.up.sql` /
`NNNN_nome.down.sql` para aplicares manualmente (ex: com psql):

```bash
cd backend/migrations
psql "$DATABASE_URL" -f 0001_rooms.up.sql
psql "$DATABASE_URL" -f 0002_game_history.up.sql
```

Para reverter: `psql "$DATABASE_URL" -f 0001_rooms.down.sql` (por ordem inversa).

Sem PostgreSQL o jogo funciona na mesma — o histórico é apenas registado no fim
de cada partida quando `DATABASE_URL` está definido.

## Correr o frontend

```bash
cd frontend
npm install
npm run dev       # http://localhost:5173 (proxy para o backend :8080)
```

Build de produção: `npm run build` (output em `frontend/dist`).

### Ligar o frontend ao backend (produção)

O frontend só sabe onde está o backend através de variáveis de ambiente do
Vite (`VITE_*`). São injectadas **em tempo de build** — copia
`frontend/.env.example` para `frontend/.env`, define os URLs e volta a correr
`npm run build`:

```bash
cd frontend
cp .env.example .env
# edita .env: VITE_API_URL e VITE_WS_URL
npm run build
```

| Variável | Descrição | Default |
|---|---|---|
| `VITE_API_URL` | URL do backend para os pedidos HTTP (ex: `https://api.meusite.com`) | vazio (usa o mesmo host) |
| `VITE_WS_URL` | URL do backend para o WebSocket (ex: `wss://api.meusite.com`) | vazio (usa o mesmo host) |

Se o frontend e o backend estiverem em **domínios diferentes**, o backend
também precisa de:

```bash
CORS_ORIGINS=https://frontend.meusite.com   # domínio do frontend, não "*"
```

## Testes

```bash
cd backend
go test ./...              # testes do motor (mãos, side pots, rounds, sala)
go test -race ./...        # confirma ausência de race conditions

# End-to-end (backend a correr em :8080):
node scripts/smoke-test.mjs
```

O smoke test cria uma sala, liga 3 bots via WebSocket, joga uma partida completa
e valida: conservação de fichas, cartas privadas só para o dono, fases do jogo,
showdowns e fim da partida.

## Como funciona

1. **Entrada (sem contas):** o jogador escreve um nickname → o browser gera um
   `session_id` (UUID) guardado em `localStorage`. Este ID é a identidade real
   (RF2.2) — permite reconexão à mesma mesa/posição ao recarregar a página.
2. **Criar/entrar:** o host cria a sala (nome, nº de jogadores, fichas, blinds)
   e recebe um código de 6 caracteres. Os amigos introduzem o código para entrar.
3. **Jogo:** o servidor é a única fonte de verdade (RNF1.2). Os clientes só
   enviam ações (`fold/check/call/raise/all_in`); cartas privadas só chegam ao
   dono. Timer de 25s por jogada — em timeout o servidor faz `fold` (ou `check`
   se possível) (RF3.10).
4. **Partida:** jogador com 0 fichas passa a espectador; quando sobra um jogador
   com fichas a partida termina. O host pode começar outra partida — as fichas
   de todos são repostas (RF1.6).

## Segurança

- Shuffle com `crypto/rand` (RNF1.1)
- Validação de todas as ações no servidor (RNF1.4)
- `session_id` (UUID) validado na ligação WebSocket — ninguém assume a identidade
  de outro jogador só com o nome (RNF1.5)
- Cada sala corre isolada numa goroutine própria (RNF2.1/RNF2.2)

## Próximos passos sugeridos

- Mesa 3D (`@react-three/fiber`) — o backend não muda, só a camada visual
- Blinds crescentes automáticos (torneio)
- Persistir histórico de mãos em `game_history`
