// Copyright 2026 Privahub Authors. Licensed under the Apache License, Version 2.0.
package config

import (
	"strings"
	"testing"
)

func TestValidateRejectsOwnershipBypassWhitelist(t *testing.T) {
	cfg := Config{Auth: AuthConfig{DataResourceWhitelist: []string{"/api/v1alpha1/graph/detail"}}}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "auth.data_resource_whitelist") {
		t.Fatalf("must explicitly reject legacy ownership bypass configuration, got %v", err)
	}
}
