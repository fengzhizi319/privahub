package service

import (
	"testing"
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
	if h1, h2 := HashPassword("test"), HashPassword("test"); h1 != h2 {
		t.Error("HashPassword should be deterministic")
	}

	// Verify different inputs produce different hashes
	if HashPassword("a") == HashPassword("b") {
		t.Error("HashPassword should produce different hashes for different inputs")
	}
}

func TestVerifyPassword(t *testing.T) {
	// Plaintext passwords are only hashed server side in dev profiles.
	svc := &AuthService{opts: AuthOptions{AllowPlaintextPassword: true}}

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
