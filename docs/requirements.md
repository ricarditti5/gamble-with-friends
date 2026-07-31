# Requisitos — Poker Online (Texas Hold'em) com Amigos

## 1. Visão geral

Aplicação web estilo **Kahoot**: um utilizador cria uma sala privada de poker, partilha um código com amigos, e cada jogador entra apenas com um **nome** (sem conta/password) para jogar Texas Hold'em em tempo real com fichas virtuais que são repostas a cada nova partida. Frontend com mesa em **3D**.
Site d epocker como referência: https://pokerpatio.com/lobbies

**Fora de âmbito (v1):** dinheiro real, múltiplas variantes de poker, torneios com blinds crescentes automáticos, mobile app nativa, contas persistentes com password.

---

## 2. Requisitos Funcionais (RF)

### RF1 — Gestão de salas
- RF1.1: Utilizador autenticado pode criar uma sala, definindo: nome da sala, número máximo de jogadores (2–9), valor inicial de fichas por jogador, valores de small/big blind
- RF1.2: A criação de sala gera um código único (ex: 6 caracteres alfanuméricos) partilhável
- RF1.3: Outro utilizador entra na sala introduzindo o código
- RF1.4: Sala tem estados: `waiting` (à espera de jogadores) → `in_progress` → `finished`
- RF1.5: O criador da sala (host) pode expulsar jogadores e iniciar a partida quando houver ≥2 jogadores
- RF1.6: Ao iniciar nova partida dentro da mesma sala, as fichas de todos os jogadores são repostas ao valor inicial definido em RF1.1

### RF2 — Entrada na sala (estilo Kahoot, sem conta)
- RF2.1: Jogador acede à aplicação e escreve apenas um **nickname** — sem email, sem password
- RF2.2: No momento em que escreve o nome, o sistema gera-lhe um **ID único de sessão** (UUID), guardado no browser (cookie ou localStorage)
- RF2.3: Este ID único garante que dois jogadores com o mesmo nickname (ex: dois "Rui") não são confundidos internamente — o ID é a chave real, o nome é só apresentação
- RF2.4: Depois de ter nome + ID, o jogador insere o **código da sala** para entrar (tal como no Kahoot)
- RF2.5: Se o jogador recarregar a página ou perder ligação, o ID guardado localmente permite **reconectar à mesma sala/posição** sem precisar de novo nome (ver RF4.3)
- RF2.6: Não há persistência de jogador entre sessões diferentes — é efémero, existe só enquanto a sala/partida decorre

### RF3 — Motor de jogo (regras clássicas de Poker — Texas Hold'em)
- RF3.1: Baralho de 52 cartas, shuffle aleatório seguro a cada mão
- RF3.2: Distribuição de 2 cartas privadas (hole cards) por jogador
- RF3.3: Rondas de apostas: Pre-flop → Flop (3 cartas comunitárias) → Turn (+1) → River (+1)
- RF3.4: Ações disponíveis por jogador na sua vez: `fold`, `check`, `call`, `raise`, `all-in`
- RF3.5: Sistema de blinds (small/big blind) que roda entre jogadores a cada mão (dealer button)
- RF3.6: Gestão de pot (principal) e side pots (quando há all-in com stacks diferentes)
- RF3.7: No showdown, avaliação automática da melhor mão de 5 cartas (par, dois pares, trinca, straight, flush, full house, poker, straight flush, royal flush)
- RF3.8: Distribuição do pot ao(s) vencedor(es); empates dividem o pot
- RF3.9: Jogador com 0 fichas fica em `spectator` / eliminado da partida atual
- RF3.10: Timer por jogada (ex: 20-30s) — se expirar, ação automática é `fold` (ou `check` se possível)

### RF4 — Tempo real / sincronização
- RF4.1: Todos os jogadores da sala veem o estado da mesa atualizado em tempo real (cartas comunitárias, pot, ação atual, fichas de cada jogador)
- RF4.2: Cada jogador só vê as suas próprias hole cards — nunca as dos outros antes do showdown
- RF4.3: Reconexão: se um jogador perder ligação, consegue voltar à mesma sala/partida sem perder o estado (fichas, posição)

### RF5 — Interface
- RF5.1: Mesa renderizada em **3D** (Three.js / react-three-fiber) com posições dos jogadores à volta, cartas comunitárias, pot central, fichas de cada um
- RF5.2: Indicação visual de: de quem é a vez, quem é dealer/small blind/big blind, jogadores em fold
- RF5.3: Painel de ações (fold/check/call/raise) com slider ou input para valor de raise — este painel pode continuar a ser HTML/UI normal sobreposto à cena 3D (não precisa de ser 3D também)
- RF5.4: Histórico simples de ações da mão atual (ex: "Rui apostou 50")
- RF5.5: Animações mínimas mas importantes para legibilidade: distribuir cartas, fichas a mover-se para o pot, revelar cartas no showdown
- RF5.6: Écran de entrada (nickname + código da sala) é simples, **2D/HTML normal** — só a mesa de jogo em si é 3D

---

## 3. Requisitos Não Funcionais (RNF)

