package server

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/websocket/v2"
	"github.com/gofrs/uuid/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	appotel "gamblefriends/backend/internal/otel"
	"gamblefriends/backend/internal/db"
	"gamblefriends/backend/internal/room"
)

type Server struct {
	Manager *room.Manager
	app     *fiber.App
	tracer  trace.Tracer
}

func New(manager *room.Manager, allowedOrigins string) *Server {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(logger.New())
	corsCfg := cors.Config{
		AllowOrigins: allowedOrigins,
		AllowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders: "Content-Type, X-CSRF-Token",
	}
	// Wildcard + credentials is invalid; when an explicit origin list is
	// configured, echo credentials so the CSRF cookie survives CORS.
	if allowedOrigins != "*" {
		corsCfg.AllowCredentials = true
	}
	app.Use(cors.New(corsCfg))

	s := &Server{
		Manager: manager,
		app:     app,
		tracer:  otel.GetTracerProvider().Tracer(appotel.ServiceName),
	}
	s.routes()
	return s
}

func (s *Server) App() *fiber.App { return s.app }

func (s *Server) Listen(addr string) error {
	return s.app.Listen(addr)
}

func (s *Server) Shutdown() error {
	return s.app.Shutdown()
}

func (s *Server) routes() {
	s.app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true, "db": db.Enabled()})
	})

	api := s.app.Group("/api")
	api.Use(s.otelMiddleware(), csrfMiddleware(secureCookies(), csrfSameSite()))
	api.Get("/csrf", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"token": c.Locals(csrfLocalsKey)})
	})
	api.Post("/rooms", s.handleCreateRoom)
	api.Get("/rooms/:code", s.handleGetRoom)
	api.Get("/rooms/:code/config", s.handleGetRoomConfig)

	s.app.Get("/ws", websocket.New(s.handleWS, websocket.Config{Subprotocols: []string{"chat"}}))
}

// otelMiddleware starts a server span per HTTP request and records the result.
func (s *Server) otelMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, span := s.tracer.Start(c.UserContext(), "http "+c.Method()+" "+c.Path(),
			trace.WithSpanKind(trace.SpanKindServer))
		c.SetUserContext(ctx)
		defer span.End()
		err := c.Next()
		status := c.Response().StatusCode()
		span.SetAttributes(
			semconv.HTTPRequestMethodKey.String(c.Method()),
			semconv.HTTPRouteKey.String(c.Route().Path),
			semconv.HTTPResponseStatusCodeKey.Int(status),
			attribute.String("http.client_ip", c.IP()),
		)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
		if status >= 500 {
			span.SetStatus(codes.Error, http.StatusText(status))
		}
		return nil
	}
}

type createRoomRequest struct {
	Name         string `json:"name"`
	MaxPlayers   int    `json:"max_players"`
	InitialChips int    `json:"initial_chips"`
	SmallBlind   int    `json:"small_blind"`
	BigBlind     int    `json:"big_blind"`
	Nickname     string `json:"nickname"`
	SessionID    string `json:"session_id"`
}

