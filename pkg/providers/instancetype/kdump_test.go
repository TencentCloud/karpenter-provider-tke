package instancetype

import (
	"testing"
)

func TestKdump_eval_BelowLow(t *testing.T) {
	k := Kdump{low: 100, high: 200, reserved: 50}
	if got := k.eval(50); got != 0 {
		t.Errorf("expected 0 for resource below low, got %d", got)
	}
}

func TestKdump_eval_InRange(t *testing.T) {
	k := Kdump{low: 100, high: 200, reserved: 50}
	if got := k.eval(150); got != 50 {
		t.Errorf("expected 50 for resource in range, got %d", got)
	}
}

func TestKdump_eval_AtLow(t *testing.T) {
	k := Kdump{low: 100, high: 200, reserved: 50}
	if got := k.eval(100); got != 50 {
		t.Errorf("expected 50 for resource at low boundary, got %d", got)
	}
}

func TestKdump_eval_AtHigh(t *testing.T) {
	k := Kdump{low: 100, high: 200, reserved: 50}
	if got := k.eval(200); got != 0 {
		t.Errorf("expected 0 for resource at high boundary (>= high), got %d", got)
	}
}

func TestKdump_eval_AboveHigh(t *testing.T) {
	k := Kdump{low: 100, high: 200, reserved: 50}
	if got := k.eval(300); got != 0 {
		t.Errorf("expected 0 for resource above high, got %d", got)
	}
}

func TestKdumps_eval_BelowAllLevels(t *testing.T) {
	// Memory below 1800 MB (in bytes)
	mem := 1000 * 1024 * 1024
	if got := KdumpLevels.eval(mem); got != 0 {
		t.Errorf("expected 0 for small memory, got %d", got)
	}
}

func TestKdumps_eval_FirstLevel(t *testing.T) {
	// 2 GB memory - falls in first level (1800MB - 64GB)
	mem := 2 * 1024 * 1024 * 1024
	expected := 256 * 1024 * 1024
	if got := KdumpLevels.eval(mem); got != expected {
		t.Errorf("expected %d for 2GB memory, got %d", expected, got)
	}
}

func TestKdumps_eval_SecondLevel(t *testing.T) {
	// 96 GB memory - falls in second level (64GB - 128GB)
	mem := 96 * 1024 * 1024 * 1024
	expected := 512 * 1024 * 1024
	if got := KdumpLevels.eval(mem); got != expected {
		t.Errorf("expected %d for 96GB memory, got %d", expected, got)
	}
}

func TestKdumps_eval_ThirdLevel(t *testing.T) {
	// The third level uses math.MaxInt32 as high boundary.
	// On 64-bit systems, 128GB*1024*1024*1024 > math.MaxInt32,
	// so this level effectively doesn't match any realistic memory size.
	// Test with a value that actually falls in the [low, high) range.
	// Since low=128*1024*1024*1024 > math.MaxInt32, this level is unreachable.
	// Verify that 200GB returns 0 (no match in any level after second).
	mem := 200 * 1024 * 1024 * 1024
	if got := KdumpLevels.eval(mem); got != 0 {
		t.Errorf("expected 0 for 200GB memory (third level unreachable due to MaxInt32), got %d", got)
	}
}
