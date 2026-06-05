package main

import (
	"github.com/moxcomic/go-webview"
	_ "github.com/moxcomic/go-webview/embedded"
)

func main() {
	w, err := webview.New(true)
	if err != nil {
		panic(err)
	}
	w.SetTitle("Basic Example")
	w.SetSize(480, 320, webview.HintNone)
	w.SetHtml("Thanks for using webview!")
	w.Run()
	w.Destroy()
}
