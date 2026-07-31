-- Rooms created by players (optional persistent history).

CREATE TABLE IF NOT EXISTS rooms (
    id              BIGSERIAL PRIMARY KEY,
    code            TEXT UNIQUE NOT NULL,
    name            TEXT NOT NULL,
    host_session_id TEXT NOT NULL,
    max_players     INT  NOT NULL,
    initial_chips   INT  NOT NULL,
    small_blind     INT  NOT NULL,
    big_blind       INT  NOT NULL,
    status          TEXT NOT NULL DEFAULT 'waiting',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
