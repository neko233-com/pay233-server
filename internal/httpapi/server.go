package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/neko233-com/pay233-server/internal/config"
	"github.com/neko233-com/pay233-server/internal/payment"
)

type Dependencies struct {
	Config   config.Config
	Registry *payment.Registry
	Store    payment.Store
	Logger   *slog.Logger
}

type Server struct {
	cfg      config.Config
	service  *payment.Service
	registry *payment.Registry
	logger   *slog.Logger
}

func NewServer(deps Dependencies) *Server {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		cfg:      deps.Config,
		service:  payment.NewService(deps.Registry, deps.Store),
		registry: deps.Registry,
		logger:   logger,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /v1/channels", s.channels)
	mux.HandleFunc("POST /v1/payments", s.withSignedBody(s.createPayment))
	mux.HandleFunc("GET /v1/payments/{id}", s.getPayment)
	mux.HandleFunc("POST /v1/payments/{id}/close", s.withSignedBody(s.closePayment))
	mux.HandleFunc("POST /v1/webhooks/{channel}", s.webhook)
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) channels(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"channels": s.registry.Channels()})
}

func (s *Server) createPayment(w http.ResponseWriter, r *http.Request, body []byte) {
	var req payment.CreatePaymentRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	created, err := s.service.Create(r.Context(), req)
	if err != nil {
		s.writePaymentError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) getPayment(w http.ResponseWriter, r *http.Request) {
	got, err := s.service.Get(r.PathValue("id"))
	if err != nil {
		s.writePaymentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, got)
}

func (s *Server) closePayment(w http.ResponseWriter, r *http.Request, _ []byte) {
	closed, err := s.service.Close(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writePaymentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, closed)
}

func (s *Server) webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	headers := make(map[string]string, len(r.Header))
	for name, values := range r.Header {
		headers[name] = strings.Join(values, ",")
	}
	updated, err := s.service.ApplyWebhook(r.Context(), r.PathValue("channel"), body, headers)
	if err != nil {
		s.writePaymentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) withSignedBody(next func(http.ResponseWriter, *http.Request, []byte)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if !verifySignature(r, s.cfg.API.SigningSecret, body) {
			writeError(w, http.StatusUnauthorized, errors.New("invalid request signature"))
			return
		}
		next(w, r, body)
	}
}

func (s *Server) writePaymentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, payment.ErrChannelNotConfigured):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, payment.ErrPaymentNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, payment.ErrInvalidPaymentState):
		writeError(w, http.StatusConflict, err)
	default:
		writeError(w, http.StatusBadRequest, err)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
