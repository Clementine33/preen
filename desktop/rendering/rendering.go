// Package rendering settles the WebKitGTK environment before Wails initializes.
// Wails' Linux package disables the DMA-BUF renderer on any NVIDIA GPU unless
// the variable is already set, but current drivers render correctly with it
// and noticeably slower without it. main imports this package, which sorts
// before github.com/wailsapp and imports none of it, so Go initializes it
// first. An explicit value from the user's environment is kept.
package rendering

import (
	"os"
	"runtime"
)

func init() {
	if _, set := os.LookupEnv("WEBKIT_DISABLE_DMABUF_RENDERER"); runtime.GOOS == "linux" && !set {
		_ = os.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "0")
	}
}
