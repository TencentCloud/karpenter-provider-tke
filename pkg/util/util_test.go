package util

import (
	"os"
	"testing"
)

func TestPrettySlice_Empty(t *testing.T) {
	result := PrettySlice([]string{}, 3)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestPrettySlice_LessThanMax(t *testing.T) {
	result := PrettySlice([]string{"a", "b"}, 3)
	expected := "a, b"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestPrettySlice_EqualToMax(t *testing.T) {
	result := PrettySlice([]string{"a", "b", "c"}, 3)
	expected := "a, b, c"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestPrettySlice_ExceedsMax(t *testing.T) {
	result := PrettySlice([]string{"a", "b", "c", "d", "e"}, 3)
	expected := "a, b, c and 2 other(s)"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestPrettySlice_SingleElement(t *testing.T) {
	result := PrettySlice([]string{"only"}, 3)
	expected := "only"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestPrettySlice_IntSlice(t *testing.T) {
	result := PrettySlice([]int{1, 2, 3, 4}, 2)
	expected := "1, 2 and 2 other(s)"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestWithDefaultFloat64_EnvNotSet(t *testing.T) {
	os.Unsetenv("TEST_FLOAT_KEY")
	result := WithDefaultFloat64("TEST_FLOAT_KEY", 1.5)
	if result != 1.5 {
		t.Errorf("expected 1.5, got %f", result)
	}
}

func TestWithDefaultFloat64_ValidValue(t *testing.T) {
	os.Setenv("TEST_FLOAT_KEY", "3.14")
	defer os.Unsetenv("TEST_FLOAT_KEY")
	result := WithDefaultFloat64("TEST_FLOAT_KEY", 1.5)
	if result != 3.14 {
		t.Errorf("expected 3.14, got %f", result)
	}
}

func TestWithDefaultFloat64_InvalidValue(t *testing.T) {
	os.Setenv("TEST_FLOAT_KEY", "not-a-number")
	defer os.Unsetenv("TEST_FLOAT_KEY")
	result := WithDefaultFloat64("TEST_FLOAT_KEY", 1.5)
	if result != 1.5 {
		t.Errorf("expected 1.5 for invalid env value, got %f", result)
	}
}
