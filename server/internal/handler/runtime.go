package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/pkg/agent"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

type AgentRuntimeResponse struct {
	ID           string  `json:"id"`
	WorkspaceID  string  `json:"workspace_id"`
	DaemonID     *string `json:"daemon_id"`
	Name         string  `json:"name"`
	RuntimeMode  string  `json:"runtime_mode"`
	Provider     string  `json:"provider"`
	LaunchHeader string  `json:"launch_header"`
	Status       string  `json:"status"`
	DeviceInfo   string  `json:"device_info"`
	Metadata     any     `json:"metadata"`
	OwnerID      *string `json:"owner_id"`
	// Visibility is "private" (default — only the owner / workspace admins
	// can bind agents) or "public" (any workspace member can). See migration
	// 083 and canUseRuntimeForAgent.
	Visibility string  `json:"visibility"`
	Timezone   string  `json:"timezone"`
	LastSeenAt *string `json:"last_seen_at"`
	// HealthStatus is derived server-side from the runtime heartbeat evidence:
	// status + last_seen_at. Values: online | stale | offline | unknown.
	HealthStatus string `json:"health_status"`
	// HeartbeatAgeSeconds is the age of last_seen_at in seconds. Nil means
	// last_seen_at is not available.
	HeartbeatAgeSeconds *int64 `json:"heartbeat_age_seconds,omitempty"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

const (
	// runtimeOfflineRecentWindow bounds how long after a runtime's last
	// heartbeat we still render it as "stale" rather than "offline" once it is
	// no longer alive.
	runtimeOfflineRecentWindow = 5 * time.Minute

	// runtimeDBLivenessFallbackWindow is the maximum age of
	// agent_runtime.last_seen_at we still treat as "alive" when the hot
	// liveness store (Redis) is unavailable or errors. The DB column is
	// intentionally write-throttled (see runtimeHeartbeatDBFlushInterval, 60s,
	// in daemon.go) and bulk-coalesced by the HeartbeatScheduler, so an
	// alive-but-batched runtime can legitimately carry a last_seen_at age of
	// flush(60s) + heartbeat(15s) + batch tick(30s) = 105s. Deriving health
	// from a 45s DB threshold therefore false-flagged healthy runtimes "stale"
	// on every flush cycle.
	//
	// This window is aligned with the sweeper's staleThresholdSeconds (150s in
	// cmd/server/runtime_sweeper.go): both decide DB-fallback aliveness from the
	// same signal so listing, command dispatch, and the offline sweeper never
	// disagree about whether a runtime is alive. Keep them in lockstep — if you
	// change one, change the other and re-derive the 105s worst-case chain.
	runtimeDBLivenessFallbackWindow = 150 * time.Second
)

// resolveRuntimeLive reports whether a runtime is alive right now under the
// documented liveness contract:
//
//   - A runtime the DB has already moved out of the active "online" state is
//     authoritatively NOT live; we never resurrect it from a lingering liveness
//     key or a recent-but-batched last_seen_at.
//   - When the hot liveness store is available, Redis is the authority: the
//     runtime is live iff it holds an unexpired liveness key. last_seen_at is
//     ignored because it is intentionally throttled and would produce false
//     "stale" readings between DB flushes.
//   - When the liveness store is unavailable/errored, we fall back to the DB
//     last_seen_at window (runtimeDBLivenessFallbackWindow), matching the
//     sweeper's DB-only fallback.
//
// `alive` and `livenessAvailable` come from a prior IsAliveBatch call so list
// endpoints resolve many runtimes in a single round-trip.
func resolveRuntimeLive(rt db.AgentRuntime, alive map[string]bool, livenessAvailable bool, now time.Time) bool {
	// agent_runtime.status is DB-constrained to "online" | "offline" (migration
	// 004_agent_runtime_loop); there is no "busy" runtime state. A row the
	// sweeper has already flipped to "offline" is authoritatively not live and
	// must never be resurrected from a lingering liveness key or a
	// recent-but-batched last_seen_at.
	if rt.Status != "online" {
		return false
	}
	if livenessAvailable {
		return alive[uuidToString(rt.ID)]
	}
	if !rt.LastSeenAt.Valid {
		return false
	}
	return now.Sub(rt.LastSeenAt.Time) < runtimeDBLivenessFallbackWindow
}

// deriveRuntimeHealthStatus maps a runtime + its resolved liveness onto the
// operator-facing health signal (online | stale | offline | unknown) and the
// last_seen_at age in seconds. Aliveness is resolved upstream by
// resolveRuntimeLive; this function only grades a non-live runtime as stale
// (recently seen) vs offline (long gone). heartbeat_age_seconds intentionally
// reflects DB last_seen_at age, which under Redis-backed liveness is the age of
// the last *persisted* heartbeat, not the last network beat — a live runtime
// can therefore report a non-zero age while still being "online".
func deriveRuntimeHealthStatus(rt db.AgentRuntime, live bool, now time.Time) (string, *int64) {
	if !rt.LastSeenAt.Valid {
		if live {
			// Live with no persisted last_seen_at yet (e.g. just-registered,
			// Redis-touched but pre-first-flush). Report online without an age.
			return "online", nil
		}
		return "unknown", nil
	}

	age := now.Sub(rt.LastSeenAt.Time)
	if age < 0 {
		age = 0
	}
	ageSeconds := int64(age / time.Second)

	if live {
		return "online", &ageSeconds
	}
	if age <= runtimeOfflineRecentWindow {
		return "stale", &ageSeconds
	}
	return "offline", &ageSeconds
}

// runtimeLivenessSnapshot fetches the hot-liveness snapshot for the given
// runtimes in a single round-trip. Returns the alive map and whether the
// liveness store was available/healthy for this call. Callers pass both to
// resolveRuntimeLive so a DB-batched last_seen_at never yields a false "stale"
// reading while Redis still holds a live key.
func (h *Handler) runtimeLivenessSnapshot(ctx context.Context, rts []db.AgentRuntime) (map[string]bool, bool) {
	if h.LivenessStore == nil || !h.LivenessStore.Available() || len(rts) == 0 {
		return nil, false
	}
	ids := make([]string, len(rts))
	for i, rt := range rts {
		ids[i] = uuidToString(rt.ID)
	}
	alive, ok := h.LivenessStore.IsAliveBatch(ctx, ids)
	if !ok {
		return nil, false
	}
	return alive, true
}

// runtimeResponse builds a single runtime response, resolving health from the
// hot liveness store (single-key batch) with DB fallback.
func (h *Handler) runtimeResponse(ctx context.Context, rt db.AgentRuntime) AgentRuntimeResponse {
	alive, ok := h.runtimeLivenessSnapshot(ctx, []db.AgentRuntime{rt})
	live := resolveRuntimeLive(rt, alive, ok, time.Now())
	return runtimeToResponse(rt, live)
}

// runtimeResponses builds responses for many runtimes, resolving liveness for
// all of them in one IsAliveBatch round-trip.
func (h *Handler) runtimeResponses(ctx context.Context, rts []db.AgentRuntime) []AgentRuntimeResponse {
	alive, ok := h.runtimeLivenessSnapshot(ctx, rts)
	now := time.Now()
	resp := make([]AgentRuntimeResponse, len(rts))
	for i, rt := range rts {
		resp[i] = runtimeToResponse(rt, resolveRuntimeLive(rt, alive, ok, now))
	}
	return resp
}

func runtimeToResponse(rt db.AgentRuntime, live bool) AgentRuntimeResponse {
	var metadata any
	if rt.Metadata != nil {
		json.Unmarshal(rt.Metadata, &metadata)
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	healthStatus, heartbeatAgeSeconds := deriveRuntimeHealthStatus(rt, live, time.Now())

	return AgentRuntimeResponse{
		ID:                  uuidToString(rt.ID),
		WorkspaceID:         uuidToString(rt.WorkspaceID),
		DaemonID:            textToPtr(rt.DaemonID),
		Name:                rt.Name,
		RuntimeMode:         rt.RuntimeMode,
		Provider:            rt.Provider,
		LaunchHeader:        agent.LaunchHeader(rt.Provider),
		Status:              rt.Status,
		DeviceInfo:          rt.DeviceInfo,
		Metadata:            metadata,
		OwnerID:             uuidToPtr(rt.OwnerID),
		Visibility:          rt.Visibility,
		Timezone:            rt.Timezone,
		LastSeenAt:          timestampToPtr(rt.LastSeenAt),
		HealthStatus:        healthStatus,
		HeartbeatAgeSeconds: heartbeatAgeSeconds,
		CreatedAt:           timestampToString(rt.CreatedAt),
		UpdatedAt:           timestampToString(rt.UpdatedAt),
	}
}

// ---------------------------------------------------------------------------
// Runtime Usage
// ---------------------------------------------------------------------------

type RuntimeUsageResponse struct {
	RuntimeID        string `json:"runtime_id"`
	Date             string `json:"date"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
}

