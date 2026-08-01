import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";

// Injeta a meta tag CSP apenas no build de produção (no dev quebraria o HMR).
// "Qualquer browser pode jogar": conecta ao backend da própria app (https/wss)
// e a mais nada; scripts só locais; sem iframes nem objetos embebidos.
function securityHeadersPlugin(): Plugin {
  const csp = [
    "default-src 'self'",
    "script-src 'self'",
    "style-src 'self' 'unsafe-inline'",
    "img-src 'self' data:",
    "font-src 'self' data:",
    "connect-src 'self' https: wss: ws:",
    "base-uri 'self'",
    "form-action 'self'",
    "frame-ancestors 'none'",
    "object-src 'none'",
  ].join("; ");
  return {
    name: "gwf-security-headers",
    apply: "build",
    transformIndexHtml(html) {
      return html.replace(
        "<head>",
        `<head>\n    <meta http-equiv="Content-Security-Policy" content="${csp}">`,
      );
    },
  };
}

export default defineConfig({
  plugins: [react(), securityHeadersPlugin()],
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: process.env.VITE_API_PROXY ?? "http://localhost:8080",
        changeOrigin: true,
      },
      "/ws": {
        target: process.env.VITE_API_PROXY ?? "ws://localhost:8080",
        ws: true,
        changeOrigin: true,
      },
    },
  },
});
