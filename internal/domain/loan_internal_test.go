package domain

import (
	"math/big"
	"testing"
)

func TestRoundRatCents(t *testing.T) {
	cases := map[string]int64{
		"0":       0,
		"12.4":    12,
		"12.5":    13,
		"12.4999": 12,
		"-12.5":   -13,
		"-12.4":   -12,
	}
	for input, want := range cases {
		value, ok := new(big.Rat).SetString(input)
		if !ok {
			t.Fatalf("parse %s", input)
		}
		if got := roundRatCents(value); got != want {
			t.Fatalf("%s: got %d, want %d", input, got, want)
		}
	}
}
