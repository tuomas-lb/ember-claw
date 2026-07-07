package api

import (
	"context"
	"encoding/json"
	"net/http"

	"strconv"

	"github.com/tuomas-lb/ember-claw/dashboard/internal/config"
	grpcclient "github.com/tuomas-lb/ember-claw/dashboard/internal/grpc"
	"github.com/tuomas-lb/ember-claw/dashboard/internal/k8s"
	"github.com/tuomas-lb/ember-claw/dashboard/internal/providers"
	"github.com/tuomas-lb/ember-claw/dashboard/internal/store"
	"github.com/go-chi/chi/v5"
)

// Handler holds the dependencies needed by all HTTP handlers.
type Handler struct {
	k8s   *k8s.Client
	grpc  *grpcclient.Client
	store *store.Store // may be nil when DATABASE_URL is not configured
}

// NewHandler constructs a Handler. store may be nil (persistence disabled).
func NewHandler(k8sClient *k8s.Client, grpcClient *grpcclient.Client, chatStore *store.Store) *Handler {
	return &Handler{k8s: k8sClient, grpc: grpcClient, store: chatStore}
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func instanceName(r *http.Request) string {
	return chi.URLParam(r, "name")
}

// --------------------------------------------------------------------------
// Instance handlers
// --------------------------------------------------------------------------

// ListInstances handles GET /api/instances.
func (h *Handler) ListInstances(w http.ResponseWriter, r *http.Request) {
	instances, err := h.k8s.ListInstances(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, instances)
}

// GetInstance handles GET /api/instances/{name}.
func (h *Handler) GetInstance(w http.ResponseWriter, r *http.Request) {
	name := instanceName(r)

	status, err := h.k8s.GetInstance(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Try to enrich with gRPC status if the pod is ready.
	if status.Ready {
		grpcStatus, err := h.grpcStatus(r.Context(), name)
		if err == nil {
			status.GRPCStatus = grpcStatus
		}
	}

	writeJSON(w, http.StatusOK, status)
}

// DeployInstance handles POST /api/instances.
func (h *Handler) DeployInstance(w http.ResponseWriter, r *http.Request) {
	var req config.DeployRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}
	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model is required")
		return
	}
	if req.APIKey == "" {
		writeError(w, http.StatusBadRequest, "api_key is required")
		return
	}

	if err := h.k8s.DeployInstance(r.Context(), req); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"name": req.Name, "status": "deploying"})
}

// DeleteInstance handles DELETE /api/instances/{name}.
func (h *Handler) DeleteInstance(w http.ResponseWriter, r *http.Request) {
	name := instanceName(r)
	if err := h.k8s.DeleteInstance(r.Context(), name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name, "status": "deleted"})
}

// RestartInstance handles POST /api/instances/{name}/restart.
func (h *Handler) RestartInstance(w http.ResponseWriter, r *http.Request) {
	name := instanceName(r)
	if err := h.k8s.RestartInstance(r.Context(), name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name, "status": "restarting"})
}

// GetConfig handles GET /api/instances/{name}/config.
func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	name := instanceName(r)
	cfg, err := h.k8s.GetConfig(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// PushConfig handles PUT /api/instances/{name}/config.
func (h *Handler) PushConfig(w http.ResponseWriter, r *http.Request) {
	name := instanceName(r)

	var cfg map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if err := h.k8s.PushConfig(r.Context(), name, cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name, "status": "updated"})
}

// SetSecret handles POST /api/instances/{name}/secret.
func (h *Handler) SetSecret(w http.ResponseWriter, r *http.Request) {
	name := instanceName(r)

	var update config.SecretUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if update.Key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}

	if err := h.k8s.SetSecret(r.Context(), name, update.Key, update.Value); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name, "key": update.Key, "status": "updated"})
}

// GetInstanceStatus handles GET /api/instances/{name}/status.
func (h *Handler) GetInstanceStatus(w http.ResponseWriter, r *http.Request) {
	name := instanceName(r)
	grpcStatus, err := h.grpcStatus(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, grpcStatus)
}

// ListProviders handles GET /api/providers.
func (h *Handler) ListProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, config.SupportedProviders)
}

// --------------------------------------------------------------------------
// Internal helpers
// --------------------------------------------------------------------------

// ListModels handles GET /api/providers/{provider}/models?api_key=...
func (h *Handler) ListModels(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	apiKey := r.URL.Query().Get("api_key")
	if apiKey == "" {
		writeError(w, http.StatusBadRequest, "api_key query parameter required")
		return
	}

	models, err := providers.ListModels(r.Context(), provider, apiKey)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models)
}

// GetCallRouting handles GET /api/telephony/routing.
func (h *Handler) GetCallRouting(w http.ResponseWriter, r *http.Request) {
	data, err := h.k8s.GetCallRouting(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// PutCallRouting handles PUT /api/telephony/routing.
func (h *Handler) PutCallRouting(w http.ResponseWriter, r *http.Request) {
	var data map[string]string
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.k8s.PutCallRouting(r.Context(), data); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// RestartVoiceBridge handles POST /api/telephony/restart.
func (h *Handler) RestartVoiceBridge(w http.ResponseWriter, r *http.Request) {
	if err := h.k8s.RestartDeployment(r.Context(), "voice-bridge"); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "restarted"})
}

// GetFleetMD handles GET /api/fleet.
func (h *Handler) GetFleetMD(w http.ResponseWriter, r *http.Request) {
	content, err := h.k8s.GetFleetMD(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": content})
}

// PutFleetMD handles PUT /api/fleet.
func (h *Handler) PutFleetMD(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.k8s.PutFleetMD(r.Context(), req.Content); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ListMessages handles GET /api/instances/{name}/messages?session=<id>&limit=N.
// Returns persisted chat history oldest-first. Empty session → all sessions.
func (h *Handler) ListMessages(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	name := instanceName(r)
	session := r.URL.Query().Get("session")
	limit := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			limit = n
		}
	}
	msgs, err := h.store.ListMessages(r.Context(), name, session, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, msgs)
}

// ListSessions handles GET /api/instances/{name}/sessions.
func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	name := instanceName(r)
	sessions, err := h.store.ListSessions(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (h *Handler) grpcStatus(ctx context.Context, name string) (*config.GRPCStatus, error) {
	resp, err := h.grpc.Status(ctx, name)
	if err != nil {
		return nil, err
	}
	return &config.GRPCStatus{
		Ready:         resp.GetReady(),
		Model:         resp.GetModel(),
		Provider:      resp.GetProvider(),
		UptimeSeconds: resp.GetUptimeSeconds(),
	}, nil
}
