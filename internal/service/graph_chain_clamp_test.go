package service

import (
	"math"
	"testing"
)

func TestClampInt32(t *testing.T) {
	cases := map[int]int32{
		0:                 0,
		5:                 5,
		-3:                -3,
		math.MaxInt32:     math.MaxInt32,
		math.MaxInt32 + 1: math.MaxInt32,
		math.MinInt32 - 1: math.MinInt32,
		math.MaxInt:       math.MaxInt32,
	}
	for in, want := range cases {
		if got := clampInt32(in); got != want {
			t.Errorf("clampInt32(%d)=%d want %d", in, got, want)
		}
	}
}
