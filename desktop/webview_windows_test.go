package main

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestInstalledWebView2(t *testing.T) {
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		for _, view := range []uint32{registry.WOW64_32KEY, registry.WOW64_64KEY} {
			got := installedWebView2(func(candidate registry.Key, candidateView uint32) string {
				if candidate == root && candidateView == view {
					return "142.0.3595.65"
				}
				return "0.0.0.0"
			})
			if got != "142.0.3595.65" {
				t.Fatalf("root %v view %v: got %q", root, view, got)
			}
		}
	}
	for _, value := range []string{"", "0.0.0.0"} {
		if got := installedWebView2(func(registry.Key, uint32) string { return value }); got != "" {
			t.Fatalf("missing Runtime %q: got %q", value, got)
		}
	}
}
