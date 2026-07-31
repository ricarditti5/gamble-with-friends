package server

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"

	"github.com/gofiber/fiber/v2"
)

const csrfCookieName = "gwf_csrf"
const csrfLocalsKey = "csrf_token"

func newCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// csrfMiddleware implements a double-submit CSRF token: the server sets a
// cookie on every response and unsafe methods (POST/PUT/PATCH/DELETE) must
// echo the same token back in the X-CSRF-Token header. A cross-site attacker
// cannot read the victim's cookie or guess the 256-bit token.
//
// secure: mark the cookie Secure (HTTPS). samesite: "Lax" (default) or "None"
// (needed when the frontend and API are served from different sites).
func csrfMiddleware(secure bool, samesite string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := c.Cookies(csrfCookieName)
		if token == "" {
			t, err := newCSRFToken()
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
			}
			token = t
			c.Cookie(&fiber.Cookie{
				Name:     csrfCookieName,
				Value:    token,
				Path:     "/",
				SameSite: samesite,
				Secure:   secure,
			})
		}
		c.Locals(csrfLocalsKey, token)

		switch c.Method() {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			return c.Next()
		}
		if h := c.Get("X-CSRF-Token"); h != token {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "invalid csrf token"})
		}
		return c.Next()
	}
}
