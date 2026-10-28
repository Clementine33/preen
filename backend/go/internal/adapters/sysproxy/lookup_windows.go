package sysproxy

import (
	"fmt"
	"net/url"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The WinHTTP structures and flags used below, as winhttp.h declares them.
type ieProxyConfig struct {
	autoDetect    int32
	autoConfigURL *uint16
	proxy         *uint16
	proxyBypass   *uint16
}

type autoProxyOptions struct {
	flags                 uint32
	autoDetectFlags       uint32
	autoConfigURL         *uint16
	reserved1             uintptr
	reserved2             uint32
	autoLogonIfChallenged int32
}

type proxyInfo struct {
	accessType  uint32
	proxy       *uint16
	proxyBypass *uint16
}

const (
	accessTypeNoProxy    = 1
	accessTypeNamedProxy = 3
	autoProxyAutoDetect  = 0x1
	autoProxyConfigURL   = 0x2
	autoDetectDHCPAndDNS = 0x1 | 0x2
)

var (
	winhttp           = windows.NewLazySystemDLL("winhttp.dll")
	procIEProxyConfig = winhttp.NewProc("WinHttpGetIEProxyConfigForCurrentUser")
	procOpen          = winhttp.NewProc("WinHttpOpen")
	procProxyForURL   = winhttp.NewProc("WinHttpGetProxyForUrl")
	procGlobalFree    = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalFree")

	// One session serves every PAC evaluation, so WinHTTP caches the script.
	session = sync.OnceValues(func() (uintptr, error) {
		agent, _ := windows.UTF16PtrFromString("Preen")
		h, _, err := procOpen.Call(uintptr(unsafe.Pointer(agent)), accessTypeNoProxy, 0, 0, 0)
		if h == 0 {
			return 0, err
		}
		return h, nil
	})
)

// Lookup reads the current user's Internet Options, the settings WebView2
// uses: auto-detection and a PAC address are evaluated by WinHTTP, and when
// that fails the manual proxy applies, as in browsers.
func Lookup(target *url.URL) (*url.URL, error) {
	if loopback(target.Hostname()) {
		return nil, nil
	}
	var cfg ieProxyConfig
	if ok, _, err := procIEProxyConfig.Call(uintptr(unsafe.Pointer(&cfg))); ok == 0 {
		return nil, fmt.Errorf("read the system proxy: %w", err)
	}
	defer free(cfg.autoConfigURL, cfg.proxy, cfg.proxyBypass)
	if cfg.autoDetect != 0 || cfg.autoConfigURL != nil {
		if proxy, ok := autoProxy(target, &cfg); ok {
			return parseWindows(proxy, "").route(target)
		}
	}
	return parseWindows(text(cfg.proxy), text(cfg.proxyBypass)).route(target)
}

// autoProxy evaluates auto-detection or the PAC address for target; ok is
// false when neither produced an answer. "" with ok means direct.
func autoProxy(target *url.URL, cfg *ieProxyConfig) (string, bool) {
	h, err := session()
	if err != nil {
		return "", false
	}
	opts := autoProxyOptions{autoLogonIfChallenged: 1}
	if cfg.autoDetect != 0 {
		opts.flags |= autoProxyAutoDetect
		opts.autoDetectFlags = autoDetectDHCPAndDNS
	}
	if cfg.autoConfigURL != nil {
		opts.flags |= autoProxyConfigURL
		opts.autoConfigURL = cfg.autoConfigURL
	}
	address, err := windows.UTF16PtrFromString(target.String())
	if err != nil {
		return "", false
	}
	var info proxyInfo
	if ok, _, _ := procProxyForURL.Call(h, uintptr(unsafe.Pointer(address)), uintptr(unsafe.Pointer(&opts)),
		uintptr(unsafe.Pointer(&info))); ok == 0 {
		return "", false
	}
	defer free(info.proxy, info.proxyBypass)
	if info.accessType != accessTypeNamedProxy {
		return "", true
	}
	return text(info.proxy), true
}

func text(p *uint16) string {
	if p == nil {
		return ""
	}
	return windows.UTF16PtrToString(p)
}

func free(ps ...*uint16) {
	for _, p := range ps {
		if p != nil {
			_, _, _ = procGlobalFree.Call(uintptr(unsafe.Pointer(p)))
		}
	}
}
