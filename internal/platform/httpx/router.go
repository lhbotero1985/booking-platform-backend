package httpx

import "net/http"

// Router is a thin wrapper over http.ServeMux whose 404 and 405 responses are RFC 9457
// problems instead of plain text.
type Router struct {
	mux *http.ServeMux
}

// NewRouter returns an empty router.
func NewRouter() *Router {
	return &Router{mux: http.NewServeMux()}
}

// Handle registers h for a ServeMux pattern such as "GET /healthz".
func (rt *Router) Handle(pattern string, h http.Handler) {
	rt.mux.Handle(pattern, h)
}

// HandleFunc registers f for a ServeMux pattern.
func (rt *Router) HandleFunc(pattern string, f http.HandlerFunc) {
	rt.mux.HandleFunc(pattern, f)
}

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h, pattern := rt.mux.Handler(r)
	if pattern != "" {
		rt.mux.ServeHTTP(w, r)
		return
	}

	// No route matched: run ServeMux's own fallback against a probe to learn whether it is a
	// 404, a 405 (with its Allow header) or a redirect, then answer accordingly.
	probe := &probeWriter{header: http.Header{}}
	h.ServeHTTP(probe, r)

	switch probe.code {
	case http.StatusMethodNotAllowed:
		w.Header().Set("Allow", probe.header.Get("Allow"))
		WriteProblem(w, r, NewProblem(http.StatusMethodNotAllowed, CodeMethodNotAllowed, ""))
	case http.StatusMovedPermanently, http.StatusPermanentRedirect, http.StatusTemporaryRedirect, http.StatusFound:
		rt.mux.ServeHTTP(w, r)
	default:
		WriteProblem(w, r, NewProblem(http.StatusNotFound, CodeNotFound, ""))
	}
}

type probeWriter struct {
	header http.Header
	code   int
}

func (p *probeWriter) Header() http.Header { return p.header }

func (p *probeWriter) Write(b []byte) (int, error) {
	if p.code == 0 {
		p.code = http.StatusOK
	}
	return len(b), nil
}

func (p *probeWriter) WriteHeader(code int) {
	if p.code == 0 {
		p.code = code
	}
}
