package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// Error codes of the authentication endpoints.
const (
	CodeInvalidCredentials = "INVALID_CREDENTIALS" //nolint:gosec // G101: API error code, not a credential
	CodeEmailNotVerified   = "EMAIL_NOT_VERIFIED"
	CodeUnauthenticated    = "UNAUTHENTICATED"
	CodeCSRFRejected       = "CSRF_REJECTED"
)

// SessionCookie carries the web session token. The __Host- prefix forces Secure, Path=/ and no
// Domain attribute, so the cookie cannot be set or read by sibling subdomains.
const SessionCookie = "__Host-session"

type loginRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	ClientType string `json:"clientType"`
}

type loginResponse struct {
	TokenType     string    `json:"tokenType,omitempty"`
	Token         string    `json:"token,omitempty"`
	ExpiresAt     time.Time `json:"expiresAt"`
	IdleExpiresAt time.Time `json:"idleExpiresAt"`
}

type meResponse struct {
	ID              uuid.UUID  `json:"id"`
	Email           string     `json:"email"`
	FullName        string     `json:"fullName"`
	Phone           *string    `json:"phone"`
	Status          string     `json:"status"`
	IsPlatformAdmin bool       `json:"isPlatformAdmin"`
	EmailVerifiedAt *time.Time `json:"emailVerifiedAt"`
	CreatedAt       time.Time  `json:"createdAt"`
}

type logoutAllResponse struct {
	Revoked int64 `json:"revoked"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	res, err := h.svc.Login(r.Context(), application.LoginInput{
		Email: req.Email, Password: req.Password, ClientType: req.ClientType,
		IP: clientIP(r), UserAgent: r.UserAgent(),
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	body := loginResponse{ExpiresAt: res.AbsoluteExpiresAt, IdleExpiresAt: res.IdleExpiresAt}
	if res.ClientType == "WEB" {
		// Web: the token travels only in the HttpOnly cookie, never in the body.
		http.SetCookie(w, &http.Cookie{
			Name: SessionCookie, Value: res.Token.Reveal(), Path: "/",
			Expires: res.AbsoluteExpiresAt, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
		})
	} else {
		body.TokenType, body.Token = "Bearer", res.Token.Reveal()
	}
	httpx.WriteJSON(w, http.StatusOK, body)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	p, _ := contract.PrincipalFrom(r.Context())
	if err := h.svc.Logout(r.Context(), p); err != nil {
		h.fail(w, r, err)
		return
	}
	clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) logoutAll(w http.ResponseWriter, r *http.Request) {
	p, _ := contract.PrincipalFrom(r.Context())
	n, err := h.svc.LogoutAll(r.Context(), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	clearSessionCookie(w)
	httpx.WriteJSON(w, http.StatusOK, logoutAllResponse{Revoked: n})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	p, _ := contract.PrincipalFrom(r.Context())
	u, err := h.svc.Me(r.Context(), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toMeResponse(u))
}

func toMeResponse(u *domain.User) meResponse {
	out := meResponse{
		ID: u.ID(), Email: u.Email().String(), FullName: u.FullName(), Status: string(u.Status()),
		IsPlatformAdmin: u.IsPlatformAdmin(), EmailVerifiedAt: u.EmailVerifiedAt(), CreatedAt: u.CreatedAt(),
	}
	if phone := u.Phone(); phone != "" {
		out.Phone = &phone
	}
	return out
}

type setPhoneRequest struct {
	Phone string `json:"phone"`
}

// setPhone serves PUT /api/v1/me/phone (T6.3): the mobile number customers need to book.
func (h *Handler) setPhone(w http.ResponseWriter, r *http.Request) {
	var req setPhoneRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	u, err := h.svc.SetPhone(r.Context(), p, req.Phone)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toMeResponse(u))
}

// RequireAuth authenticates the request (Bearer token for mobile apps, session cookie for web
// portals) and stores the Principal in the context. Cookie-authenticated requests that change
// state must come from an allowed Origin (CSRF defense in depth on top of SameSite=Lax).
func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, viaCookie := sessionToken(r)
		if viaCookie && !safeMethod(r.Method) && !h.originAllowed(r.Header.Get("Origin")) {
			h.logger.WarnContext(r.Context(), "csrf check failed",
				slog.String("request_id", httpx.RequestIDFrom(r.Context())),
				slog.String("origin", r.Header.Get("Origin")))
			httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusForbidden, CodeCSRFRejected, "cross-site request rejected"))
			return
		}
		p, err := h.svc.Authenticate(r.Context(), token)
		if err != nil {
			if !errors.Is(err, application.ErrUnauthenticated) {
				h.fail(w, r, err)
				return
			}
			w.Header().Set("WWW-Authenticate", `Bearer realm="booking-platform"`)
			httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusUnauthorized, CodeUnauthenticated, "authentication required"))
			return
		}
		next.ServeHTTP(w, r.WithContext(contract.WithPrincipal(r.Context(), p)))
	})
}

// sessionToken prefers the Authorization header; falls back to the session cookie.
func sessionToken(r *http.Request) (token string, viaCookie bool) {
	if auth := r.Header.Get("Authorization"); auth != "" {
		scheme, value, ok := strings.Cut(auth, " ")
		if ok && strings.EqualFold(scheme, "Bearer") {
			return strings.TrimSpace(value), false
		}
		return "", false // present but not Bearer: treated as invalid, no cookie fallback
	}
	if c, err := r.Cookie(SessionCookie); err == nil {
		return c.Value, true
	}
	return "", false
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func (h *Handler) originAllowed(origin string) bool {
	if origin == "" {
		return false
	}
	for _, o := range h.allowedOrigins {
		if o == origin {
			return true
		}
	}
	return false
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
}

// clientIP returns the TCP peer address (see httpx.ClientIP).
func clientIP(r *http.Request) *netip.Addr {
	ip, ok := httpx.ClientIP(r)
	if !ok {
		return nil
	}
	return &ip
}
