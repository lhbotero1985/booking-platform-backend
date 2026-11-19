package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// CodeInvalidToken is returned for unknown, malformed, expired or used tokens alike.
const CodeInvalidToken = "INVALID_TOKEN"

// Service is the subset of the application used by these handlers.
type Service interface {
	Register(ctx context.Context, in application.RegisterInput) error
	VerifyEmail(ctx context.Context, rawToken string) error
	ResendVerification(ctx context.Context, email string) error
	Login(ctx context.Context, in application.LoginInput) (application.LoginResult, error)
	Authenticate(ctx context.Context, rawToken string) (contract.Principal, error)
	Logout(ctx context.Context, p contract.Principal) error
	LogoutAll(ctx context.Context, p contract.Principal) (int64, error)
	Me(ctx context.Context, p contract.Principal) (*domain.User, error)
	ForgotPassword(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, in application.ResetPasswordInput) error
	ChangePassword(ctx context.Context, p contract.Principal, in application.ChangePasswordInput) error
}

// Options configures the handler.
type Options struct {
	// AllowedOrigins are the web origins allowed to make cookie-authenticated state-changing
	// requests (same list as CORS_ALLOWED_ORIGINS).
	AllowedOrigins []string
}

// Handler serves the identity endpoints.
type Handler struct {
	svc            Service
	logger         *slog.Logger
	allowedOrigins []string
}

// New builds the handler.
func New(svc Service, logger *slog.Logger, opts Options) *Handler {
	return &Handler{svc: svc, logger: logger, allowedOrigins: opts.AllowedOrigins}
}

// Routes registers the endpoints.
func (h *Handler) Routes(rt *httpx.Router) {
	// Public.
	rt.HandleFunc("POST /api/v1/auth/register", h.register)
	rt.HandleFunc("POST /api/v1/auth/verify-email", h.verifyEmail)
	rt.HandleFunc("POST /api/v1/auth/verify-email/resend", h.resendVerification)
	rt.HandleFunc("POST /api/v1/auth/login", h.login)
	rt.HandleFunc("POST /api/v1/auth/password/forgot", h.forgotPassword)
	rt.HandleFunc("POST /api/v1/auth/password/reset", h.resetPassword)
	// Authenticated.
	rt.Handle("POST /api/v1/auth/logout", h.RequireAuth(http.HandlerFunc(h.logout)))
	rt.Handle("POST /api/v1/auth/logout-all", h.RequireAuth(http.HandlerFunc(h.logoutAll)))
	rt.Handle("GET /api/v1/me", h.RequireAuth(http.HandlerFunc(h.me)))
	rt.Handle("POST /api/v1/me/password", h.RequireAuth(http.HandlerFunc(h.changePassword)))
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"fullName"`
}

type verifyEmailRequest struct {
	Token string `json:"token"`
}

type resendRequest struct {
	Email string `json:"email"`
}

type statusResponse struct {
	Status string `json:"status"`
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	err := h.svc.Register(r.Context(), application.RegisterInput{Email: req.Email, Password: req.Password, FullName: req.FullName})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	// Same response whether or not the email was already registered (anti-enumeration).
	httpx.WriteJSON(w, http.StatusAccepted, statusResponse{Status: "accepted"})
}

func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var req verifyEmailRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.VerifyEmail(r.Context(), req.Token); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, statusResponse{Status: "verified"})
}

func (h *Handler) resendVerification(w http.ResponseWriter, r *http.Request) {
	var req resendRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.ResendVerification(r.Context(), req.Email); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, statusResponse{Status: "accepted"})
}

// fail maps application errors to problems. Unexpected errors are logged and answered with a
// generic 500 (no internal details).
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var verr *application.ValidationError
	switch {
	case errors.As(err, &verr):
		p := httpx.NewProblem(http.StatusBadRequest, httpx.CodeInvalidRequest, "one or more fields are invalid")
		for _, f := range verr.Fields {
			p.Errors = append(p.Errors, httpx.FieldError{Field: f.Field, Message: f.Code})
		}
		httpx.WriteProblem(w, r, p)
	case errors.Is(err, domain.ErrInvalidToken):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusBadRequest, CodeInvalidToken, "the link is invalid or has expired"))
	case errors.Is(err, application.ErrInvalidCredentials):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusUnauthorized, CodeInvalidCredentials, "invalid email or password"))
	case errors.Is(err, application.ErrEmailNotVerified):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusForbidden, CodeEmailNotVerified, "verify your email before signing in"))
	default:
		h.logger.ErrorContext(r.Context(), "identity request failed",
			slog.String("request_id", httpx.RequestIDFrom(r.Context())),
			slog.String("error", err.Error()))
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusInternalServerError, httpx.CodeInternal, ""))
	}
}
