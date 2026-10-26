package main

import (
	"log"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Microsoft documents the Evergreen Runtime's pv value for per-user and machine installs:
// https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/distribution
func installedWebView2(read func(registry.Key, uint32) string) string {
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		for _, view := range []uint32{registry.WOW64_32KEY, registry.WOW64_64KEY} {
			if version := read(root, view); version != "" && version != "0.0.0.0" {
				return version
			}
		}
	}
	return ""
}

func checkWebView2() bool {
	version := installedWebView2(func(root registry.Key, view uint32) string {
		key, err := registry.OpenKey(root, `Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`, registry.QUERY_VALUE|view)
		if err != nil {
			return ""
		}
		defer key.Close()
		value, _, _ := key.GetStringValue("pv")
		return value
	})
	if version != "" {
		return true
	}
	message, _ := windows.UTF16PtrFromString("Preen 需要 Microsoft Edge WebView2 Runtime。\n请从微软官方下载页安装后重新启动 Preen：\nhttps://developer.microsoft.com/microsoft-edge/webview2/\n\n便携包不包含 WebView2 Runtime。")
	title, _ := windows.UTF16PtrFromString("Preen — 缺少 WebView2 Runtime")
	if _, err := windows.MessageBox(0, message, title, windows.MB_OK|windows.MB_ICONERROR|windows.MB_SYSTEMMODAL); err != nil {
		log.Print(err)
	}
	return false
}
