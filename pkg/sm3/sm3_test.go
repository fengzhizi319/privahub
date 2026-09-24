package sm3

import (
	"strings"
	"testing"
)

func TestSM3StandardVectors(t *testing.T) {
	if got := SumHex([]byte("abc")); got != "66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a2297da02b8f4ba8e0" {
		t.Fatalf("sm3(abc)=%s", got)
	}
	if got := SumHex([]byte(strings.Repeat("abcd", 16))); got != "debe9ff92275b8a138604889c18e5a4d6fdb70e5387e5765293dcba39c0c5732" {
		t.Fatalf("sm3(abcd*16)=%s", got)
	}
}
