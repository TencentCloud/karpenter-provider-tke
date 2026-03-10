package instancetype

import (
	"testing"
)

func TestLevel_eval_BelowLow(t *testing.T) {
	l := Level{low: 100, high: 200, base: 10, fact: 0.5}
	if got := l.eval(50); got != 0.0 {
		t.Errorf("expected 0 for resource below low, got %f", got)
	}
}

func TestLevel_eval_InRange(t *testing.T) {
	l := Level{low: 100, high: 200, base: 10, fact: 0.5}
	got := l.eval(150)
	// (150-100)*0.5 + 10 = 35
	expected := 35.0
	if got != expected {
		t.Errorf("expected %f, got %f", expected, got)
	}
}

func TestLevel_eval_AtLow(t *testing.T) {
	l := Level{low: 100, high: 200, base: 10, fact: 0.5}
	got := l.eval(100)
	// (100-100)*0.5 + 10 = 10
	expected := 10.0
	if got != expected {
		t.Errorf("expected %f, got %f", expected, got)
	}
}

func TestLevel_eval_AboveHigh(t *testing.T) {
	l := Level{low: 100, high: 200, base: 10, fact: 0.5}
	got := l.eval(300)
	// (200-100)*0.5 + 10 = 60
	expected := 60.0
	if got != expected {
		t.Errorf("expected %f, got %f", expected, got)
	}
}

func TestLevel_eval_AtHigh(t *testing.T) {
	l := Level{low: 100, high: 200, base: 10, fact: 0.5}
	got := l.eval(200)
	// resource >= high: (200-100)*0.5 + 10 = 60
	expected := 60.0
	if got != expected {
		t.Errorf("expected %f, got %f", expected, got)
	}
}

func TestLevels_eval_MultiLevel(t *testing.T) {
	ls := Levels{
		{low: 0, high: 100, base: 0, fact: 1.0},
		{low: 100, high: 200, base: 0, fact: 0.5},
	}
	// For resource 150:
	// Level 1: resource >= high -> (100-0)*1.0 + 0 = 100
	// Level 2: resource in range -> (150-100)*0.5 + 0 = 25
	// Total: 125
	got := ls.eval(150)
	if got != 125 {
		t.Errorf("expected 125, got %d", got)
	}
}

func TestLevels_eval_BelowAll(t *testing.T) {
	ls := Levels{
		{low: 100, high: 200, base: 0, fact: 1.0},
		{low: 200, high: 300, base: 0, fact: 0.5},
	}
	got := ls.eval(50)
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestLevels_eval_KubeReservedCPU(t *testing.T) {
	// Test with the actual kube reserved CPU levels (k8s > 1.29)
	cpuLevel := Levels{
		{low: 0, high: 1000, base: 60},
		{low: 1000, high: 2000, fact: 0.01},
		{low: 2000, high: 4000, fact: 0.005},
		{low: 4000, high: 1 << 31, fact: 0.0025},
	}
	// For 4000 milliCPU:
	// L1: resource >= high -> (1000-0)*0 + 60 = 60
	// L2: resource >= high -> (2000-1000)*0.01 + 0 = 10
	// L3: resource >= high -> (4000-2000)*0.005 + 0 = 10
	// L4: resource in range -> (4000-4000)*0.0025 + 0 = 0
	// Total: 80
	got := cpuLevel.eval(4000)
	if got != 80 {
		t.Errorf("expected 80, got %d", got)
	}
}
