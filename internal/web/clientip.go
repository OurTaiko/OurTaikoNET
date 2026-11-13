// Package web holds what every part of the site does with an HTTP request in the same way.
package web

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ParseTrustedProxies reads TRUSTED_PROXY_CIDRS: the addresses of the proxies in front of the
// site, whose X-Real-IP is believed.
func ParseTrustedProxies(value string) ([]netip.Prefix, error) {
	var result []netip.Prefix
	if strings.TrimSpace(value) == "" {
		return result, nil
	}
	for _, raw := range strings.Split(value, ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXY_CIDRS must contain comma-separated IP CIDRs")
		}
		result = append(result, prefix.Masked())
	}
	return result, nil
}

// ClientIP is the address a request came from. The final trusted proxy must overwrite
// X-Real-IP with the verified client IP. Never trust arbitrary forwarding chains or headers
// from direct clients.
func ClientIP(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	peer = peer.Unmap()
	for _, prefix := range trusted {
		if !prefix.Contains(peer) {
			continue
		}
		values := r.Header.Values("X-Real-IP")
		if len(values) == 1 {
			client, err := netip.ParseAddr(strings.TrimSpace(values[0]))
			if err == nil && client.Zone() == "" {
				return client.Unmap().String()
			}
		}
		break
	}
	return peer.String()
}
