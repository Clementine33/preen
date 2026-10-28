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
// window, its click-through lock, the small unlock window that exists only
// while the lock is on, and the lyrics:closed notification. Unlocking is the
// unlock window's business alone; the main window only opens and closes.

const (
	lyricsTitle  = "Preen 歌词" // stable, so users can match it in compositor window rules
	lyricsWidth  = 520
	lyricsHeight = 220
	lyricsMargin = 48 // gap above the bottom edge of the work area on Windows

	unlockTitle  = "Preen 歌词解锁"
	unlockWidth  = 36
	unlockHeight = 28
)

type lyricsWindow struct {
	mu     sync.Mutex
	window *application.WebviewWindow
	locked bool
	unlock *application.WebviewWindow // the unlock window; non-nil exactly while locked and placed
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

// SetLyricsLocked makes the Lyrics Window click-through and floats a small
// unlock window over its top edge, since a locked window can no longer be
// clicked; unlocking closes that window again. Only Windows has a transparent
// window worth locking; elsewhere it does nothing.
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
	var stale *application.WebviewWindow
	if locked {
		l.unlock = d.newUnlockWindow(l.window)
	} else {
		stale, l.unlock = l.unlock, nil
	}
	l.mu.Unlock()
	if stale != nil {
		stale.Close() // outside the lock: its closing handler takes it
	}
	d.app.Event.Emit("lyrics:locked", locked) // the page hides its controls while locked
}

// newUnlockWindow creates the not click-through, always-on-top unlock window at
// the top centre of the locked Lyrics Window. The locked window cannot move,
// so the position is computed once. Wails reports position and size in DIP.
func (d *Desktop) newUnlockWindow(lyrics *application.WebviewWindow) *application.WebviewWindow {
	x, y := lyrics.Position()
	width, _ := lyrics.Size()
	// Wails offers no non-activating option without replacing the whole extended style; hidden on the taskbar is the closest.
	window := d.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: unlockTitle, Width: unlockWidth, Height: unlockHeight, URL: "/lyrics-unlock",
		Frameless: true, AlwaysOnTop: true, DisableResize: true,
		CloseButtonState: application.ButtonHidden, // the 36px chip has no room for the system close button
		InitialPosition:  application.WindowXY, X: x + (width-unlockWidth)/2, Y: y,
		BackgroundType:   application.BackgroundTypeTransparent,
		BackgroundColour: application.NewRGBA(0, 0, 0, 0),
		Windows:          application.WindowsWindow{HiddenOnTaskbar: true},
	})
	window.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		d.lyrics.mu.Lock()
		if d.lyrics.unlock == window {
			d.lyrics.unlock = nil
		}
		d.lyrics.mu.Unlock()
	})
	return window
}

// lyricsClosed runs when the window goes away for any cause; the lock and its
// unlock window die with it.
func (d *Desktop) lyricsClosed(window *application.WebviewWindow) {
	l := &d.lyrics
	l.mu.Lock()
	if l.window != window {
		l.mu.Unlock()
		return
	}
	stale := l.unlock
	l.window, l.locked, l.unlock = nil, false, nil
	l.mu.Unlock()
	if stale != nil {
		stale.Close()
	}
	d.app.Event.Emit("lyrics:closed")
}
