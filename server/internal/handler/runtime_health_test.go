package handler

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func TestDeriveRuntimeHealthStatus(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		rt         db.AgentRuntime
		wantStatus string
		wantAgeSec *int64
	}{
		{
			name:       "unknown when last seen missing",
			rt:         db.AgentRuntime{Status: "online"},
			wantStatus: "unknown",
			wantAgeSec: nil,
		},
		{
			name:       "online when heartbeat is fresh",
			rt:         db.AgentRuntime{Status: "online", LastSeenAt: ts(now.Add(-10 * time.Second))},
			wantStatus: "online",
			wantAgeSec: ptrInt64(10),
		},
		{
			name:       "stale when online runtime heartbeat is old",
			rt:         db.AgentRuntime{Status: "online", LastSeenAt: ts(now.Add(-2 * time.Minute))},
			wantStatus: "stale",
			wantAgeSec: ptrInt64(120),
		},
		{
			name:       "stale when offline runtime just dropped",
			rt:         db.AgentRuntime{Status: "offline", LastSeenAt: ts(now.Add(-2 * time.Minute))},
			wantStatus: "stale",
			wantAgeSec: ptrInt64(120),
		},
		{
			name:       "offline when runtime has been gone for a while",
			rt:         db.AgentRuntime{Status: "offline", LastSeenAt: ts(now.Add(-10 * time.Minute))},
			wantStatus: "offline",
			wantAgeSec: ptrInt64(600),
		},
		{
			name:       "future timestamp clamps age to zero",
			rt:         db.AgentRuntime{Status: "online", LastSeenAt: ts(now.Add(10 * time.Second))},
			wantStatus: "online",
			wantAgeSec: ptrInt64(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotAge := deriveRuntimeHealthStatus(tt.rt, now)
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

func ptrInt64(v int64) *int64 {
	return &v
}
