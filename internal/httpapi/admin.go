package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/neko233-com/pay233-server/internal/payment"
)

//go:embed adminstatic/*
var adminFiles embed.FS

const adminCookieName = "pay233_admin"

func (s *Server) adminIndex(w http.ResponseWriter, r *http.Request) {
	if s.requestIsAdmin(r) {
		http.Redirect(w, r, "/admin/dashboard.html", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/admin/login.html", http.StatusFound)
}

func (s *Server) adminLoginPage(w http.ResponseWriter, r *http.Request) {
	if s.requestIsAdmin(r) {
		http.Redirect(w, r, "/admin/dashboard.html", http.StatusFound)
		return
	}
	s.serveAdminHTML(w, "login.html")
}

func (s *Server) adminDashboardPage(w http.ResponseWriter, r *http.Request) {
	if !s.requestIsAdmin(r) {
		http.Redirect(w, r, "/admin/login.html", http.StatusFound)
		return
	}
	s.serveAdminHTML(w, "dashboard.html")
}

func (s *Server) serveAdminHTML(w http.ResponseWriter, file string) {
	content, err := adminFiles.ReadFile("adminstatic/" + file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (s *Server) adminStatic(w http.ResponseWriter, r *http.Request) {
	file := r.PathValue("file")
	if strings.Contains(file, "..") || file == "" {
		http.NotFound(w, r)
		return
	}
	content, err := adminFiles.ReadFile("adminstatic/" + file)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch {
	case strings.HasSuffix(file, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(file, ".js"):
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Username != s.cfg.Admin.Username || req.Password != s.cfg.Admin.Password {
		writeError(w, http.StatusUnauthorized, errors.New("invalid admin credentials"))
		return
	}
	expires := time.Now().UTC().Add(12 * time.Hour)
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    s.adminToken(req.Username, expires),
		Path:     "/admin",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"username": req.Username})
}

func (s *Server) adminLogout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    "",
		Path:     "/admin",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) adminSession(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(adminCookieName)
	if err != nil || !s.verifyAdminToken(cookie.Value) {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "username": s.cfg.Admin.Username})
}

func (s *Server) adminMe(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"username": s.cfg.Admin.Username})
}

func (s *Server) adminDashboard(w http.ResponseWriter, r *http.Request) {
	filter, err := listFilterFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, s.service.Dashboard(filter))
}

func (s *Server) adminPayments(w http.ResponseWriter, r *http.Request) {
	filter, err := listFilterFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"payments": s.service.ListFiltered(filter)})
}

func (s *Server) adminMarkLost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	payment, err := s.service.MarkLost(r.PathValue("id"), req.Reason)
	if err != nil {
		s.writePaymentError(w, err)
		return
	}
	s.logPayment("payment_marked_lost", payment)
	writeJSON(w, http.StatusOK, payment)
}

func (s *Server) adminRetryCallback(w http.ResponseWriter, r *http.Request) {
	payment, err := s.service.Get(r.PathValue("id"))
	if err != nil {
		s.writePaymentError(w, err)
		return
	}
	if payment.NotifyURL == "" {
		writeError(w, http.StatusBadRequest, errors.New("payment has no notify_url"))
		return
	}
	updated, err := s.deliverMerchantCallback(r.Context(), payment)
	if err != nil {
		s.logger.Warn("manual merchant callback retry failed", "payment_id", payment.ID, "error", err)
		s.logPayment("payment_callback_retry_failed", updated)
		writeJSON(w, http.StatusOK, updated)
		return
	}
	s.logPayment("payment_callback_retry_success", updated)
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) withAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.requestIsAdmin(r) {
			writeError(w, http.StatusUnauthorized, errors.New("admin login required"))
			return
		}
		next(w, r)
	}
}

func (s *Server) requestIsAdmin(r *http.Request) bool {
	cookie, err := r.Cookie(adminCookieName)
	return err == nil && s.verifyAdminToken(cookie.Value)
}

func (s *Server) adminToken(username string, expires time.Time) string {
	payload := username + "|" + expires.Format(time.RFC3339)
	mac := hmac.New(sha256.New, []byte(s.adminSecret()))
	mac.Write([]byte(payload))
	return payload + "|" + hex.EncodeToString(mac.Sum(nil))
}

func (s *Server) verifyAdminToken(token string) bool {
	parts := strings.Split(token, "|")
	if len(parts) != 3 {
		return false
	}
	expires, err := time.Parse(time.RFC3339, parts[1])
	if err != nil || time.Now().UTC().After(expires) {
		return false
	}
	expected := s.adminToken(parts[0], expires)
	return hmac.Equal([]byte(expected), []byte(token))
}

func (s *Server) adminSecret() string {
	if s.cfg.Admin.SessionSecret != "" {
		return s.cfg.Admin.SessionSecret
	}
	if s.cfg.API.SigningSecret != "" {
		return s.cfg.API.SigningSecret
	}
	return "pay233-dev-admin-secret"
}

func listFilterFromRequest(r *http.Request) (payment.ListFilter, error) {
	env := r.URL.Query().Get("envType")
	if env == "" {
		env = r.URL.Query().Get("env_type")
	}
	if env == "" || env == "all" {
		return payment.ListFilter{}, nil
	}
	envType, err := payment.NormalizeEnvType(env)
	if err != nil {
		return payment.ListFilter{}, err
	}
	return payment.ListFilter{EnvType: envType}, nil
}
