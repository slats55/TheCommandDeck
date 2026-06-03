package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetConfigIncludesRuntimeAuthConfig(t *testing.T) {
	origStorage := testHandler.Storage
	testHandler.Storage = &mockStorage{}
	defer func() { testHandler.Storage = origStorage }()

	t.Setenv("ALLOW_SIGNUP", "false")
	t.Setenv("GOOGLE_CLIENT_ID", "google-client-id")
	t.Setenv("POSTHOG_API_KEY", "phc_test")
	t.Setenv("POSTHOG_HOST", "https://eu.i.posthog.com")

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()

	testHandler.GetConfig(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GetConfig: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var cfg AppConfig
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}

	if cfg.CdnDomain != "cdn.example.com" {
		t.Fatalf("cdn_domain: want cdn.example.com, got %q", cfg.CdnDomain)
	}
	if cfg.AllowSignup {
		t.Fatalf("allow_signup: want false, got true")
	}
	if cfg.GoogleClientID != "google-client-id" {
		t.Fatalf("google_client_id: want google-client-id, got %q", cfg.GoogleClientID)
	}
	if cfg.PosthogKey != "phc_test" {
		t.Fatalf("posthog_key: want phc_test, got %q", cfg.PosthogKey)
	}
	if cfg.PosthogHost != "https://eu.i.posthog.com" {
		t.Fatalf("posthog_host: want https://eu.i.posthog.com, got %q", cfg.PosthogHost)
	}
	if cfg.AnalyticsEnvironment != "dev" {
		t.Fatalf("analytics_environment: want dev, got %q", cfg.AnalyticsEnvironment)
	}
}

// getConfigResult runs GetConfig and returns both the decoded struct and the
// raw JSON body, so a test can assert the dev code value never appears on the
// wire regardless of how the struct is shaped.
func getConfigResult(t *testing.T) (AppConfig, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()
	testHandler.GetConfig(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GetConfig: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var cfg AppConfig
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	return cfg, w.Body.String()
}

func TestGetConfigDevAuthEnabledOnlyInNonProductionWithValidCode(t *testing.T) {
	const devCode = "424242"

	t.Run("non-production with valid six-digit code enables dev auth", func(t *testing.T) {
		t.Setenv("APP_ENV", "development")
		t.Setenv(devVerificationCodeEnv, devCode)

		cfg, body := getConfigResult(t)
		if !cfg.DevAuthEnabled {
			t.Fatalf("dev_auth_enabled: want true in development with valid code, got false")
		}
		// The flag must never leak the actual code onto the public endpoint.
		if strings.Contains(body, devCode) {
			t.Fatalf("config body leaked the dev verification code: %s", body)
		}
	})

	t.Run("production never enables dev auth even with a code set", func(t *testing.T) {
		t.Setenv("APP_ENV", "production")
		t.Setenv(devVerificationCodeEnv, devCode)

		cfg, body := getConfigResult(t)
		if cfg.DevAuthEnabled {
			t.Fatalf("dev_auth_enabled: want false in production, got true")
		}
		if strings.Contains(body, devCode) {
			t.Fatalf("config body leaked the dev verification code: %s", body)
		}
	})

	t.Run("non-production without a code keeps dev auth disabled", func(t *testing.T) {
		t.Setenv("APP_ENV", "development")
		t.Setenv(devVerificationCodeEnv, "")

		cfg, _ := getConfigResult(t)
		if cfg.DevAuthEnabled {
			t.Fatalf("dev_auth_enabled: want false with no code, got true")
		}
	})

	t.Run("non-production with malformed code keeps dev auth disabled", func(t *testing.T) {
		t.Setenv("APP_ENV", "development")
		t.Setenv(devVerificationCodeEnv, "abc123")

		cfg, _ := getConfigResult(t)
		if cfg.DevAuthEnabled {
			t.Fatalf("dev_auth_enabled: want false with malformed code, got true")
		}
	})
}
