package worker

import "testing"

func TestVectorLiteral(t *testing.T) {
	got := vectorLiteral([]float32{0.5, -1, 0.25})
	want := "[0.5,-1,0.25]"
	if got != want {
		t.Errorf("vectorLiteral = %q, want %q", got, want)
	}
	if got := vectorLiteral(nil); got != "[]" {
		t.Errorf("vectorLiteral(nil) = %q, want []", got)
	}
}
