package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"os"

	"github.com/gofiber/fiber/v2"
)

// csrfTokens emite e verifica tokens CSRF assinados (HMAC-SHA256). Sem cookies:
// o frontend guarda o token devolvido por GET /api/csrf em memória e devolve-o
// no header X-CSRF-Token. Funciona entre sites (Vercel -> API) sem depender de
// SameSite/Secure ou do bloqueio de third-party cookies.
//
// Segredo: GWF_CSRF_SECRET (hex). Se vazio, é gerado no arranque — os tokens
// deixam de ser válidos após um restart.
type csrfTokens struct {
	secret []byte
}

func newCsrfTokens() (*csrfTokens, error) {
	var secret []byte
	if raw := os.Getenv("GWF_CSRF_SECRET"); raw != "" {
		secret, _ = hex.DecodeString(raw)
		if len(secret) == 0 {
			secret = []byte(raw)
		}
	}
	if len(secret) == 0 {
		secret = make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return nil, err
		}
	}
	return &csrfTokens{secret: secret}, nil
}

func (t *csrfTokens) issue() (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(append(nonce, t.sign(nonce)...)), nil
}

func (t *csrfTokens) sign(nonce []byte) []byte {
	mac := hmac.New(sha256.New, t.secret)
	mac.Write(nonce)
	return mac.Sum(nil)
}

func (t *csrfTokens) verify(token string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 16+sha256.Size {
		return false
	}
	return hmac.Equal(raw[16:], t.sign(raw[:16]))
}

// csrfMiddleware exige um token CSRF válido em métodos que alteram estado
// (POST/PUT/PATCH/DELETE). Um site malicioso não consegue ler o token (fica em
// memória no frontend legítimo) nem enviar headers personalizados sem passar
// pelo preflight CORS.
func csrfMiddleware(tokens *csrfTokens) fiber.Handler {
	return func(c *fiber.Ctx) error {
		switch c.Method() {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			return c.Next()
		}
		tok := c.Get("X-CSRF-Token")
		if tok == "" || !tokens.verify(tok) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "invalid csrf token"})
		}
		return c.Next()
	}
}