func (s *Server) handleCreateRoom(c *fiber.Ctx) error {
	span := trace.SpanFromContext(c.UserContext())
	var req createRoomRequest
	if err := c.BodyParser(&req); err != nil {
		slog.Warn("create room: invalid body", "error", err)
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	req.Nickname = strings.TrimSpace(req.Nickname)
	if len(req.Nickname) < 1 || len(req.Nickname) > 20 {
		slog.Warn("create room: invalid nickname", "nickname", req.Nickname)
		return c.Status(400).JSON(fiber.Map{"error": "nickname must be 1-20 characters"})
	}
	if _, err := uuid.FromString(req.SessionID); err != nil {
		slog.Warn("create room: invalid session_id", "session_id", req.SessionID, "error", err)
		return c.Status(400).JSON(fiber.Map{"error": "invalid session_id"})
	}
	cfg := room.Config{
		Name:         strings.TrimSpace(req.Name),
		MaxPlayers:   req.MaxPlayers,
		InitialChips: req.InitialChips,
		SmallBlind:   req.SmallBlind,
		BigBlind:     req.BigBlind,
	}
	if err := cfg.Validate(); err != nil {
		slog.Warn("create room: invalid config", "config", cfg, "error", err)
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	r, err := s.Manager.CreateRoom(cfg, &room.Seat{SessionID: req.SessionID, Nickname: req.Nickname, IsHost: true})
	if err != nil {
		slog.Error("create room: failed", "error", err, "config", cfg)
		span.RecordError(err)
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	slog.Info("room created",
		"code", r.Code(), "name", r.Config().Name, "max_players", cfg.MaxPlayers,
		"initial_chips", cfg.InitialChips, "small_blind", cfg.SmallBlind, "big_blind", cfg.BigBlind)
	return c.Status(201).JSON(fiber.Map{"code": r.Code(), "name": r.Config().Name})
}

func (s *Server) handleGetRoom(c *fiber.Ctx) error {
	r, ok := s.Manager.GetRoom(strings.ToUpper(c.Params("code")))
	if !ok {
		slog.Debug("get room: not found", "code", c.Params("code"))
		return c.Status(404).JSON(fiber.Map{"found": false})
	}
	return c.JSON(fiber.Map{
		"found":        true,
		"code":         r.Code(),
		"name":         r.Config().Name,
		"status":       r.Status(),
		"player_count": r.PlayerCount(),
		"max_players":  r.Config().MaxPlayers,
	})
}

func (s *Server) handleGetRoomConfig(c *fiber.Ctx) error {
	r, ok := s.Manager.GetRoom(strings.ToUpper(c.Params("code")))
	if !ok {
		slog.Debug("get room config: not found", "code", c.Params("code"))
		return c.Status(404).JSON(fiber.Map{"error": "room not found"})
	}
	return c.JSON(r.Config())
}

type wsMessage struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	Nickname  string `json:"nickname"`
	Action    string `json:"action"`
	Amount    int    `json:"amount"`
}

func (s *Server) handleWS(conn *websocket.Conn) {
	_, span := s.tracer.Start(context.Background(), "ws connect",
		trace.WithSpanKind(trace.SpanKindServer))
	defer span.End()

	roomCode := strings.ToUpper(conn.Query("room"))
	r, ok := s.Manager.GetRoom(roomCode)
	if !ok {
		slog.Info("ws: room not found", "room", roomCode, "ip", conn.RemoteAddr().String())
		span.SetAttributes(attribute.String("ws.room", roomCode), attribute.Bool("ws.room_found", false))
		conn.WriteJSON(fiber.Map{"type": "error", "payload": "room not found"})
		conn.Close()
		return
	}
	span.SetAttributes(attribute.String("ws.room", roomCode), attribute.Bool("ws.room_found", true))

	client := &room.Client{Send: make(chan []byte, 64)}
	var closeOnce sync.Once

	// Single writer goroutine (fasthttp websocket is not safe for concurrent
	// writes). It exits when the send channel is closed or a write fails.
	go func() {
		for b := range client.Send {
			if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
				return
			}
		}
	}()
	// Called from the room goroutine (kick/leave): flush pending messages
	// briefly, then close the connection to unblock the read loop.
	client.Close = func() {
		closeOnce.Do(func() {
			time.AfterFunc(250*time.Millisecond, func() {
				conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				conn.Close()
			})
		})
	}
	defer closeOnce.Do(func() { conn.Close() })
	defer close(client.Send)

	var joinedSession string

	for {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}
		switch msg.Type {
		case "join":
			msg.Nickname = strings.TrimSpace(msg.Nickname)
			if _, err := uuid.FromString(msg.SessionID); err != nil {
				slog.Warn("ws join: invalid session_id", "room", roomCode, "session_id", msg.SessionID)
				conn.WriteJSON(fiber.Map{"type": "error", "payload": "invalid session_id"})
				continue
			}
			if len(msg.Nickname) < 1 || len(msg.Nickname) > 20 {
				slog.Warn("ws join: invalid nickname", "room", roomCode, "nickname", msg.Nickname)
				conn.WriteJSON(fiber.Map{"type": "error", "payload": "invalid nickname"})
				continue
			}
			client.SessionID = msg.SessionID
			client.Nickname = msg.Nickname
			joinedSession = msg.SessionID
			resp := make(chan error, 1)
			r.Command(room.Command{Kind: room.CmdJoin, Client: client, Resp: resp})
			if err := <-resp; err != nil {
				slog.Warn("ws join: rejected", "room", roomCode, "session_id", msg.SessionID, "nickname", msg.Nickname, "error", err)
				conn.WriteJSON(fiber.Map{"type": "error", "payload": err.Error()})
				conn.Close()
				return
			}
			slog.Info("ws join", "room", roomCode, "session_id", msg.SessionID, "nickname", msg.Nickname)

		case "action":
			if joinedSession == "" {
				continue
			}
			act := room.ActionFromString(msg.Action)
			act.Amount = msg.Amount
			resp := make(chan error, 1)
			r.Command(room.Command{Kind: room.CmdAction, SessionID: joinedSession, Action: act, Resp: resp})
			if err := <-resp; err != nil {
				slog.Warn("ws action: rejected", "room", roomCode, "session_id", joinedSession, "action", msg.Action, "error", err)
				conn.WriteJSON(fiber.Map{"type": "error", "payload": err.Error()})
			}

		case "start":
			if joinedSession == "" {
				continue
			}
			resp := make(chan error, 1)
			r.Command(room.Command{Kind: room.CmdStart, SessionID: joinedSession, Resp: resp})
			if err := <-resp; err != nil {
				slog.Warn("ws start: rejected", "room", roomCode, "session_id", joinedSession, "error", err)
				conn.WriteJSON(fiber.Map{"type": "error", "payload": err.Error()})
			}

		case "kick":
			if joinedSession == "" {
				continue
			}
			resp := make(chan error, 1)
			r.Command(room.Command{Kind: room.CmdKick, SessionID: joinedSession, Target: msg.SessionID, Resp: resp})
			if err := <-resp; err != nil {
				slog.Warn("ws kick: rejected", "room", roomCode, "session_id", joinedSession, "target", msg.SessionID, "error", err)
				conn.WriteJSON(fiber.Map{"type": "error", "payload": err.Error()})
			}
		}
	}

	if joinedSession != "" {
		slog.Info("ws leave", "room", roomCode, "session_id", joinedSession)
		resp := make(chan error, 1)
		r.Command(room.Command{Kind: room.CmdLeave, SessionID: joinedSession, Resp: resp})
		<-resp
	}
}

func secureCookies() bool {
	return strings.EqualFold(os.Getenv("GWF_COOKIE_SECURE"), "true")
}

func csrfSameSite() string {
	switch strings.ToLower(os.Getenv("GWF_COOKIE_SAMESITE")) {
	case "none":
		return "None"
	case "strict":
		return "Strict"
	}
	return "Lax"
}
