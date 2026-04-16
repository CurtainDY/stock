package data

import "testing"

func TestBarAdjClose(t *testing.T) {
	// Tonghuashun additive format: adj = close + factor
	b := Bar{Close: 10.0, AdjFactor: -1.0}
	want := 9.0
	got := b.AdjClose()
	if got != want {
		t.Errorf("AdjClose() = %v, want %v", got, want)
	}
}

func TestBarAdjCloseNoFactor(t *testing.T) {
	// AdjFactor=0 means no data, return Close unchanged
	b := Bar{Close: 10.0, AdjFactor: 0}
	want := 10.0
	got := b.AdjClose()
	if got != want {
		t.Errorf("AdjClose() zero factor = %v, want %v", got, want)
	}
}
