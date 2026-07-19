package database

import "testing"

func TestVectorLiteral(t *testing.T) {
	got := VectorLiteral([]float32{0.5, -1, 0.25})
	if want := "[0.5,-1,0.25]"; got != want {
		t.Errorf("VectorLiteral = %q, want %q", got, want)
	}
	if got := VectorLiteral(nil); got != "[]" {
		t.Errorf("VectorLiteral(nil) = %q, want []", got)
	}
}
