package config

import (
	"strings"
	"testing"
)

func TestValidateNodeSecurity(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{"edge https gateway", func(c *Config) { c.Server.Mode = "lite"; c.Kuscia.Gateway = "https://gw:1080" }, ""},
		{"edge loopback http gateway", func(c *Config) { c.Server.Mode = "EDGE"; c.Kuscia.Gateway = "127.0.0.1:80" }, ""},
		{"edge plain remote gateway", func(c *Config) { c.Server.Mode = "lite"; c.Kuscia.Gateway = "http://10.0.0.5:80" }, "https://"},
		{"edge bare remote gateway", func(c *Config) { c.Server.Mode = "lite"; c.Kuscia.Gateway = "gw.example:80" }, "https://"},
		{"edge plain gateway in dev", func(c *Config) {
			c.Profile = "dev"
			c.Server.Mode = "lite"
			c.Kuscia.Gateway = "gw.example:80"
		}, ""},
		{"center plain gateway", func(c *Config) { c.Kuscia.Gateway = "http://10.0.0.5:80" }, ""},
		{"bad trusted proxy", func(c *Config) { c.Server.TrustedProxies = []string{"not-an-ip"} }, "trusted_proxies"},
		{"cidr trusted proxy", func(c *Config) { c.Server.TrustedProxies = []string{"10.0.0.0/8", "::1"} }, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := baseConfig("")
			tc.mutate(c)
			err := c.Validate()
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}
