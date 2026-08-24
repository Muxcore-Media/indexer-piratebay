package internal

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Shared VPN HTTP policy with downloader-native-torrent (documented in README / _mvp/tls):
// live upstream HTTP must egress via the source-routed WireGuard iface derived from WG_CONF
// (SO_BINDTODEVICE + LocalAddr). Fixture / injected clients skip this path.

func ifaceFromWGConf(path string) string {
	base := filepath.Base(strings.TrimSpace(path))
	ext := filepath.Ext(base)
	if ext != "" {
		return strings.TrimSuffix(base, ext)
	}
	return base
}

func interfaceIPv4(iface string) (net.IP, error) {
	ifi, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, fmt.Errorf("vpn iface %q: %w", iface, err)
	}
	addrs, err := ifi.Addrs()
	if err != nil {
		return nil, err
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() {
			continue
		}
		if ip4 := ipnet.IP.To4(); ip4 != nil {
			return ip4, nil
		}
	}
	return nil, fmt.Errorf("vpn iface %q has no IPv4 address", iface)
}

// newVPNBoundHTTPClient dials only via the WireGuard interface named from WG_CONF.
func newVPNBoundHTTPClient(wgConf string, timeout time.Duration) (*http.Client, string, error) {
	wgConf = strings.TrimSpace(wgConf)
	if wgConf == "" {
		wgConf = strings.TrimSpace(os.Getenv("WG_CONF"))
	}
	if err := validateWGConfReadable(wgConf); err != nil {
		return nil, "", err
	}
	iface := ifaceFromWGConf(wgConf)
	localIP, err := interfaceIPv4(iface)
	if err != nil {
		return nil, "", err
	}
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		LocalAddr: &net.TCPAddr{IP: localIP},
		Control: func(network, address string, c syscall.RawConn) error {
			var ctrlErr error
			if err := c.Control(func(fd uintptr) {
				ctrlErr = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, iface)
			}); err != nil {
				return err
			}
			return ctrlErr
		},
	}
	tr := &http.Transport{
		Proxy:                 nil, // never clearnet HTTP proxy around the VPN bind
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          8,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{Timeout: timeout, Transport: tr}, iface, nil
}
