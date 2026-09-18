package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"cee.io/pkg/auth"
	"github.com/go-chi/chi/v5"
)

// POST /api/keys
func (h *Handler) GenerateKey(w http.ResponseWriter, r *http.Request) {
	if h.keyStore == nil {
		respondError(w, http.StatusServiceUnavailable, "Service Unavailable", "Key management store is not available")
		return
	}

	caller := GetKeyFromContext(r.Context())
	if caller == nil {
		respondError(w, http.StatusUnauthorized, "Unauthorized", "Invalid or unauthorized API key")
		return
	}

	var req auth.GenerateKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid Request", "Malformed JSON request body")
		return
	}

	req.Type = auth.CredentialType(strings.ToLower(string(req.Type)))
	req.Role = auth.Role(strings.ToLower(string(req.Role)))

	if err := auth.ValidateCombination(req.Type, req.Role); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid Request", err.Error())
		return
	}

	if !auth.CanGenerate(caller, req.Type, req.Role) {
		respondError(w, http.StatusForbidden, "Forbidden", "Invalid or unauthorized API key")
		return
	}

	key, rawKey, err := auth.GenerateKey(req.Type, req.Role, caller.ID, req.Description)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}

	if err := h.keyStore.CreateKey(r.Context(), key); err != nil {
		respondError(w, http.StatusInternalServerError, "Internal Server Error", fmt.Sprintf("Failed saving key: %v", err))
		return
	}

	respondJSON(w, http.StatusCreated, auth.GenerateKeyResponse{
		ID:        key.ID,
		Type:      key.Type,
		Role:      key.Role,
		APIKey:    rawKey,
		Prefix:    key.Prefix,
		CreatedAt: key.CreatedAt,
		CreatedBy: key.CreatedBy,
	})
}

// GET /api/keys
func (h *Handler) ListKeys(w http.ResponseWriter, r *http.Request) {
	if h.keyStore == nil {
		respondJSON(w, http.StatusOK, []any{})
		return
	}

	caller := GetKeyFromContext(r.Context())
	if caller == nil || caller.Type != auth.TypeAuth || caller.Role != auth.RoleMaster {
		respondError(w, http.StatusForbidden, "Forbidden", "Invalid or unauthorized API key")
		return
	}

	keys, err := h.keyStore.ListKeys(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}

	respondJSON(w, http.StatusOK, keys)
}

// DELETE /api/keys/{id}
func (h *Handler) RevokeKey(w http.ResponseWriter, r *http.Request) {
	if h.keyStore == nil {
		respondError(w, http.StatusServiceUnavailable, "Service Unavailable", "Key management store is not available")
		return
	}

	caller := GetKeyFromContext(r.Context())
	if caller == nil || caller.Type != auth.TypeAuth || caller.Role != auth.RoleMaster {
		respondError(w, http.StatusForbidden, "Forbidden", "Invalid or unauthorized API key")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "Invalid Request", "Key ID is required")
		return
	}

	err := h.keyStore.RevokeKey(r.Context(), id)
	if err != nil {
		if errors.Is(err, auth.ErrKeyNotFound) {
			respondError(w, http.StatusNotFound, "Not Found", "API key not found")
			return
		}
		if errors.Is(err, auth.ErrLastMaster) {
			respondError(w, http.StatusConflict, "Conflict", "Cannot revoke the last active master AUTH API key")
			return
		}
		respondError(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status":  "revoked",
		"id":      id,
		"message": "API key successfully revoked",
	})
}

// GET /api/capabilities
func (h *Handler) GetCapabilities(w http.ResponseWriter, r *http.Request) {
	caller := GetKeyFromContext(r.Context())
	if caller == nil {
		respondError(w, http.StatusUnauthorized, "Unauthorized", "Invalid or unauthorized API key")
		return
	}

	expectedRole := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("role")))
	if expectedRole == "" {
		expectedRole = strings.ToLower(strings.TrimSpace(r.Header.Get("X-Expected-Role")))
	}

	if expectedRole != "" {
		if caller.Type != auth.TypeAuth || !strings.EqualFold(string(caller.Role), expectedRole) {
			respondError(w, http.StatusForbidden, "Forbidden", "Invalid or unauthorized API key")
			return
		}
	}

	info := auth.CapabilityInfo{
		Authenticated:  true,
		KeyID:          caller.ID,
		Prefix:         caller.Prefix,
		CredentialType: caller.Type,
		Role:           caller.Role,
		Permissions:    auth.ListPermissions(caller),
	}

	respondJSON(w, http.StatusOK, info)
}

// POST /api/auth/login or /api/auth/verify
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	caller := GetKeyFromContext(r.Context())
	if caller == nil {
		respondError(w, http.StatusUnauthorized, "Unauthorized", "Invalid or unauthorized API key")
		return
	}

	var req struct {
		Role string `json:"role"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	expectedRole := strings.ToLower(strings.TrimSpace(req.Role))
	if expectedRole == "" {
		expectedRole = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("role")))
	}
	if expectedRole == "" {
		expectedRole = strings.ToLower(strings.TrimSpace(r.Header.Get("X-Expected-Role")))
	}

	if expectedRole != "" {
		if caller.Type != auth.TypeAuth || !strings.EqualFold(string(caller.Role), expectedRole) {
			respondError(w, http.StatusForbidden, "Forbidden", "Invalid or unauthorized API key")
			return
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status":  "success",
		"message": "Successfully logged in!",
		"key_id":  caller.ID,
		"prefix":  caller.Prefix,
	})
}

// POST /api/auth/logout
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"status":  "success",
		"message": "Successfully logged out!",
	})
}

