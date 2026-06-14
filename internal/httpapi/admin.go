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
	"strconv"
	"strings"
	"time"

	adminstore "github.com/neko233-com/pay233-server/internal/admin"
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
	user, ok := s.userStore.Authenticate(req.Username, req.Password)
	if !ok {
		s.audit(req.Username, "", "admin_login_failed", "admin", nil)
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
	s.audit(user.Username, user.Role, "admin_login_success", "admin", nil)
	writeJSON(w, http.StatusOK, map[string]any{"username": user.Username, "role": user.Role})
}

func (s *Server) adminLogout(w http.ResponseWriter, r *http.Request) {
	if user, ok := s.currentAdmin(r); ok {
		s.audit(user.Username, user.Role, "admin_logout", "admin", nil)
	}
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
	user, ok := s.currentAdmin(r)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "username": user.Username, "role": user.Role})
}

func (s *Server) adminMe(w http.ResponseWriter, r *http.Request) {
	user, _ := s.currentAdmin(r)
	writeJSON(w, http.StatusOK, user.Public())
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
	actor, _ := s.currentAdmin(r)
	s.audit(actor.Username, actor.Role, "payment_mark_lost", payment.ID, map[string]string{
		"out_trade_no": payment.OutTradeNo,
		"env_type":     string(payment.EnvType),
		"reason":       payment.FailureReason,
	})
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
		actor, _ := s.currentAdmin(r)
		s.audit(actor.Username, actor.Role, "payment_callback_retry_failed", payment.ID, map[string]string{
			"out_trade_no": payment.OutTradeNo,
			"error":        err.Error(),
		})
		writeJSON(w, http.StatusOK, updated)
		return
	}
	actor, _ := s.currentAdmin(r)
	s.audit(actor.Username, actor.Role, "payment_callback_retry_success", payment.ID, map[string]string{
		"out_trade_no": payment.OutTradeNo,
	})
	s.logPayment("payment_callback_retry_success", updated)
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) adminUsers(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"users": s.userStore.List()})
}

func (s *Server) adminCreateUser(w http.ResponseWriter, r *http.Request) {
	var req adminstore.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actor, _ := s.currentAdmin(r)
	user, err := s.userStore.Create(actor.Username, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.audit(actor.Username, actor.Role, "admin_user_created", user.Username, map[string]string{"role": string(user.Role)})
	writeJSON(w, http.StatusCreated, user)
}

func (s *Server) adminDeleteUser(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if err := s.userStore.Delete(username); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actor, _ := s.currentAdmin(r)
	s.audit(actor.Username, actor.Role, "admin_user_deleted", username, nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) adminAudit(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": s.auditStore.List(limit)})
}

func (s *Server) adminPruneAudit(w http.ResponseWriter, r *http.Request) {
	removed, err := s.auditStore.PruneExpired(time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	actor, _ := s.currentAdmin(r)
	s.audit(actor.Username, actor.Role, "audit_pruned", "audit", map[string]string{"removed": strconv.Itoa(removed)})
	writeJSON(w, http.StatusOK, map[string]any{"removed": removed})
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

func (s *Server) withRoles(next http.HandlerFunc, roles ...adminstore.Role) http.HandlerFunc {
	allowed := make(map[adminstore.Role]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.currentAdmin(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, errors.New("admin login required"))
			return
		}
		if _, ok := allowed[user.Role]; !ok {
			writeError(w, http.StatusForbidden, errors.New("insufficient admin role"))
			return
		}
		next(w, r)
	}
}

func (s *Server) requestIsAdmin(r *http.Request) bool {
	_, ok := s.currentAdmin(r)
	return ok
}

func (s *Server) adminToken(username string, expires time.Time) string {
	payload := username + "|" + expires.Format(time.RFC3339)
	mac := hmac.New(sha256.New, []byte(s.adminSecret()))
	mac.Write([]byte(payload))
	return payload + "|" + hex.EncodeToString(mac.Sum(nil))
}

func (s *Server) currentAdmin(r *http.Request) (adminstore.User, bool) {
	cookie, err := r.Cookie(adminCookieName)
	if err != nil {
		return adminstore.User{}, false
	}
	username, ok := s.verifyAdminToken(cookie.Value)
	if !ok {
		return adminstore.User{}, false
	}
	return s.userStore.Get(username)
}

func (s *Server) verifyAdminToken(token string) (string, bool) {
	parts := strings.Split(token, "|")
	if len(parts) != 3 {
		return "", false
	}
	expires, err := time.Parse(time.RFC3339, parts[1])
	if err != nil || time.Now().UTC().After(expires) {
		return "", false
	}
	expected := s.adminToken(parts[0], expires)
	return parts[0], hmac.Equal([]byte(expected), []byte(token))
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

func (s *Server) audit(actor string, role adminstore.Role, action string, target string, details map[string]string) {
	if s.auditStore == nil {
		return
	}
	if actor == "" {
		actor = "anonymous"
	}
	if err := s.auditStore.Write(adminstore.AuditEntry{
		Actor:   actor,
		Role:    role,
		Action:  action,
		Target:  target,
		Details: details,
	}); err != nil {
		s.logger.Warn("write audit log failed", "actor", actor, "action", action, "error", err)
	}
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
