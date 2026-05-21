package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/auth"
)

// TestDaemonAuth_DaemonTokenCacheHit pins the daemon-token cache short-circuit:
// when the cache holds an entry for an mdt_ token, DaemonAuth must skip the DB
// lookup. nil queries would otherwise nil-deref on a miss.
func TestDaemonAuth_DaemonTokenCacheHit(t *testing.T) {
	rdb := newRedisTestClient(t)
	cache := auth.NewDaemonTokenCache(rdb)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}

	const rawToken = "mdt_cache_hit_test_token"
	hash := auth.HashToken(rawToken)
	cache.Set(context.Background(), hash, auth.DaemonTokenIdentity{
		WorkspaceID: "ws-cached",
		DaemonID:    "daemon-cached",
	}, auth.AuthCacheTTL)

	var gotWS, gotDaemon, gotPath string
	mw := DaemonAuth(nil, nil, cache, false) // nil queries — only safe on cache hit
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotWS = DaemonWorkspaceIDFromContext(r.Context())
		gotDaemon = DaemonIDFromContext(r.Context())
		gotPath = DaemonAuthPathFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/api/daemon/heartbeat", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on cache hit, got %d: %s", w.Code, w.Body.String())
	}
	if gotWS != "ws-cached" || gotDaemon != "daemon-cached" {
		t.Fatalf("expected (ws-cached, daemon-cached), got (%q, %q)", gotWS, gotDaemon)
	}
	if gotPath != DaemonAuthPathDaemonToken {
		t.Fatalf("expected auth path %q, got %q", DaemonAuthPathDaemonToken, gotPath)
	}
}

// TestDaemonAuth_PATCacheHit pins the PAT-fallback short-circuit. Production
// daemon traffic today uses mul_ PATs (mdt_ minting isn't wired up yet), so
// this is the cache hit that actually matters for /api/daemon/* DB load.
func TestDaemonAuth_PATCacheHit(t *testing.T) {
	rdb := newRedisTestClient(t)
	cache := auth.NewPATCache(rdb)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}

	const rawToken = "mul_daemon_pat_cache_hit_test"
	hash := auth.HashToken(rawToken)
	cache.Set(context.Background(), hash, "cached-user-id", auth.AuthCacheTTL)

	var gotUserID, gotPath string
	mw := DaemonAuth(nil, cache, nil, false)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = r.Header.Get("X-User-ID")
		gotPath = DaemonAuthPathFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/api/daemon/heartbeat", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotUserID != "cached-user-id" {
		t.Fatalf("expected cached X-User-ID, got %q", gotUserID)
	}
	if gotPath != DaemonAuthPathPAT {
		t.Fatalf("expected auth path %q, got %q", DaemonAuthPathPAT, gotPath)
	}
}

func TestDaemonAuth_MissingAuth(t *testing.T) {
	mw := DaemonAuth(nil, nil, nil, false)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next must not be called")
	}))
	req := httptest.NewRequest("POST", "/api/daemon/heartbeat", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestDaemonAuth_InvalidMDT_NilQueries(t *testing.T) {
	mw := DaemonAuth(nil, nil, nil, false) // no caches, no DB
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next must not be called")
	}))
	req := httptest.NewRequest("POST", "/api/daemon/heartbeat", nil)
	req.Header.Set("Authorization", "Bearer mdt_unknown")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// --- Strict mode tests ---

// TestDaemonAuth_StrictMode_RejectsPAT verifies that when strictDaemon=true,
// a PAT (mul_ prefix) token is rejected with 401 even if it would normally
// hit the PAT cache.
func TestDaemonAuth_StrictMode_RejectsPAT(t *testing.T) {
	rdb := newRedisTestClient(t)
	patCache := auth.NewPATCache(rdb)
	if patCache == nil {
		t.Fatal("expected non-nil patCache")
	}

	const rawToken = "mul_strict_test_pat"
	hash := auth.HashToken(rawToken)
	patCache.Set(context.Background(), hash, "some-user-id", auth.AuthCacheTTL)

	mw := DaemonAuth(nil, patCache, nil, true) // strict=true
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next must not be called in strict mode for PAT token")
	}))

	req := httptest.NewRequest("POST", "/api/daemon/heartbeat", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 in strict mode with PAT, got %d: %s", w.Code, w.Body.String())
	}
}

// TestDaemonAuth_StrictMode_RejectsJWT verifies that when strictDaemon=true,
// a JWT token is rejected with 401 even if it would otherwise be valid.
func TestDaemonAuth_StrictMode_RejectsJWT(t *testing.T) {
	// A well-formed JWT with the right secret would parse as valid,
	// but strict mode must deny before we even get to JWT parsing.
	mw := DaemonAuth(nil, nil, nil, true) // strict=true
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next must not be called in strict mode for JWT token")
	}))

	// Valid-looking JWT structure (would be valid with real HMAC secret).
	// Strict mode must reject before jwt.Parse is even called.
	jwtToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1c2VyLTEyMyJ9.invalidSig"

	req := httptest.NewRequest("POST", "/api/daemon/heartbeat", nil)
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 in strict mode with JWT, got %d: %s", w.Code, w.Body.String())
	}
}

// TestDaemonAuth_StrictMode_AcceptsMDT verifies that when strictDaemon=true,
// a valid mdt_ daemon token in the cache still allows the request.
func TestDaemonAuth_StrictMode_AcceptsMDT(t *testing.T) {
	rdb := newRedisTestClient(t)
	daemonCache := auth.NewDaemonTokenCache(rdb)
	if daemonCache == nil {
		t.Fatal("expected non-nil daemonCache")
	}

	const rawToken = "mdt_strict_mode_valid_token"
	hash := auth.HashToken(rawToken)
	daemonCache.Set(context.Background(), hash, auth.DaemonTokenIdentity{
		WorkspaceID: "ws-strict",
		DaemonID:   "daemon-strict",
	}, auth.AuthCacheTTL)

	var gotWS, gotDaemon, gotPath string
	mw := DaemonAuth(nil, nil, daemonCache, true) // strict=true
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotWS = DaemonWorkspaceIDFromContext(r.Context())
		gotDaemon = DaemonIDFromContext(r.Context())
		gotPath = DaemonAuthPathFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/api/daemon/heartbeat", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 in strict mode with valid mdt_, got %d: %s", w.Code, w.Body.String())
	}
	if gotWS != "ws-strict" || gotDaemon != "daemon-strict" {
		t.Fatalf("expected (ws-strict, daemon-strict), got (%q, %q)", gotWS, gotDaemon)
	}
	if gotPath != DaemonAuthPathDaemonToken {
		t.Fatalf("expected auth path %q, got %q", DaemonAuthPathDaemonToken, gotPath)
	}
}