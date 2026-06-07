package webview

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"sync"

	"github.com/ebitengine/purego"
)

// Global state for bound callbacks. bindingMap is keyed by a globally-unique
// counter so entries from different webview instances never collide; boundNames
// is namespaced by handle so the same JavaScript name can be bound on multiple
// windows.
var (
	bindMu         sync.Mutex
	bindingMap     = make(map[uintptr]bindingEntry)
	boundNames     = make(map[uintptr]map[string]uintptr)
	bindingCounter uintptr
)

// bindingEntry stores a bound callback and the webview it belongs to.
type bindingEntry struct {
	fn func(id, req string) (any, error)
	w  *webview
}

func (w *webview) Bind(name string, f any) error {
	fn, err := makeFuncWrapper(f)
	if err != nil {
		return err
	}

	bindMu.Lock()
	names := boundNames[w.handle]
	if names == nil {
		names = make(map[string]uintptr)
		boundNames[w.handle] = names
	}
	if _, exists := names[name]; exists {
		bindMu.Unlock()
		return errors.New("function name already bound")
	}
	contextKey := bindingCounter
	bindingCounter++
	bindingMap[contextKey] = bindingEntry{w: w, fn: fn}
	names[name] = contextKey
	bindMu.Unlock()

	nameBytes, namePtr := cString(name)
	purego.SyscallN(pBind, w.handle, namePtr, bindingCallback, contextKey)
	runtime.KeepAlive(nameBytes)
	return nil
}

func (w *webview) Unbind(name string) error {
	bindMu.Lock()
	names := boundNames[w.handle]
	contextKey, exists := names[name]
	if !exists {
		bindMu.Unlock()
		return errors.New("function name not bound")
	}
	delete(names, name)
	delete(bindingMap, contextKey)
	if len(names) == 0 {
		delete(boundNames, w.handle)
	}
	bindMu.Unlock()

	cs, namePtr := cString(name)
	purego.SyscallN(pUnbind, w.handle, namePtr)
	runtime.KeepAlive(cs)
	return nil
}

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// makeFuncWrapper inspects a user-supplied function "f" via reflection once,
// validating its signature and caching the relevant details. It returns a
// closure that, given (id, req string), decodes JSON args, calls the underlying
// function, and returns (value, error).
//
//nolint:cyclop,funlen
func makeFuncWrapper(f any) (func(id, req string) (any, error), error) {
	v := reflect.ValueOf(f)
	if v.Kind() != reflect.Func {
		return nil, errors.New("only functions can be bound")
	}

	funcType := v.Type()
	outCount := funcType.NumOut()
	if outCount > 2 {
		return nil, errors.New("function may only return a value or value+error")
	}

	numIn := funcType.NumIn()
	isVariadic := funcType.IsVariadic()
	inTypes := make([]reflect.Type, numIn)
	for i := range numIn {
		inTypes[i] = funcType.In(i)
	}

	var returnsError bool
	switch outCount {
	case 1:
		if funcType.Out(0).Implements(errorType) {
			returnsError = true
		}
	case 2:
		if !funcType.Out(1).Implements(errorType) {
			return nil, errors.New("second return value must implement error")
		}
	}

	fn := func(id, req string) (any, error) {
		var rawArgs []json.RawMessage
		if err := json.Unmarshal([]byte(req), &rawArgs); err != nil {
			return nil, err
		}
		if (!isVariadic && len(rawArgs) != numIn) || (isVariadic && len(rawArgs) < numIn-1) {
			return nil, errors.New("function arguments mismatch")
		}

		args := make([]reflect.Value, len(rawArgs))
		for i := range rawArgs {
			var argVal reflect.Value
			if isVariadic && i >= numIn-1 {
				argVal = reflect.New(inTypes[numIn-1].Elem())
			} else {
				argVal = reflect.New(inTypes[i])
			}
			if err := json.Unmarshal(rawArgs[i], argVal.Interface()); err != nil {
				return nil, err
			}
			args[i] = argVal.Elem()
		}

		res := v.Call(args)

		switch outCount {
		case 0:
			return nil, nil //nolint:nilnil
		case 1:
			if returnsError {
				// Comma-ok: a nil error return slot yields a nil interface{},
				// and a single-value assertion on nil would panic.
				resErr, _ := res[0].Interface().(error)
				return nil, resErr
			}
			return res[0].Interface(), nil
		case 2:
			resErr, _ := res[1].Interface().(error)
			return res[0].Interface(), resErr
		default:
			panic("unreachable")
		}
	}

	return fn, nil
}

// bindingCallbackFn is invoked by the native webview when a bound JS function is
// called. It copies the native strings, then runs the user callback on its own
// goroutine so a slow callback doesn't block the UI thread.
func bindingCallbackFn(idPtr, reqPtr, arg uintptr) uintptr {
	bindMu.Lock()
	entry, ok := bindingMap[arg]
	bindMu.Unlock()
	if !ok {
		return 0
	}

	w := entry.w
	w.mu.Lock()
	if !w.alive {
		w.mu.Unlock()
		return 0
	}
	w.wg.Add(1)
	w.mu.Unlock()

	id := goString(idPtr)
	req := goString(reqPtr)

	go func() {
		defer w.wg.Done()

		// Run the user callback with panic protection so a panicking binding is
		// reported back to JS as an error instead of crashing the whole process.
		var resultValue any
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("webview: bound function panicked: %v", r)
				}
			}()
			resultValue, err = entry.fn(id, req)
		}()

		status, resultJSON := marshalBindingResult(resultValue, err)

		// Skip the native return if the webview was destroyed while we ran. The
		// WaitGroup guarantees Destroy cannot free the handle until this
		// goroutine returns, so calling pReturn here is safe.
		w.mu.Lock()
		if !w.alive {
			w.mu.Unlock()
			return
		}
		// Create new C strings for the ID and result as the originals are no
		// longer valid.
		idBytes, newIDPtr := cString(id)
		resBytes, newResPtr := cString(resultJSON)
		purego.SyscallN(pReturn, w.handle, newIDPtr, uintptr(status), newResPtr)
		runtime.KeepAlive(idBytes)
		runtime.KeepAlive(resBytes)
		w.mu.Unlock()
	}()

	return 0
}

// marshalBindingResult turns a bound callback's (value, error) outcome into the
// (status, JSON) pair the native webview_return expects: status -1 with the
// error text on failure, status 0 with the JSON-encoded value on success.
func marshalBindingResult(value any, callErr error) (int, string) {
	if callErr != nil {
		return -1, jsonString(callErr.Error())
	}
	data, err := json.Marshal(value)
	if err != nil {
		return -1, jsonString(err.Error())
	}
	return 0, string(data)
}
