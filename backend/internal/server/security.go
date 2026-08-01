package server

import (
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

// clientIP devolve o IP real atrás de proxies (Render/Vercel/nginx), lendo o
// primeiro valor de X-Forwarded-For; sem proxy usa o IP do socket.
func clientIP(c *fiber.Ctx) string {
	if xff := c.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i >= 0 {
			xff = xff[:i]
		}
		if xff = strings.TrimSpace(xff); xff != "" {
			return xff
		}
	}
	return c.IP()
}

// rateLimiter limita pedidos por IP (em memória). Serve para travar floods,
// bots e abuso da API — o CORS não protege contra pedidos diretos.
func rateLimiter(max int, window time.Duration) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:          max,
		Expiration:   window,
		KeyGenerator: clientIP,
		LimitReached: func(c *fiber.Ctx) error {
			slog.Warn("rate limit", "ip", clientIP(c), "path", c.Path())
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": "too many requests"})
		},
	})
}

// wsOriginGuard rejeita ligações WebSocket de origens não autorizadas (CSWSH).
// Permitido: clientes sem Origin (bots/testes), same-origin (Origin == Host),
// origens da lista de CORS, ou qualquer origem quando o acesso é aberto.
func wsOriginGuard(allowed []string, open bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if open {
			return c.Next()
		}
		origin := c.Get("Origin")
		if origin == "" {
			return c.Next()
		}
		originHost := origin
		if i := strings.Index(origin, "://"); i >= 0 {
			originHost = origin[i+3:]
		}
		if originHost == c.Hostname() {
			return c.Next()
		}
		for _, o := range allowed {
			if strings.EqualFold(o, origin) {
				return c.Next()
			}
		}
		slog.Warn("ws: origin not allowed", "origin", origin)
		return c.SendStatus(fiber.StatusForbidden)
	}
}

// securityHeaders aplica headers de proteção básica a todas as respostas:
// CSP é servido pelo frontend (meta tag no build); aqui ficam os headers de
// transporte e de navegação que o browser aplica mesmo em APIs.
func securityHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("Referrer-Policy", "no-referrer")
		c.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=(), usb=()")
		// HSTS: browsers ignoram em http, mas quando o site é servido por
		// HTTPS (Render/Vercel terminam o TLS) passam a recusar http.
		c.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		return c.Next()
	}
}

// forceHTTPS redireciona pedidos http -> https quando o tráfego passa por um
// proxy de TLS (Render, Vercel, nginx...), detetado por X-Forwarded-Proto.
// O WebSocket fica de fora: browsers não seguem redirects em ligações WS.
func forceHTTPS() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Get("X-Forwarded-Proto") == "http" && !strings.HasPrefix(c.Path(), "/ws") {
			return c.Redirect("https://"+c.Hostname()+c.OriginalURL(), fiber.StatusMovedPermanently)
		}
		return c.Next()
	}
}

// parseOrigins converte "a.com, b.com" numa lista (ignora "*" e vazios).
// Lista vazia = qualquer origem permitida (usada no CORS e no WebSocket).
func parseOrigins(s string) []string {
	var out []string
	for _, o := range strings.Split(s, ",") {
		o = strings.TrimSpace(o)
		if o != "" && o != "*" {
			out = append(out, o)
		}
	}
	return out
}

// sanitizeText remove caracteres de controlo (que poderiam injetar linhas nos
// logs ou quebrar a saída). O XSS é já impedido pelo React (escapes) e pela
// validação de tamanho; isto é a última linha de defesa.
func sanitizeText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\t' {
			return -1
		}
		return r
	}, s)
}
