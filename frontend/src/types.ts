// Types mirroring the Go backend payloads (internal/game, internal/room).

export type Phase = 0 | 1 | 2 | 3 | 4; // preflop flop turn river hand_over
export type PlayerStatus = 0 | 1 | 2 | 3; // active folded all_in spectator
export type HandCategory = 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8; // high..straight flush

export const PHASE_NAMES = ["Pré-flop", "Flop", "Turn", "River", "Fim da mão"] as const;

export const HAND_NAMES: Record<HandCategory, string> = {
  0: "Carta Alta",
  1: "Par",
  2: "Dois Pares",
  3: "Trinca",
  4: "Sequência",
  5: "Flush",
  6: "Full House",
  7: "Quadra",
  8: "Sequência de Copas", // straight flush (inclui royal)
};

export interface Card {
  rank: number; // 2..14
  suit: number; // 0..3 (S H D C)
}

export interface PublicPlayer {
  session_id: string;
  nickname: string;
  chips: number;
  status: PlayerStatus;
  bet_this_round: number;
  has_acted: boolean;
  disconnected: boolean;
}

export interface PublicState {
  hand_number: number;
  phase: Phase;
  community: Card[];
  pot: number;
  current_bet: number;
  min_raise: number;
  dealer_idx: number;
  current_idx: number;
  players: PublicPlayer[];
  small_blind: number;
  big_blind: number;
  hand_over: boolean;
}

export interface SeatInfo {
  session_id: string;
  nickname: string;
  is_host: boolean;
  is_bot?: boolean;
  disconnected?: boolean;
}

export interface LogEntry {
  text: string;
  kind: string;
}

export interface RoomInfo {
  code: string;
  name: string;
  status: "waiting" | "in_progress" | "finished";
  host_id: string;
  max_players: number;
}

export interface GameStateMsg {
  type: "game_state";
  room: RoomInfo;
  seats: SeatInfo[];
  log: LogEntry[];
  remaining: number;
  state: PublicState;
  your_cards: Card[];
  your_idx: number;
  /** Result of the finished match; present when room.status === "finished". */
  champion?: MatchOverMsg["payload"];
}

export interface ShowdownWinner {
  player_idx: number;
  nickname: string;
  amount: number;
  hand: { category: HandCategory; tie: number[] };
  cards: Card[];
}

export interface ShowdownMsg {
  type: "showdown";
  payload: {
    showdown: boolean;
    pots: { amount: number; eligible: number[] }[];
    winners: ShowdownWinner[];
    community: Card[];
  };
}

export interface MatchOverMsg {
  type: "match_over";
  payload: {
    winner_session: string;
    winner_name: string;
    final_chips: number;
    total_pot: number;
    player_count: number;
  };
}

export interface LogMsg {
  type: "log";
  payload: LogEntry;
}

export interface PlayerJoinedMsg {
  type: "player_joined" | "player_left" | "host_changed";
  payload: { session_id: string; nickname?: string };
}

export interface ErrorMsg {
  type: "error";
  payload: string;
}

export interface KickedMsg {
  type: "kicked";
}

export type ServerMsg =
  | GameStateMsg
  | ShowdownMsg
  | MatchOverMsg
  | LogMsg
  | PlayerJoinedMsg
  | ErrorMsg
  | KickedMsg;

export interface RoomLookup {
  found: boolean;
  code?: string;
  name?: string;
  status?: string;
  player_count?: number;
  max_players?: number;
}
