package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/esnunes/bobot/auth"
)

type pushSubscribeRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256DH string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

type pushUnsubscribeRequest struct {
	Endpoint string `json:"endpoint"`
}

func (s *Server) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	var req pushSubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Endpoint == "" || req.Keys.P256DH == "" || req.Keys.Auth == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}

	// Validate HTTPS endpoint (SSRF mitigation)
	u, err := url.Parse(req.Endpoint)
	if err != nil || !strings.EqualFold(u.Scheme, "https") {
		http.Error(w, "endpoint must use HTTPS", http.StatusBadRequest)
		return
	}

	if err := s.db.SavePushSubscription(userData.UserID, req.Endpoint, req.Keys.P256DH, req.Keys.Auth); err != nil {
		http.Error(w, "failed to save subscription", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var req pushUnsubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Endpoint == "" {
		http.Error(w, "missing endpoint", http.StatusBadRequest)
		return
	}

	if err := s.db.DeletePushSubscription(req.Endpoint); err != nil {
		http.Error(w, "failed to delete subscription", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
