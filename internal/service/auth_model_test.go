package service

import (
	"testing"

	"github.com/fengzhizi319/privahub/pkg/kuscia"
)

func TestHashPassword(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"known password",
			"12345678",
			"ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f",
		},
		{
			"empty password",
			"",
			"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HashPassword(tt.input)
			if result != tt.expected {
				t.Errorf("HashPassword(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}

	// Verify determinism
	if HashPassword("test") != HashPassword("test") {
		t.Error("HashPassword should be deterministic")
	}

	// Verify different inputs produce different hashes
	if HashPassword("a") == HashPassword("b") {
		t.Error("HashPassword should produce different hashes for different inputs")
	}
}

func TestVerifyPassword(t *testing.T) {
	svc := &AuthService{}

	tests := []struct {
		name           string
		plainPassword  string
		hashedPassword string
		expected       bool
	}{
		{
			"direct hash match (frontend sends pre-hashed)",
			HashPassword("12345678"),
			HashPassword("12345678"),
			true,
		},
		{
			"raw password match (hash-then-compare path)",
			"12345678",
			HashPassword("12345678"),
			true,
		},
		{
			"wrong password",
			"wrongpassword",
			HashPassword("12345678"),
			false,
		},
		{
			"empty password vs non-empty hash",
			"",
			HashPassword("12345678"),
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.verifyPassword(tt.plainPassword, tt.hashedPassword)
			if result != tt.expected {
				t.Errorf("verifyPassword(%q, %q) = %v, want %v",
					tt.plainPassword, tt.hashedPassword, result, tt.expected)
			}
		})
	}
}

func TestBuildServingParties(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"empty string", "", 0},
		{"invalid json falls back to csv", "not json", 1},
		{
			"valid json array",
			`[{"domain_id":"alice","role":"guest","app_image":"sf"},{"domain_id":"bob","role":"host","app_image":"sf"}]`,
			2,
		},
		{
			"comma separated fallback",
			"alice,bob,charlie",
			3,
		},
		{
			"comma separated with spaces",
			" alice , bob ",
			2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildServingParties(tt.input)
			if len(result) != tt.expected {
				t.Errorf("buildServingParties(%q) len = %d, want %d", tt.input, len(result), tt.expected)
			}
		})
	}

	// Verify comma-separated fallback produces correct domain IDs
	parties := buildServingParties("alice,bob")
	if len(parties) == 2 {
		if parties[0].DomainID != "alice" || parties[1].DomainID != "bob" {
			t.Errorf("buildServingParties fallback domain IDs incorrect: %+v", parties)
		}
		if parties[0].Role != "guest" {
			t.Errorf("buildServingParties fallback role = %q, want 'guest'", parties[0].Role)
		}
	}

	// Verify JSON parsing preserves fields
	jsonParties := buildServingParties(`[{"domain_id":"node1","role":"host","app_image":"custom"}]`)
	if len(jsonParties) == 1 {
		expected := kuscia.ServingParty{DomainID: "node1", Role: "host", AppImage: "custom"}
		if jsonParties[0] != expected {
			t.Errorf("buildServingParties JSON = %+v, want %+v", jsonParties[0], expected)
		}
	}
}
