package kernel

import "testing"

func TestKeepCoinByOILiquidity(t *testing.T) {
	// enable=true, threshold 15M, low OI -> drop (false)
	if keepCoinByOILiquidity(false, false, 1_000_000, true, 15_000_000) {
		t.Fatal("expected low-OI coin dropped when filter enabled")
	}
	// enable=false -> keep regardless of OI
	if !keepCoinByOILiquidity(false, false, 1_000_000, false, 15_000_000) {
		t.Fatal("expected low-OI coin kept when filter disabled")
	}
	// threshold lowered to 1M, OI == 1M -> keep (equal passes)
	if !keepCoinByOILiquidity(false, false, 1_000_000, true, 1_000_000) {
		t.Fatal("expected OI equal to threshold to pass")
	}
	// existing position or XYZ asset always kept
	if !keepCoinByOILiquidity(true, false, 1_000_000, true, 15_000_000) {
		t.Fatal("expected existing position always kept")
	}
	if !keepCoinByOILiquidity(false, true, 1_000_000, true, 15_000_000) {
		t.Fatal("expected XYZ asset always kept")
	}
}
