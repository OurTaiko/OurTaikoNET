// Package web holds what every part of the site does with an HTTP request in the same way.
package web

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
)

// DefaultGateways reads the IPv4 default gateways of a Linux route table (/proc/net/route).
//
// In a container the default gateway is the host, and a proxy on the host that reaches a
// published port arrives from it. Which of the container's networks gives the gateway is
// Docker's choice (for a container on two networks, the one first by name unless a priority
// says otherwise), so a fixed TRUSTED_PROXY_CIDRS stops matching when a network is added;
// TRUSTED_PROXY_GATEWAY=true trusts whatever the gateway is. Other containers on the same
// networks come from their own addresses, never from the gateway's.
func DefaultGateways(table io.Reader) []netip.Prefix {
	var result []netip.Prefix
	lines := bufio.NewScanner(table)
	for lines.Scan() {
		// Iface Destination Gateway Flags ... Mask ...; addresses in hex, host byte order.
		fields := strings.Fields(lines.Text())
		if len(fields) < 8 || fields[1] != "00000000" || fields[7] != "00000000" {
			continue
		}
		raw, err := hex.DecodeString(fields[2])
		if err != nil || len(raw) != 4 {
			continue
		}
		var ip [4]byte
		binary.BigEndian.PutUint32(ip[:], binary.LittleEndian.Uint32(raw))
		if addr := netip.AddrFrom4(ip); !addr.IsUnspecified() {
			result = append(result, netip.PrefixFrom(addr, 32))
		}
	}
	return result
}

// ReadDefaultGateways is DefaultGateways of this system's route table; none where there is none.
func ReadDefaultGateways() []netip.Prefix {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return nil
	}
	defer f.Close()
	return DefaultGateways(f)
}

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
