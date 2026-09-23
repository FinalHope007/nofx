package trader

import "testing"

func TestShouldSkipForNoCandidates(t *testing.T) {
	cases := []struct {
		candidates, positions int
		want                  bool
	}{
		{0, 0, true},
		{0, 1, false},
		{3, 0, false},
		{3, 2, false},
	}
	for _, c := range cases {
		if got := shouldSkipForNoCandidates(c.candidates, c.positions); got != c.want {
			t.Fatalf("candidates=%d positions=%d: got %v want %v", c.candidates, c.positions, got, c.want)
		}
	}
}
