package app

import (
	"io"
	"net/http"
)

// appPendingPage is shown while the app is not in the stores yet. Plain HTML: the security
// headers forbid styles and scripts.
const appPendingPage = `<!doctype html>
<html lang="es">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>App de reservas</title></head>
<body>
<h1>La app de reservas estará disponible muy pronto</h1>
<p>Tu cita ya quedó registrada. Mientras tanto, si necesitas cambiarla o cancelarla, comunícate con el negocio.</p>
<p>Cuando la app esté publicada, este mismo enlace te llevará a descargarla.</p>
</body>
</html>
`

// appDownload serves GET /app, the link of the WhatsApp button "Descargar la app" (P40, T7.3):
// a redirect to the store when APP_DOWNLOAD_URL is set, a short notice otherwise. The template in
// Meta keeps the same link either way.
func appDownload(storeURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if storeURL != "" {
			w.Header().Set("Cache-Control", "no-store")
			http.Redirect(w, r, storeURL, http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, appPendingPage)
	}
}
