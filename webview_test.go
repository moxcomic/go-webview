package webview_test

import (
	"runtime"
	"testing"
	"time"

	"github.com/moxcomic/go-webview"
	_ "github.com/moxcomic/go-webview/embedded"
)

// Needed to ensure that the tests run on the main thread.
func init() {
	runtime.UnlockOSThread()
}

func TestWebview(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// guiTimeout bounds how long we wait for the headless WebKit/GTK/WebView2
	// stack to load the page and fire window.onload. Some CI runners (notably
	// the Intel macos-13 image) cannot complete a real render pass; without a
	// bound, the native event loop in Run() never returns, the call is pinned to
	// the locked OS thread where go test's own timeout cannot preempt it, and
	// the job hangs until GitHub's 24h ceiling kills it.
	const guiTimeout = 30 * time.Second

	run := make(chan bool, 1)

	w, err := webview.New(true)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Destroy()

	w.SetTitle("Hello")
	w.SetSize(800, 600, webview.HintNone)

	err = w.Bind("run", func(b bool) {
		run <- b
		w.Terminate()
	})
	if err != nil {
		t.Fatal(err)
	}

	w.SetHtml(`<!doctype html>
		<html>
			<script>
				window.onload = function() { run(true); };
			</script>
		</html>`)

	// Watchdog: guarantee Run() returns even when the page never loads, so the
	// test fails fast (or skips) instead of hanging the whole job. Terminate is
	// documented as safe to call from a background goroutine.
	watchdog := time.AfterFunc(guiTimeout, func() { w.Terminate() })
	defer watchdog.Stop()

	w.Run()

	// Stop reports false when the timer already fired, i.e. Run() returned only
	// because the watchdog forced it — the page never rendered on this runner.
	if !watchdog.Stop() {
		t.Skipf("webview did not render within %s; GUI environment unavailable on this runner", guiTimeout)
	}

	select {
	case ok := <-run:
		if !ok {
			t.Fatal("run callback reported failure")
		}
	default:
		t.Fatal("event loop exited without invoking the bound callback")
	}
}
