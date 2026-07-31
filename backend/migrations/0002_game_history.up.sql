-- Record of finished matches (written only when the match ends).

CREATE TABLE IF NOT EXISTS game_history (
    id             BIGSERIAL PRIMARY KEY,
    room_id        BIGINT REFERENCES rooms(id) ON DELETE SET NULL,
    room_code      TEXT NOT NULL,
    winner_nickname TEXT NOT NULL,
    pot_amount     INT NOT NULL,
    player_count   INT NOT NULL,
    played_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
