package config

import "testing"

func TestRateLimitAndAPIDocsDefaults(t *testing.T) {
	t.Setenv("PRIVAHUB_PROFILE", "")
	t.Setenv("PRIVAHUB_AUTH_JWT_SECRET", "a-very-long-random-secret-value-123456")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	rl := cfg.Server.RateLimit
	if rl.UserPermits != 5 || rl.UserWindow.Seconds() != 60 || rl.UserBurst != 1 || rl.LoginPerMinute != 20 {
		t.Fatalf("unexpected rate limit defaults: %+v", rl)
	}
	want := map[string]bool{
		"/api/v1alpha1/inst/node/newToken": true, "/api/v1alpha1/datasource/create": true,
		"/api/v1alpha1/datatable/create": true, "/api/v1alpha1/feature_datasource/create": true,
	}
	for _, p := range rl.UserPaths {
		delete(want, p)
	}
	if len(want) != 0 {
		t.Fatalf("missing default user paths: %v", want)
	}
	if cfg.Server.APIDocs.Enabled {
		t.Fatal("api docs must be disabled by default outside dev profiles")
	}

	t.Setenv("PRIVAHUB_PROFILE", "test")
	cfg, err = Load("")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Server.APIDocs.Enabled || cfg.Server.APIDocs.Public {
		t.Fatalf("test profile: docs enabled behind auth, got %+v", cfg.Server.APIDocs)
	}

	t.Setenv("PRIVAHUB_SERVER_API_DOCS_ENABLED", "false")
	cfg, err = Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.APIDocs.Enabled {
		t.Fatal("explicit api_docs.enabled=false must win over the profile default")
	}
}
