package buffer

import (
	"math"
	"testing"
)

func TestRingBuffer_PushSingleValue(t *testing.T) {
	rb := New(30)
	rb.Push(42.0)

	values := rb.Values()
	if len(values) != 1 {
		t.Fatalf("expected 1 value, got %d", len(values))
	}
	if values[0] != 42.0 {
		t.Errorf("expected 42.0, got %f", values[0])
	}
}

func TestRingBuffer_PushFillsToCapacity(t *testing.T) {
	rb := New(30)

	for i := 0; i < 30; i++ {
		rb.Push(float64(i))
	}

	values := rb.Values()
	if len(values) != 30 {
		t.Fatalf("expected 30 values, got %d", len(values))
	}

	// Verify all values in order
	for i := 0; i < 30; i++ {
		if values[i] != float64(i) {
			t.Errorf("at index %d: expected %f, got %f", i, float64(i), values[i])
		}
	}
}

func TestRingBuffer_PushOverflow(t *testing.T) {
	rb := New(30)

	// Push 31 values (0 through 30)
	for i := 0; i <= 30; i++ {
		rb.Push(float64(i))
	}

	values := rb.Values()
	if len(values) != 30 {
		t.Fatalf("expected 30 values, got %d", len(values))
	}

	// First value should be 1 (0 was dropped)
	if values[0] != 1.0 {
		t.Errorf("expected first value 1.0, got %f", values[0])
	}

	// Last value should be 30
	if values[29] != 30.0 {
		t.Errorf("expected last value 30.0, got %f", values[29])
	}
}

func TestRingBuffer_ValuesChronological(t *testing.T) {
	rb := New(30)

	// Push 35 values (0 through 34)
	for i := 0; i < 35; i++ {
		rb.Push(float64(i))
	}

	values := rb.Values()
	if len(values) != 30 {
		t.Fatalf("expected 30 values, got %d", len(values))
	}

	// Should contain values 5-34 in chronological order
	for i := 0; i < 30; i++ {
		expected := float64(i + 5)
		if values[i] != expected {
			t.Errorf("at index %d: expected %f, got %f", i, expected, values[i])
		}
	}
}

func TestRingBuffer_EmptyReturnsNil(t *testing.T) {
	rb := New(30)

	values := rb.Values()
	if values != nil {
		t.Errorf("expected nil for empty buffer, got %v", values)
	}
}

func TestRingBuffer_PushNaN(t *testing.T) {
	rb := New(30)
	rb.Push(math.NaN())

	values := rb.Values()
	if len(values) != 1 {
		t.Fatalf("expected 1 value, got %d", len(values))
	}

	if !math.IsNaN(values[0]) {
		t.Errorf("expected NaN, got %f", values[0])
	}
}

func TestRingBuffer_PushInf(t *testing.T) {
	rb := New(30)
	rb.Push(math.Inf(1))  // +Inf
	rb.Push(math.Inf(-1)) // -Inf

	values := rb.Values()
	if len(values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(values))
	}

	if !math.IsInf(values[0], 1) {
		t.Errorf("expected +Inf, got %f", values[0])
	}
	if !math.IsInf(values[1], -1) {
		t.Errorf("expected -Inf, got %f", values[1])
	}
}

func TestRingBuffer_Len(t *testing.T) {
	rb := New(30)

	if rb.Len() != 0 {
		t.Errorf("expected len 0, got %d", rb.Len())
	}

	rb.Push(1.0)
	if rb.Len() != 1 {
		t.Errorf("expected len 1, got %d", rb.Len())
	}

	// Fill to capacity
	for i := 1; i < 30; i++ {
		rb.Push(float64(i))
	}
	if rb.Len() != 30 {
		t.Errorf("expected len 30, got %d", rb.Len())
	}

	// Overflow should still be 30
	rb.Push(100.0)
	if rb.Len() != 30 {
		t.Errorf("after overflow: expected len 30, got %d", rb.Len())
	}
}

func TestRingBuffer_Latest(t *testing.T) {
	rb := New(30)

	// Empty buffer
	_, ok := rb.Latest()
	if ok {
		t.Error("expected ok=false for empty buffer")
	}

	rb.Push(42.0)
	val, ok := rb.Latest()
	if !ok {
		t.Error("expected ok=true after push")
	}
	if val != 42.0 {
		t.Errorf("expected 42.0, got %f", val)
	}

	rb.Push(99.0)
	val, ok = rb.Latest()
	if !ok {
		t.Error("expected ok=true")
	}
	if val != 99.0 {
		t.Errorf("expected 99.0, got %f", val)
	}
}
