package config

import "testing"

func TestValidateAuditRetention(t *testing.T) {
	cases := []struct {
		profile string
		days    int
		archive string
		ok      bool
	}{
		{"", 0, "", true},
		{"", 1095, "/a", true},
		{"", 365, "/a", false},
		{"", -1, "/a", false},
		{"dev", 7, "/a", true},
		{"", 2000, "", false},
	}
	for _, tc := range cases {
		c := &Config{Profile: tc.profile, Audit: AuditConfig{RetentionDays: tc.days, ArchiveDir: tc.archive}}
		if err := c.validateAudit(); (err == nil) != tc.ok {
			t.Errorf("profile=%q days=%d archive=%q: err=%v", tc.profile, tc.days, tc.archive, err)
		}
	}
	prod := &Config{}
	if len(prod.AuditWarnings()) == 0 {
		t.Error("expected anchoring warning in production")
	}
	prod.Audit.Anchor.File = "/x"
	if len(prod.AuditWarnings()) != 0 {
		t.Error("unexpected warning with anchoring")
	}
}
