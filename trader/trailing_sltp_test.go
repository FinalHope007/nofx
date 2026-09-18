package trader

import "testing"

func TestIsStopTightening(t *testing.T) {
	cases := []struct {
		name    string
		side    string
		next    float64
		current float64
		want    bool
	}{
		{"long raises stop", "long", 0.80, 0.75, true},
		{"long lowers stop", "long", 0.70, 0.75, false},
		{"long same stop", "long", 0.75, 0.75, false},
		{"long replaces unset stop", "long", 0.75, 0, true},
		{"short lowers stop", "short", 0.70, 0.75, true},
		{"short raises stop", "short", 0.80, 0.75, false},
		{"short same stop", "short", 0.75, 0.75, false},
		{"short replaces unset stop", "short", 0.75, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isStopTightening(c.side, c.next, c.current); got != c.want {
				t.Fatalf("isStopTightening(%q, %v, %v) = %v, want %v", c.side, c.next, c.current, got, c.want)
			}
		})
	}
}
