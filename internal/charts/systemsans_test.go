package charts

import "testing"

func TestSystemSansPadsItsWidths(t *testing.T) {
	plain, padded := SystemSans(0), SystemSans(0.1)
	a, b := plain.Width("Reference 200 W", 12, 400), padded.Width("Reference 200 W", 12, 400)
	if a < 80 || a > 100 {
		t.Errorf("unpadded width %.1f is not a plausible system-sans width", a)
	}
	if d := b/a - 1.1; d > 0.001 || d < -0.001 {
		t.Errorf("padding: got ratio %.4f, want 1.1", b/a)
	}
}

// A rune outside the table falls back to the width of 'n', not zero, so a
// label in another script still gets room.
func TestSystemSansFallsBackOutsideItsTable(t *testing.T) {
	m := SystemSans(0)
	if m.Width("日本", 12, 400) != 2*m.Width("n", 12, 400) {
		t.Error("runes outside the table did not fall back to the width of n")
	}
}
