package webview

import (
	"errors"
	"testing"
)

// These tests exercise the binding wrapper directly, without the native library
// or an event loop. They are a regression guard for the nil-error panic: a bound
// function declaring an error return must succeed on the nil-error path instead
// of panicking on a single-value `.(error)` assertion.
//
// Each scenario is its own function (rather than subtests of one large table) so
// no single function trips the cyclomatic-complexity limit and the per-case
// assertions stay flat and readable.

func TestMakeFuncWrapperValueOnly(t *testing.T) {
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
}

func TestMakeFuncWrapperValueAndNilError(t *testing.T) {
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
}

func TestMakeFuncWrapperJustNilError(t *testing.T) {
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
}

func TestMakeFuncWrapperRealErrorPropagated(t *testing.T) {
	sentinel := errors.New("boom")
	fn, err := makeFuncWrapper(func() (int, error) { return 0, sentinel })
	if err != nil {
		t.Fatalf("makeFuncWrapper: %v", err)
	}
	if _, err := fn("1", "[]"); !errors.Is(err, sentinel) {
		t.Fatalf("call: got %v, want sentinel", err)
	}
}

func TestMakeFuncWrapperRejectsNonFunc(t *testing.T) {
	if _, err := makeFuncWrapper(42); err == nil {
		t.Fatal("expected error binding a non-function")
	}
}

func TestMakeFuncWrapperArgumentCountMismatch(t *testing.T) {
	fn, err := makeFuncWrapper(func(a, b int) int { return a + b })
	if err != nil {
		t.Fatalf("makeFuncWrapper: %v", err)
	}
	if _, err := fn("1", "[1]"); err == nil {
		t.Fatal("expected arguments-mismatch error")
	}
}
