package webview

import (
	"errors"
	"testing"
)

// TestMakeFuncWrapper exercises the binding wrapper directly, without the native
// library or an event loop. It is a regression guard for the nil-error panic:
// a bound function declaring an error return must succeed on the nil-error path
// instead of panicking on a single-value `.(error)` assertion.
func TestMakeFuncWrapper(t *testing.T) {
	t.Run("value only", func(t *testing.T) {
		fn, err := makeFuncWrapper(func(a, b int) int { return a + b })
		if err != nil {
			t.Fatalf("makeFuncWrapper: %v", err)
		}
		v, err := fn("1", "[2,3]")
		if err != nil {
			t.Fatalf("call: unexpected error: %v", err)
		}
		if v != 5 {
			t.Fatalf("call: got %v, want 5", v)
		}
	})

	t.Run("value and nil error", func(t *testing.T) {
		fn, err := makeFuncWrapper(func(a, b int) (int, error) { return a + b, nil })
		if err != nil {
			t.Fatalf("makeFuncWrapper: %v", err)
		}
		v, err := fn("1", "[2,3]")
		if err != nil {
			t.Fatalf("call: unexpected error: %v", err)
		}
		if v != 5 {
			t.Fatalf("call: got %v, want 5", v)
		}
	})

	t.Run("just nil error", func(t *testing.T) {
		fn, err := makeFuncWrapper(func() error { return nil })
		if err != nil {
			t.Fatalf("makeFuncWrapper: %v", err)
		}
		v, err := fn("1", "[]")
		if err != nil {
			t.Fatalf("call: unexpected error: %v", err)
		}
		if v != nil {
			t.Fatalf("call: got %v, want nil", v)
		}
	})

	t.Run("real error is propagated", func(t *testing.T) {
		sentinel := errors.New("boom")
		fn, err := makeFuncWrapper(func() (int, error) { return 0, sentinel })
		if err != nil {
			t.Fatalf("makeFuncWrapper: %v", err)
		}
		if _, err := fn("1", "[]"); !errors.Is(err, sentinel) {
			t.Fatalf("call: got %v, want sentinel", err)
		}
	})

	t.Run("non-func is rejected", func(t *testing.T) {
		if _, err := makeFuncWrapper(42); err == nil {
			t.Fatal("expected error binding a non-function")
		}
	})

	t.Run("argument count mismatch", func(t *testing.T) {
		fn, err := makeFuncWrapper(func(a, b int) int { return a + b })
		if err != nil {
			t.Fatalf("makeFuncWrapper: %v", err)
		}
		if _, err := fn("1", "[1]"); err == nil {
			t.Fatal("expected arguments-mismatch error")
		}
	})
}
