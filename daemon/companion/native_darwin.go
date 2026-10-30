//go:build darwin && cgo

// Package companion presents the local renderer in the operating system's webview.
package companion

import (
	"context"
	"fmt"
	"log"
	"runtime"

	webview "github.com/webview/webview_go"
)

func init() { runtime.LockOSThread() }

func Available() bool { return true }

func Open(ctx context.Context, address, title string, export func(string) (string, error)) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	window := webview.New(false)
	if window == nil {
		return fmt.Errorf("cannot create native stef window")
	}
	defer window.Destroy()
	window.SetTitle(title)
	window.SetSize(940, 880, webview.HintNone)
	window.SetSize(420, 560, webview.HintMin)
	if err := window.Bind("stefSaveSession", export); err != nil {
		return err
	}
	if err := window.Bind("stefNativeReady", func() { log.Print("stef native renderer ready") }); err != nil {
		return err
	}
	window.Navigate(address + "/?companion=1")
	closed := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			window.Terminate()
		case <-closed:
		}
	}()
	window.Run()
	close(closed)
	<-stopped // Never call Terminate concurrently with Destroy.
	return nil
}
