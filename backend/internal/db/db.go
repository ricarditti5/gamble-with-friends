// Package db is an optional PostgreSQL persistence layer. If DATABASE_URL is
// unset the server runs fully in-memory (RNF3.1) and history is skipped.
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var conn *sql.DB

// Init opens the database and returns the handle. Migrations are NOT applied
// automatically; apply the files in migrations/*.sql yourself (or set
// AUTO_MIGRATE=true to let the app apply them on startup).
func Init(dsn string, fs embed.FS, autoMigrate bool) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if autoMigrate {
		if err := runMigrations(db, fs, "up"); err != nil {
			db.Close()
			return nil, err
		}
	}
	conn = db
	return db, nil
}

// runMigrations applies every up.sql (or down.sql) file in ascending order,
// tracking applied versions in schema_migrations.
func runMigrations(db *sql.DB, fs embed.FS, direction string) error {
	entries, err := fs.ReadDir(".")
	if err != nil {
		return err
	}
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "."+direction+".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}

	for _, f := range files {
		version := strings.TrimSuffix(f, "."+direction+".sql")
		var exists bool
		if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		body, err := fs.ReadFile(f)
		if err != nil {
			return err
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", f, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version) VALUES($1)`, version); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		log.Printf("db: applied migration %s", f)
	}
	return nil
}

// MigrateDown is exposed for manual rollbacks (used by ops, not the app).
func MigrateDown(dsn string, fs embed.FS) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	// Apply down files in reverse order.
	entries, _ := fs.ReadDir(".")
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".down.sql") {
			files = append(files, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	for _, f := range files {
		version := strings.TrimSuffix(f, ".down.sql")
		body, err := fs.ReadFile(f)
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(body)); err != nil {
			return fmt.Errorf("down migration %s: %w", f, err)
		}
		if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version=$1`, version); err != nil {
			return err
		}
		log.Printf("db: rolled back %s", f)
	}
	return nil
}

type RoomRecord struct {
	Code         string
	Name         string
	HostSession  string
	MaxPlayers   int
	InitialChips int
	SmallBlind   int
	BigBlind     int
}

type MatchRecord struct {
	RoomCode    string
	WinnerName  string
	PotAmount   int
	PlayerCount int
}

func Enabled() bool { return conn != nil }

func SaveRoom(r RoomRecord) {
	if conn == nil {
		return
	}
	_, err := conn.Exec(`INSERT INTO rooms
		(code, name, host_session_id, max_players, initial_chips, small_blind, big_blind, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'finished'::text)`,
		r.Code, r.Name, r.HostSession, r.MaxPlayers, r.InitialChips, r.SmallBlind, r.BigBlind)
	if err != nil {
		log.Printf("db: save room %s: %v", r.Code, err)
	}
}

func SaveMatch(m MatchRecord) {
	if conn == nil {
		return
	}
	_, err := conn.Exec(`INSERT INTO game_history (room_code, winner_nickname, pot_amount, player_count, played_at)
		VALUES ($1,$2,$3,$4,$5)`,
		m.RoomCode, m.WinnerName, m.PotAmount, m.PlayerCount, time.Now())
	if err != nil {
		log.Printf("db: save match %s: %v", m.RoomCode, err)
	}
}
