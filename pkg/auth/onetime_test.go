package auth

import (
	"strings"
	"testing"
)

func TestGenerateOneTimePassword(t *testing.T) {
	if _, err := GenerateOneTimePassword(8); err == nil {
		t.Fatal("short length must be refused")
	}
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		p, err := GenerateOneTimePassword(20)
		if err != nil || len(p) != 20 {
			t.Fatalf("got %q %v", p, err)
		}
		for _, r := range p {
			if !strings.ContainsRune(oneTimeAlphabet, r) {
				t.Fatalf("unexpected rune %q", r)
			}
		}
		if seen[p] {
			t.Fatal("duplicate password")
		}
		seen[p] = true
	}
}