// GetRuntimeUsage returns daily token usage for a runtime, aggregated from
// per-task usage records captured by the daemon. This is scoped to
// Daemon-executed tasks only (i.e. excludes users' local CLI usage of the
// same tool).
func (h *Handler) GetRuntimeUsage(w http.ResponseWriter, r *http.Request) {
	runtimeID := chi.URLParam(r, "runtimeId")
	runtimeUUID, ok := parseUUIDOrBadRequest(w, runtimeID, "runtime_id")
	if !ok {
		return
	}

	rt, err := h.Queries.GetAgentRuntime(r.Context(), runtimeUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "runtime not found")
		return
	}

	if _, ok := h.requireWorkspaceMember(w, r, uuidToString(rt.WorkspaceID), "runtime not found"); !ok {
		return
	}

	since := parseSinceParamInTZ(r, 90, rt.Timezone)

	resp, err := h.listRuntimeUsage(r.Context(), rt.ID, rt.Timezone, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list usage")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// listRuntimeUsage dispatches between the raw task_usage scan and the
// task_usage_daily rollup based on the UseDailyRollupForRuntimeUsage
// feature flag. Both code paths return rows in the same shape, so the
// handler doesn't care which one ran.
func (h *Handler) listRuntimeUsage(ctx context.Context, runtimeID pgtype.UUID, tz string, since pgtype.Timestamptz) ([]RuntimeUsageResponse, error) {
	resolvedRuntimeID := uuidToString(runtimeID)
	if h.cfg.UseDailyRollupForRuntimeUsage {
		rows, err := h.Queries.ListRuntimeUsageDaily(ctx, db.ListRuntimeUsageDailyParams{
			RuntimeID: runtimeID,
			Since:     since,
			Tz:        tz,
		})
		if err != nil {
			return nil, err
		}
		resp := make([]RuntimeUsageResponse, len(rows))
		for i, row := range rows {
			resp[i] = RuntimeUsageResponse{
				RuntimeID:        resolvedRuntimeID,
				Date:             row.Date.Time.Format("2006-01-02"),
				Provider:         row.Provider,
				Model:            row.Model,
				InputTokens:      row.InputTokens,
				OutputTokens:     row.OutputTokens,
				CacheReadTokens:  row.CacheReadTokens,
				CacheWriteTokens: row.CacheWriteTokens,
			}
		}
		return resp, nil
	}

	rows, err := h.Queries.ListRuntimeUsage(ctx, db.ListRuntimeUsageParams{
		RuntimeID: runtimeID,
		Since:     since,
		Tz:        tz,
	})
	if err != nil {
		return nil, err
	}
	resp := make([]RuntimeUsageResponse, len(rows))
	for i, row := range rows {
		resp[i] = RuntimeUsageResponse{
			RuntimeID:        resolvedRuntimeID,
			Date:             row.Date.Time.Format("2006-01-02"),
			Provider:         row.Provider,
			Model:            row.Model,
			InputTokens:      row.InputTokens,
			OutputTokens:     row.OutputTokens,
			CacheReadTokens:  row.CacheReadTokens,
			CacheWriteTokens: row.CacheWriteTokens,
		}
	}
	return resp, nil
}

// GetRuntimeTaskActivity returns hourly task activity distribution for a runtime.
func (h *Handler) GetRuntimeTaskActivity(w http.ResponseWriter, r *http.Request) {
	runtimeID := chi.URLParam(r, "runtimeId")
	runtimeUUID, ok := parseUUIDOrBadRequest(w, runtimeID, "runtime_id")
	if !ok {
		return
	}

	rt, err := h.Queries.GetAgentRuntime(r.Context(), runtimeUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "runtime not found")
		return
	}

	if _, ok := h.requireWorkspaceMember(w, r, uuidToString(rt.WorkspaceID), "runtime not found"); !ok {
		return
	}

	rows, err := h.Queries.GetRuntimeTaskHourlyActivity(r.Context(), db.GetRuntimeTaskHourlyActivityParams{
		RuntimeID: rt.ID,
		Tz:        rt.Timezone,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get task activity")
		return
	}

	type HourlyActivity struct {
		Hour  int `json:"hour"`
		Count int `json:"count"`
	}

	resp := make([]HourlyActivity, len(rows))
	for i, row := range rows {
		resp[i] = HourlyActivity{Hour: int(row.Hour), Count: int(row.Count)}
	}

	writeJSON(w, http.StatusOK, resp)
}

// RuntimeUsageByAgentResponse is one (agent, model) row of "Cost by agent".
// Model stays on the wire because cost is computed client-side from a model
// pricing table, intentionally not stored server-side so pricing changes
// don't require a back-fill. The client groups by agent_id and sums.
type RuntimeUsageByAgentResponse struct {
	AgentID          string `json:"agent_id"`
	Model            string `json:"model"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	TaskCount        int32  `json:"task_count"`
}

// GetRuntimeUsageByAgent returns per-agent token aggregates for a runtime
// since the cutoff window. Drives the runtime-detail "Cost by agent" tab.
func (h *Handler) GetRuntimeUsageByAgent(w http.ResponseWriter, r *http.Request) {
	runtimeID := chi.URLParam(r, "runtimeId")

	rt, err := h.Queries.GetAgentRuntime(r.Context(), parseUUID(runtimeID))
	if err != nil {
		writeError(w, http.StatusNotFound, "runtime not found")
		return
	}

	if _, ok := h.requireWorkspaceMember(w, r, uuidToString(rt.WorkspaceID), "runtime not found"); !ok {
		return
	}

	since := parseSinceParamInTZ(r, 30, rt.Timezone)

	rows, err := h.Queries.ListRuntimeUsageByAgent(r.Context(), db.ListRuntimeUsageByAgentParams{
		RuntimeID: parseUUID(runtimeID),
		Since:     since,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list usage by agent")
		return
	}

	resp := make([]RuntimeUsageByAgentResponse, len(rows))
	for i, row := range rows {
		resp[i] = RuntimeUsageByAgentResponse{
			AgentID:          uuidToString(row.AgentID),
			Model:            row.Model,
			InputTokens:      row.InputTokens,
			OutputTokens:     row.OutputTokens,
			CacheReadTokens:  row.CacheReadTokens,
			CacheWriteTokens: row.CacheWriteTokens,
			TaskCount:        row.TaskCount,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// RuntimeUsageByHourResponse is one (hour, model) row. Hours with zero
// activity are omitted by the SQL — clients fill the gap to render a
// continuous 0..23 axis. Model is preserved for client-side cost math.
type RuntimeUsageByHourResponse struct {
	Hour             int    `json:"hour"`
	Model            string `json:"model"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	TaskCount        int32  `json:"task_count"`
}

// GetRuntimeUsageByHour returns hourly (0..23) token aggregates for a
// runtime since the cutoff window. Drives the "By hour" tab.
func (h *Handler) GetRuntimeUsageByHour(w http.ResponseWriter, r *http.Request) {
	runtimeID := chi.URLParam(r, "runtimeId")

	rt, err := h.Queries.GetAgentRuntime(r.Context(), parseUUID(runtimeID))
	if err != nil {
		writeError(w, http.StatusNotFound, "runtime not found")
		return
	}

	if _, ok := h.requireWorkspaceMember(w, r, uuidToString(rt.WorkspaceID), "runtime not found"); !ok {
		return
	}

	since := parseSinceParamInTZ(r, 30, rt.Timezone)

	rows, err := h.Queries.GetRuntimeUsageByHour(r.Context(), db.GetRuntimeUsageByHourParams{
		RuntimeID: parseUUID(runtimeID),
		Since:     since,
		Tz:        rt.Timezone,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get usage by hour")
		return
	}

	resp := make([]RuntimeUsageByHourResponse, len(rows))
	for i, row := range rows {
		resp[i] = RuntimeUsageByHourResponse{
			Hour:             int(row.Hour),
			Model:            row.Model,
			InputTokens:      row.InputTokens,
			OutputTokens:     row.OutputTokens,
			CacheReadTokens:  row.CacheReadTokens,
			CacheWriteTokens: row.CacheWriteTokens,
			TaskCount:        row.TaskCount,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetWorkspaceUsageByDay returns daily token usage aggregated by model for the workspace.
func (h *Handler) GetWorkspaceUsageByDay(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	since := parseSinceParam(r, 30)

	rows, err := h.Queries.GetWorkspaceUsageByDay(r.Context(), db.GetWorkspaceUsageByDayParams{
		WorkspaceID: parseUUID(workspaceID),
		Since:       since,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get usage")
		return
	}

	type DailyUsageRow struct {
		Date                  string `json:"date"`
		Model                 string `json:"model"`
		TotalInputTokens      int64  `json:"total_input_tokens"`
		TotalOutputTokens     int64  `json:"total_output_tokens"`
		TotalCacheReadTokens  int64  `json:"total_cache_read_tokens"`
		TotalCacheWriteTokens int64  `json:"total_cache_write_tokens"`
		TaskCount             int32  `json:"task_count"`
	}

	resp := make([]DailyUsageRow, len(rows))
	for i, row := range rows {
		resp[i] = DailyUsageRow{
			Date:                  row.Date.Time.Format("2006-01-02"),
			Model:                 row.Model,
			TotalInputTokens:      row.TotalInputTokens,
			TotalOutputTokens:     row.TotalOutputTokens,
			TotalCacheReadTokens:  row.TotalCacheReadTokens,
			TotalCacheWriteTokens: row.TotalCacheWriteTokens,
			TaskCount:             row.TaskCount,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetWorkspaceUsageSummary returns total token usage aggregated by model for the workspace.
func (h *Handler) GetWorkspaceUsageSummary(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	since := parseSinceParam(r, 30)

	rows, err := h.Queries.GetWorkspaceUsageSummary(r.Context(), db.GetWorkspaceUsageSummaryParams{
		WorkspaceID: parseUUID(workspaceID),
		Since:       since,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get usage summary")
		return
	}

	type UsageSummaryRow struct {
		Model                 string `json:"model"`
		TotalInputTokens      int64  `json:"total_input_tokens"`
		TotalOutputTokens     int64  `json:"total_output_tokens"`
		TotalCacheReadTokens  int64  `json:"total_cache_read_tokens"`
		TotalCacheWriteTokens int64  `json:"total_cache_write_tokens"`
		TaskCount             int32  `json:"task_count"`
	}

	resp := make([]UsageSummaryRow, len(rows))
	for i, row := range rows {
		resp[i] = UsageSummaryRow{
			Model:                 row.Model,
			TotalInputTokens:      row.TotalInputTokens,
			TotalOutputTokens:     row.TotalOutputTokens,
			TotalCacheReadTokens:  row.TotalCacheReadTokens,
			TotalCacheWriteTokens: row.TotalCacheWriteTokens,
			TaskCount:             row.TaskCount,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// parseSinceParam parses the "days" query parameter and returns a timestamptz.
// Wall-clock window relative to UTC. Use parseSinceParamInTZ when the cutoff
// must align with a per-runtime calendar boundary (so `days=N` returns N
// full local days under the runtime's tz instead of N×24h sliding window).
func parseSinceParam(r *http.Request, defaultDays int) pgtype.Timestamptz {
	days := defaultDays
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 && parsed <= 365 {
			days = parsed
		}
	}
	t := time.Now().AddDate(0, 0, -days)
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// parseSinceParamInTZ is the timezone-aware variant of parseSinceParam.
// Anchors the cutoff to start-of-day-(N) in the supplied IANA zone so that
// `days=N` returns full N+1 calendar buckets in that zone (today's partial
// bucket + N prior full days). If tzName is empty or unparseable, falls back
// to UTC — never returns an error so handlers stay simple.
func parseSinceParamInTZ(r *http.Request, defaultDays int, tzName string) pgtype.Timestamptz {
	days := defaultDays
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 && parsed <= 365 {
			days = parsed
		}
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil || loc == nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	cutoff := startOfToday.AddDate(0, 0, -days)
	return pgtype.Timestamptz{Time: cutoff, Valid: true}
}

// UpdateAgentRuntimeRequest is the JSON body accepted by PATCH /api/runtimes/:id.
// Only fields users may legitimately edit are listed; other runtime metadata
// (provider, daemon_id, status…) flows in from the daemon and is read-only here.
type UpdateAgentRuntimeRequest struct {
	// Timezone is an IANA zone name (e.g. "Asia/Shanghai", "America/New_York").
	// Validated server-side via time.LoadLocation; "UTC" or empty resets to UTC.
	Timezone *string `json:"timezone,omitempty"`
	// Visibility flips a runtime between "private" (default — only the owner
	// or workspace admins can bind agents) and "public" (any workspace
	// member can). Owner / workspace admin only, gated by canEditRuntime.
	Visibility *string `json:"visibility,omitempty"`
}

// UpdateAgentRuntime handles PATCH /api/runtimes/:id. Currently only the
// reporting timezone is editable, but the request shape is open-ended so
// future fields (display name, description) can be added without a route
// change. Workspace-membership-checked; no admin-only restriction since the
// runtime owner traditionally edits their own runtime.
func (h *Handler) UpdateAgentRuntime(w http.ResponseWriter, r *http.Request) {
	runtimeID := chi.URLParam(r, "runtimeId")
	runtimeUUID, ok := parseUUIDOrBadRequest(w, runtimeID, "runtime_id")
	if !ok {
		return
	}

	rt, err := h.Queries.GetAgentRuntime(r.Context(), runtimeUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "runtime not found")
		return
	}

	member, ok := h.requireWorkspaceMember(w, r, uuidToString(rt.WorkspaceID), "runtime not found")
	if !ok {
		return
	}
	if !canEditRuntime(member, rt) {
		writeError(w, http.StatusForbidden, "you can only edit your own runtimes")
		return
	}

	var req UpdateAgentRuntimeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Validate every field that's present BEFORE running any mutation. A
	// PATCH that carries both `timezone` and `visibility` must succeed or
	// fail atomically from the caller's perspective: writing timezone first
	// and then 400-ing on a bad visibility would leave the row half-updated
	// (and the usage rollup rebuilt under a tz the caller never asked for).
	//
	// This loop also fixes the no-op short-circuit: the prior version
	// returned early when `timezone == rt.Timezone`, silently dropping a
	// concurrent visibility patch in the same request body.
	var (
		newTimezone    string
		needTimezone   bool
		newVisibility  string
		needVisibility bool
	)
	if req.Timezone != nil {
		tz := *req.Timezone
		if tz == "" {
			tz = "UTC"
		}
		if _, err := time.LoadLocation(tz); err != nil {
			writeError(w, http.StatusBadRequest, "invalid IANA timezone")
			return
		}
		if tz != rt.Timezone {
			newTimezone = tz
			needTimezone = true
		}
	}
	if req.Visibility != nil {
		v := *req.Visibility
		if v != "private" && v != "public" {
			writeError(w, http.StatusBadRequest, "visibility must be 'private' or 'public'")
			return
		}
		if v != rt.Visibility {
			newVisibility = v
			needVisibility = true
		}
	}

	if needTimezone {
		tx, err := h.TxStarter.Begin(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update runtime")
			return
		}
		defer tx.Rollback(r.Context())

		qtx := h.Queries.WithTx(tx)
		if err := qtx.LockTaskUsageDailyRollup(r.Context()); err != nil {
			slog.Error("LockTaskUsageDailyRollup failed", "error", err, "runtime_id", runtimeID)
			writeError(w, http.StatusInternalServerError, "failed to update runtime")
			return
		}
		updated, err := qtx.UpdateAgentRuntimeTimezone(r.Context(), db.UpdateAgentRuntimeTimezoneParams{
			ID:       runtimeUUID,
			Timezone: newTimezone,
		})
		if err != nil {
			slog.Error("UpdateAgentRuntimeTimezone failed", "error", err, "runtime_id", runtimeID)
			writeError(w, http.StatusInternalServerError, "failed to update runtime")
			return
		}
		if _, err := qtx.DeleteTaskUsageDailyForRuntime(r.Context(), runtimeUUID); err != nil {
			slog.Error("DeleteTaskUsageDailyForRuntime failed", "error", err, "runtime_id", runtimeID)
			writeError(w, http.StatusInternalServerError, "failed to rebuild runtime usage")
			return
		}
		if _, err := qtx.DeleteTaskUsageDailyDirtyForRuntime(r.Context(), runtimeUUID); err != nil {
			slog.Error("DeleteTaskUsageDailyDirtyForRuntime failed", "error", err, "runtime_id", runtimeID)
			writeError(w, http.StatusInternalServerError, "failed to rebuild runtime usage")
			return
		}
		if _, err := qtx.InsertTaskUsageDailyForRuntime(r.Context(), runtimeUUID); err != nil {
			slog.Error("InsertTaskUsageDailyForRuntime failed", "error", err, "runtime_id", runtimeID)
			writeError(w, http.StatusInternalServerError, "failed to rebuild runtime usage")
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			slog.Error("runtime timezone transaction commit failed", "error", err, "runtime_id", runtimeID)
			writeError(w, http.StatusInternalServerError, "failed to update runtime")
			return
		}
		rt = updated
	}

	if needVisibility {
		updated, err := h.Queries.UpdateAgentRuntimeVisibility(r.Context(), db.UpdateAgentRuntimeVisibilityParams{
			ID:         runtimeUUID,
			Visibility: newVisibility,
		})
		if err != nil {
			slog.Error("UpdateAgentRuntimeVisibility failed", "error", err, "runtime_id", runtimeID)
			writeError(w, http.StatusInternalServerError, "failed to update runtime")
			return
		}
		rt = updated
		// Notify connected clients that runtime metadata changed so the
		// list/detail pages refresh — matches the pattern used by
		// DeleteAgentRuntime.
		h.publish(protocol.EventDaemonRegister, uuidToString(rt.WorkspaceID), "member", uuidToString(member.UserID), map[string]any{
			"action": "update",
		})
	}

	writeJSON(w, http.StatusOK, h.runtimeResponse(r.Context(), rt))
}

func canEditRuntime(member db.Member, rt db.AgentRuntime) bool {
	if roleAllowed(member.Role, "owner", "admin") {
		return true
	}
	return rt.OwnerID.Valid && uuidToString(rt.OwnerID) == uuidToString(member.UserID)
}

// canUseRuntimeForAgent reports whether a workspace member is allowed to
// bind a new agent to — or move an existing agent onto — the given runtime.
// Mirrors canEditRuntime but layers on the runtime's visibility flag so a
// `public` runtime is usable by anyone in the workspace while a `private`
// runtime stays bound to its owner. Workspace owners/admins keep an
// administrative override for both. See migration 083 for the visibility
// column.
func canUseRuntimeForAgent(member db.Member, rt db.AgentRuntime) bool {
	if roleAllowed(member.Role, "owner", "admin") {
		return true
	}
	if rt.Visibility == "public" {
		return true
	}
	return rt.OwnerID.Valid && uuidToString(rt.OwnerID) == uuidToString(member.UserID)
}

func (h *Handler) ListAgentRuntimes(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)

	var runtimes []db.AgentRuntime
	var err error

	if ownerFilter := r.URL.Query().Get("owner"); ownerFilter == "me" {
		userID, ok := requireUserID(w, r)
		if !ok {
			return
		}
		runtimes, err = h.Queries.ListAgentRuntimesByOwner(r.Context(), db.ListAgentRuntimesByOwnerParams{
			WorkspaceID: parseUUID(workspaceID),
			OwnerID:     parseUUID(userID),
		})
	} else {
		runtimes, err = h.Queries.ListAgentRuntimes(r.Context(), parseUUID(workspaceID))
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list runtimes")
		return
	}

	resp := h.runtimeResponses(r.Context(), runtimes)

	writeJSON(w, http.StatusOK, resp)
}

// DeleteAgentRuntime deletes a runtime after permission and dependency checks.
func (h *Handler) DeleteAgentRuntime(w http.ResponseWriter, r *http.Request) {
	runtimeID := chi.URLParam(r, "runtimeId")
	runtimeUUID, ok := parseUUIDOrBadRequest(w, runtimeID, "runtime_id")
	if !ok {
		return
	}

	rt, err := h.Queries.GetAgentRuntime(r.Context(), runtimeUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "runtime not found")
		return
	}

	wsID := uuidToString(rt.WorkspaceID)
	member, ok := h.requireWorkspaceMember(w, r, wsID, "runtime not found")
	if !ok {
		return
	}

	// Permission: owner/admin can delete any runtime; members can only delete their own.
	if !canEditRuntime(member, rt) {
		writeError(w, http.StatusForbidden, "you can only delete your own runtimes")
		return
	}
	userID := uuidToString(member.UserID)

	// Check if any active (non-archived) agents are bound to this runtime.
	activeCount, err := h.Queries.CountActiveAgentsByRuntime(r.Context(), rt.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check runtime dependencies")
		return
	}
	if activeCount > 0 {
		writeError(w, http.StatusConflict, "cannot delete runtime: it has active agents bound to it. Archive or reassign the agents first.")
		return
	}

	// Remove archived agents so the FK constraint (ON DELETE RESTRICT) won't block deletion.
	if err := h.Queries.DeleteArchivedAgentsByRuntime(r.Context(), rt.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to clean up archived agents")
		return
	}

	if err := h.Queries.DeleteAgentRuntime(r.Context(), rt.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete runtime")
		return
	}

	slog.Info("runtime deleted", "runtime_id", uuidToString(rt.ID), "deleted_by", userID)

	// Notify frontend to refresh runtime list.
	h.publish(protocol.EventDaemonRegister, wsID, "member", userID, map[string]any{
		"action": "delete",
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
