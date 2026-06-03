package handler

import (
	"net/http"
	"os"
	"strings"

	"github.com/multica-ai/multica/server/internal/analytics"
)

type AppConfig struct {
	CdnDomain string `json:"cdn_domain"`
	// Public auth config consumed by the web app at runtime so self-hosted
	// deployments do not need to rebuild the frontend image when operators
	// toggle signup or wire Google OAuth.
	AllowSignup    bool   `json:"allow_signup"`
	GoogleClientID string `json:"google_client_id,omitempty"`

	// DevAuthEnabled tells the web login screen whether a deterministic
	// local-development verification code is active, so it can show operators
	// a "local dev sign-in is enabled" hint. It is intentionally only a
	// boolean: the code itself is NEVER exposed here (this endpoint is
	// anonymous and public). It is true only on non-production instances that
	// have configured a valid six-digit MULTICA_DEV_VERIFICATION_CODE — the
	// exact same gate isDevVerificationCode applies before accepting the code,
	// so this flag can never advertise a path the backend would reject.
	DevAuthEnabled bool `json:"dev_auth_enabled"`

	// PostHog public config for the frontend. The key is the same Project
	// API Key the backend uses; returning it here (instead of baking it
	// into the frontend bundle via NEXT_PUBLIC_*) means self-hosted
	// instances — whose server returns an empty key — automatically
	// disable frontend event shipping too.
	PosthogKey           string `json:"posthog_key"`
	PosthogHost          string `json:"posthog_host"`
	AnalyticsEnvironment string `json:"analytics_environment"`
}

// GetConfig is mounted on the public (unauthenticated) route group because
// the web app calls it before login to decide whether to render the Google
// sign-in button and signup UI. Only add fields here that are safe to expose
// to anonymous callers — never user- or tenant-scoped data.
func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	config := AppConfig{
		AllowSignup:    os.Getenv("ALLOW_SIGNUP") != "false",
		GoogleClientID: os.Getenv("GOOGLE_CLIENT_ID"),
		DevAuthEnabled: devAuthEnabled(),
	}
	if h.Storage != nil {
		config.CdnDomain = h.Storage.CdnDomain()
	}

	// Re-read from env on every request so operators can rotate keys via
	// secret refresh without a server restart.
	if v := os.Getenv("ANALYTICS_DISABLED"); v != "true" && v != "1" {
		config.PosthogKey = os.Getenv("POSTHOG_API_KEY")
		config.PosthogHost = os.Getenv("POSTHOG_HOST")
		config.AnalyticsEnvironment = analytics.EnvironmentFromEnv()
		if config.PosthogHost == "" && config.PosthogKey != "" {
			config.PosthogHost = "https://us.i.posthog.com"
		}
	}

	writeJSON(w, http.StatusOK, config)
}

// devAuthEnabled reports whether the deterministic local-development sign-in
// path is active. It mirrors the exact preconditions isDevVerificationCode
// enforces before accepting the dev code: non-production environment plus a
// validly-formatted six-digit MULTICA_DEV_VERIFICATION_CODE. Surfacing it as a
// boolean lets the public /api/config endpoint hint at dev sign-in without ever
// leaking the code value itself.
func devAuthEnabled() bool {
	if isProductionEnv() {
		return false
	}
	return isSixDigitCode(strings.TrimSpace(os.Getenv(devVerificationCodeEnv)))
}
