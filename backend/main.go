package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"gamblefriends/backend/internal/db"
	appotel "gamblefriends/backend/internal/otel"
	"gamblefriends/backend/internal/room"
	"gamblefriends/backend/internal/server"
	"gamblefriends/backend/migrations"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdownTracing, err := appotel.Setup(ctx)
	if err != nil {
		slog.Error("otel setup failed", "error", err)
	} else {
		defer shutdownTracing(context.Background())
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	// CORS: vazio = apenas same-origin (navegador só acede à API a partir do
	// próprio host). Em produção com frontend noutro domínio, define
	// CORS_ORIGINS=https://dominio-do-frontend. "*" abre a qualquer origem.
	origins := os.Getenv("CORS_ORIGINS")

	// Optional persistence: set DATABASE_URL to enable history (RNF3.1).
	// Migrations are applied manually from migrations/*.sql (up/down pairs),
	// or automatically with AUTO_MIGRATE=true.
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		pool, err := db.Init(dsn, migrations.FS, os.Getenv("AUTO_MIGRATE") == "true")
		if err != nil {
			slog.Warn("db: disabled", "error", err)
		} else {
			defer pool.Close()
			slog.Info("db: connected")
		}
	} else {
		slog.Info("db: disabled (set DATABASE_URL for persistent history)")
	}

	manager := room.NewManager()
	manager.OnRoomFinish = func(r *room.Room) {
		if !db.Enabled() {
			return
		}
		cfg := r.Config()
		db.SaveRoom(db.RoomRecord{
			Code:         r.Code(),
			Name:         cfg.Name,
			HostSession:  r.HostSession(),
			MaxPlayers:   cfg.MaxPlayers,
			InitialChips: cfg.InitialChips,
			SmallBlind:   cfg.SmallBlind,
			BigBlind:     cfg.BigBlind,
		})
		mi := r.Champion()
		if mi != nil {
			db.SaveMatch(db.MatchRecord{
				RoomCode:    r.Code(),
				WinnerName:  mi.WinnerName,
				PotAmount:   mi.TotalPot,
				PlayerCount: mi.PlayerCount,
			})
		}
	}

	go manager.SweepLoop(ctx)

	srv := server.New(manager, origins)
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		cancel()
		if err := srv.Shutdown(); err != nil {
			slog.Error("server shutdown", "error", err)
		}
	}()

	slog.Info("gamble-with-friends backend listening", "port", port)
	if err := srv.Listen(":" + port); err != nil {
		slog.Error("server fatal", "error", err)
		os.Exit(1)
	}
}