### RNF1 — Segurança
- RNF1.1: Shuffle do baralho usa gerador criptograficamente seguro (`crypto/rand`, nunca `math/rand`)
- RNF1.2: Servidor é fonte única de verdade do estado do jogo — cliente nunca envia "tenho X cartas", só ações (fold/call/raise)
- RNF1.3: Hole cards de um jogador só são enviadas para o próprio (payload WebSocket diferenciado por destinatário)
- RNF1.4: Validação server-side de todas as ações (ex: não deixar apostar mais fichas do que o jogador tem)
- RNF1.5: WebSocket valida o `session_id` (UUID) do jogador na ligação — mesmo sem login tradicional, é preciso garantir que ninguém finge ser outro jogador só por saber o nome

### RNF2 — Performance / Concorrência
- RNF2.1: Cada sala corre isolada (goroutine + canal próprio em Go) para evitar que uma sala afete outra
- RNF2.2: Suportar múltiplas salas simultâneas sem partilha de estado indevida (race conditions)

### RNF3 — Disponibilidade
- RNF3.1: Estado da sala mantido em memória (ou Redis se quiseres persistência entre restarts do servidor)
- RNF3.2: Se o servidor cair, jogadores devem conseguir perceber que a ligação caiu (não ficar "pendurados")

### RNF4 — Manutenibilidade
- RNF4.1: Motor de jogo (regras, avaliação de mãos, apostas) desacoplado da camada de rede — deve ser testável com testes unitários sem precisar de WebSocket a correr

---

## 4. Stack sugerida (alinhada com o que já usas)

| Camada | Tecnologia |
|---|---|
| Backend | Go + Fiber |
| Tempo real | WebSocket (`gofiber/websocket` ou `gorilla/websocket`) |
| Base de dados | PostgreSQL (utilizadores, histórico de salas/partidas) |
| Estado em memória da partida ativa | Struct Go em memória por sala (ou Redis se quiseres escalar/persistir) |
| Frontend | React + TypeScript + `@react-three/fiber` (Three.js) para a mesa 3D |
| Identificação de jogador | UUID gerado no cliente/servidor, guardado em localStorage — sem sistema de contas |
| Deploy | Render (backend) + Vercel (frontend), à semelhança dos teus outros projetos |

---

## 5. Modelo de dados (rascunho)

**Tabelas persistentes (PostgreSQL) — opcional, só se quiseres histórico entre sessões:**
- `rooms` (id, code, host_session_id, max_players, initial_chips, small_blind, big_blind, status, created_at)
- `game_history` (id, room_id, winner_nickname, pot_amount, played_at) — opcional

**Sessão de jogador (efémera, não precisa de tabela `users` tradicional):**
- Gerada no momento em que o jogador escreve o nickname: `{ session_id: uuid, nickname: string }`
- Guardada em memória associada à sala (Room.players), não numa tabela de utilizadores permanente
- Guardada no browser (localStorage) para permitir reconexão (RF2.5)

**Estado em memória (não precisa de ir à BD a cada ação, só no fim):**
- `Room` struct: players[], deck, community_cards[], pot, current_turn, dealer_position, phase (preflop/flop/turn/river)
- `Player` struct: session_id (UUID), nickname, chips, hole_cards[], status (active/folded/all-in/spectator), current_bet

Utiliza go migrations para as migrações da base de dados(as tabelas devem estar em ficheiros up e down para easy management)
---

## 6. Fluxo de mensagens WebSocket (rascunho)

**Cliente → Servidor:**
- `join_room`, `start_game`, `player_action` (fold/check/call/raise + valor)

**Servidor → Cliente:**
- `game_state_update` (estado público: pot, cartas comunitárias, fichas de todos, de quem é a vez)
- `your_cards` (só para o próprio jogador: hole cards)
- `showdown_result` (vencedor, mãos reveladas, pot distribuído)

---

## 7. Ordem de implementação sugerida

1. **Motor de jogo puro** (sem rede): baralho, shuffle, distribuição, avaliação de mãos, rondas de apostas simples — testado com testes unitários
2. **Ecrã de entrada estilo Kahoot**: nickname + geração de session_id + entrada por código de sala
3. **WebSocket com 1 sala fixa, 2 jogadores** — validar que o fluxo de mensagens funciona
4. **Múltiplas salas com código e limite configurável**
5. **Blinds rotativos + side pots** (a parte mais complexa das regras)
6. **Frontend 2D simples** (cartas/fichas como imagens ou divs) ligado ao estado real — garante que o backend está sólido antes de complicar visualmente
7. **Reconexão e edge cases** (jogador desliga a meio da mão, timer de ação, etc.)
8. **Migração para frontend 3D** (Three.js / react-three-fiber) — só depois de tudo o resto estar a funcionar; o WebSocket/backend não muda nada, é só a camada visual que troca

---

## 8. Perguntas em aberto para decidires antes de começar

- Só Texas Hold'em, ou queres deixar preparado para outras variantes no futuro?
- Fichas resetam **por partida** (cada "jogo completo até alguém ganhar tudo") ou **por mão**? (pelo que descreveste, parece ser por partida — vale confirmar)
- Queres histórico persistente de partidas passadas, ou é só "sessão volátil" entre amigos?
- Chat de texto/voz dentro da sala é importante para ti, ou fica de fora do MVP?