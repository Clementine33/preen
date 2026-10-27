package main

import (
	"runtime"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// The Lyrics Window is a second frameless webview on the web build's
// /lyrics-window route (ADR 0016). The frames and commands travel between the
// two webviews as Wails events that Go never interprets; Go owns only the
// window, its click-through lock, and the lyrics:closed and lyrics:locked
// notifications.

const (
	lyricsTitle  = "Preen 歌词" // stable, so users can match it in compositor window rules
	lyricsWidth  = 520
	lyricsHeight = 220
	lyricsMargin = 48 // gap above the bottom edge of the work area on Windows
)

type lyricsWindow struct {
	mu     sync.Mutex
	window *application.WebviewWindow
	locked bool
}

// OpenLyricsWindow opens the Lyrics Window, or focuses it when already open.
func (d *Desktop) OpenLyricsWindow() {
	l := &d.lyrics
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.window != nil {
		l.window.Focus()
		return
	}
	options := application.WebviewWindowOptions{
		Title: lyricsTitle, Width: lyricsWidth, Height: lyricsHeight, URL: "/lyrics-window",
		Frameless: true, AlwaysOnTop: true, // Wayland ignores always-on-top; accepted (ADR 0016)
		BackgroundColour: application.NewRGBA(0x14, 0x15, 0x18, 255), // --invert-bg
	}
	if runtime.GOOS == "windows" {
		options.BackgroundType = application.BackgroundTypeTransparent
		options.BackgroundColour = application.NewRGBA(0, 0, 0, 0)
		if screen := d.app.Screen.GetPrimary(); screen != nil {
			area := screen.WorkArea
			options.Screen = screen
			options.InitialPosition = application.WindowXY
			options.X = (area.Width - lyricsWidth) / 2
			options.Y = area.Height - lyricsHeight - lyricsMargin
		}
	}
	window := d.app.Window.NewWithOptions(options)
	l.window = window
	window.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) { d.lyricsClosed(window) })
}

// CloseLyricsWindow closes the Lyrics Window if it is open.
func (d *Desktop) CloseLyricsWindow() {
	d.lyrics.mu.Lock()
	window := d.lyrics.window
	d.lyrics.mu.Unlock()
	if window != nil {
		window.Close()
	}
}

// SetLyricsLocked makes the Lyrics Window click-through. Only Windows has a
// transparent window worth locking; elsewhere it does nothing.
func (d *Desktop) SetLyricsLocked(locked bool) {
	if runtime.GOOS != "windows" {
		return
	}
	l := &d.lyrics
	l.mu.Lock()
	if l.window == nil || l.locked == locked {
		l.mu.Unlock()
		return
	}
	l.locked = locked
	l.window.SetIgnoreMouseEvents(locked)
	l.mu.Unlock()
	d.app.Event.Emit("lyrics:locked", locked)
}

// lyricsClosed runs when the window goes away for any cause; the lock dies with it.
func (d *Desktop) lyricsClosed(window *application.WebviewWindow) {
	l := &d.lyrics
	l.mu.Lock()
	if l.window != window {
		l.mu.Unlock()
		return
	}
	l.window, l.locked = nil, false
	l.mu.Unlock()
	d.app.Event.Emit("lyrics:closed")
}
