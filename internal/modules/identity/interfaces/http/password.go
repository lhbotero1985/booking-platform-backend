package httpapi

import (
	"net/http"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

type resetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"newPassword"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (h *Handler) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotPasswordRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.ForgotPassword(r.Context(), req.Email); err != nil {
		h.fail(w, r, err)
		return
	}
	// Identical response whether or not the account exists (anti-enumeration).
	httpx.WriteJSON(w, http.StatusAccepted, statusResponse{Status: "accepted"})
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if err := h.svc.ResetPassword(r.Context(), application.ResetPasswordInput{Token: req.Token, NewPassword: req.NewPassword}); err != nil {
		h.fail(w, r, err)
		return
	}
	// Every session was revoked, including any web session of this browser.
	clearSessionCookie(w)
	httpx.WriteJSON(w, http.StatusOK, statusResponse{Status: "password_reset"})
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	err := h.svc.ChangePassword(r.Context(), p, application.ChangePasswordInput{CurrentPassword: req.CurrentPassword, NewPassword: req.NewPassword})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
