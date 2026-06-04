package handler

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// testRuntimeUUID is a fixed UUID used to key the liveness map in unit tests.
var testRuntimeUUID = util.MustParseUUID("11111111-1111-1111-1111-111111111111")

func aliveMap(id pgtype.UUID, alive bool) map[string]bool {
	return map[string]bool{uuidToString(id): alive}
}

// TestResolveRuntimeLive pins the documented liveness contract: Redis is the
// authority when available, the DB last_seen_at window is the fallback when it
// is not, and a runtime the DB has moved out of an active state is never live.
func TestResolveRuntimeLive(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name              string
		rt                db.AgentRuntime
		alive             map[string]bool
		livenessAvailable bool
		want              bool
	}{
		{
			// The flap fix: stored online + DB last_seen 2m old (well past the
			// former 45s threshold) but a live Redis key → still alive.
			name:              "redis alive overrides stale DB timestamp",
			rt:                db.AgentRuntime{ID: testRuntimeUUID, Status: "online", LastSeenAt: ts(now.Add(-2 * time.Minute))},
			alive:             aliveMap(testRuntimeUUID, true),
			livenessAvailable: true,
			want:              true,
		},
		{
			// Redis is authoritative the other direction too: a fresh DB
			// timestamp does not resurrect a runtime whose hot key has expired.
			name:              "redis dead overrides fresh DB timestamp",
			rt:                db.AgentRuntime{ID: testRuntimeUUID, Status: "online", LastSeenAt: ts(now.Add(-5 * time.Second))},
			alive:             aliveMap(testRuntimeUUID, false),
			livenessAvailable: true,
			want:              false,
		},
		{
			// agent_runtime.status is DB-constrained to online|offline; a row the
			// sweeper already flipped offline is never resurrected by a stray key.
			name:              "offline status never live even with stray live key",
			rt:                db.AgentRuntime{ID: testRuntimeUUID, Status: "offline", LastSeenAt: ts(now.Add(-5 * time.Second))},
			alive:             aliveMap(testRuntimeUUID, true),
			livenessAvailable: true,
			want:              false,
		},
		{
			// DB fallback: liveness store unavailable, last_seen within the
			// documented 150s envelope (covers the 105s worst-case batched age).
			name:              "db fallback alive within window",
			rt:                db.AgentRuntime{ID: testRuntimeUUID, Status: "online", LastSeenAt: ts(now.Add(-100 * time.Second))},
			livenessAvailable: false,
			want:              true,
		},
		{
			name:              "db fallback dead beyond window",
			rt:                db.AgentRuntime{ID: testRuntimeUUID, Status: "online", LastSeenAt: ts(now.Add(-200 * time.Second))},
			livenessAvailable: false,
			want:              false,
		},
		{
			name:              "db fallback conservative when last_seen missing",
			rt:                db.AgentRuntime{ID: testRuntimeUUID, Status: "online"},
			livenessAvailable: false,
			want:              false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveRuntimeLive(tt.rt, tt.alive, tt.livenessAvailable, now); got != tt.want {
				t.Fatalf("resolveRuntimeLive = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeriveRuntimeHealthStatus(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		rt         db.AgentRuntime
		live       bool
		wantStatus string
		wantAgeSec *int64
	}{
		{
			name:       "unknown when last seen missing and not live",
			rt:         db.AgentRuntime{Status: "online"},
			live:       false,
			wantStatus: "unknown",
			wantAgeSec: nil,
		},
		{
			name:       "online without age when live but no persisted last_seen",
			rt:         db.AgentRuntime{Status: "online"},
			live:       true,
			wantStatus: "online",
			wantAgeSec: nil,
		},
		{
			// The flap fix at the health layer: a live runtime stays "online"
			// even though its persisted last_seen_at is 2 minutes old.
			name:       "online when live despite old persisted heartbeat",
			rt:         db.AgentRuntime{Status: "online", LastSeenAt: ts(now.Add(-2 * time.Minute))},
			live:       true,
			wantStatus: "online",
			wantAgeSec: ptrInt64(120),
		},
		{
			name:       "stale when not live but recently seen",
			rt:         db.AgentRuntime{Status: "online", LastSeenAt: ts(now.Add(-2 * time.Minute))},
			live:       false,
			wantStatus: "stale",
			wantAgeSec: ptrInt64(120),
		},
		{
			name:       "offline when not live and gone for a while",
			rt:         db.AgentRuntime{Status: "offline", LastSeenAt: ts(now.Add(-10 * time.Minute))},
			live:       false,
			wantStatus: "offline",
			wantAgeSec: ptrInt64(600),
		},
		{
			name:       "future timestamp clamps age to zero",
			rt:         db.AgentRuntime{Status: "online", LastSeenAt: ts(now.Add(10 * time.Second))},
			live:       true,
			wantStatus: "online",
			wantAgeSec: ptrInt64(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotAge := deriveRuntimeHealthStatus(tt.rt, tt.live, now)
			if gotStatus != tt.wantStatus {
				t.Fatalf("status: got %q want %q", gotStatus, tt.wantStatus)
			}
			if (gotAge == nil) != (tt.wantAgeSec == nil) {
				t.Fatalf("age nil mismatch: got %v want %v", gotAge == nil, tt.wantAgeSec == nil)
			}
			if gotAge != nil && tt.wantAgeSec != nil && *gotAge != *tt.wantAgeSec {
				t.Fatalf("age seconds: got %d want %d", *gotAge, *tt.wantAgeSec)
			}
		})
	}
}

// TestRuntimeHealthFlapFixEndToEnd reproduces the exact production flap at the
// resolution+derivation seam: a continuously-alive runtime whose DB last_seen_at
// has aged past the old 45s threshold must resolve "online", not "stale".
func TestRuntimeHealthFlapFixEndToEnd(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	rt := db.AgentRuntime{
		ID:         testRuntimeUUID,
		Status:     "online",
		LastSeenAt: ts(now.Add(-80 * time.Second)), // past the former 45s window
	}

	// Liveness store reports the runtime alive (Redis key present), as observed
	// in the live dogfood reproduction.
	live := resolveRuntimeLive(rt, aliveMap(testRuntimeUUID, true), true, now)
	status, _ := deriveRuntimeHealthStatus(rt, live, now)
	if status != "online" {
		t.Fatalf("alive runtime with 80s-old DB heartbeat resolved %q, want \"online\" (flap regression)", status)
	}

	// When the hot liveness store is unavailable, the DB fallback still keeps a
	// recently-batched runtime online within the documented envelope.
	liveFallback := resolveRuntimeLive(rt, nil, false, now)
	fallbackStatus, _ := deriveRuntimeHealthStatus(rt, liveFallback, now)
	if fallbackStatus != "online" {
		t.Fatalf("db-fallback for 80s-old heartbeat resolved %q, want \"online\"", fallbackStatus)
	}
}

func ptrInt64(v int64) *int64 {
	return &v
}
