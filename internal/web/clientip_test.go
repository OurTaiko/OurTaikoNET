package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientIPTrust(t *testing.T) {
	for _, tc := range []struct{ name, proxies, peer, header, expected string }{
		{"default ignores headers", "", "127.0.0.1:1234", "198.51.100.1", "127.0.0.1"},
		{"trusted loopback", "127.0.0.1/32", "127.0.0.1:1234", "198.51.100.1", "198.51.100.1"},
		{"untrusted peer", "127.0.0.1/32", "192.0.2.1:1234", "198.51.100.1", "192.0.2.1"},
		{"IPv6", "::1/128", "[::1]:1234", "2001:db8::1", "2001:db8::1"},
		{"mapped IPv4", "127.0.0.1/32", "[::ffff:127.0.0.1]:1234", "::ffff:198.51.100.1", "198.51.100.1"},
		{"invalid value", "127.0.0.1/32", "127.0.0.1:1234", "unknown", "127.0.0.1"},
		{"no header", "127.0.0.1/32", "127.0.0.1:1234", "", "127.0.0.1"},
		{"chain rejected", "127.0.0.1/32", "127.0.0.1:1234", "198.51.100.1, 192.0.2.1", "127.0.0.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proxies, err := ParseTrustedProxies(tc.proxies)
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tc.peer
			r.Header.Set("X-Real-IP", tc.header)
			if got := ClientIP(r, proxies); got != tc.expected {
				t.Fatalf("got %s, want %s", got, tc.expected)
			}
			r.Header.Add("X-Real-IP", "203.0.113.1")
			if got := ClientIP(r, proxies); got == "203.0.113.1" {
				t.Fatal("accepted duplicate IP header")
			}
		})
	}
	for _, value := range []string{"localhost", "127.0.0.1", "127.0.0.1/32,"} {
		if _, err := ParseTrustedProxies(value); err == nil {
			t.Fatalf("accepted invalid CIDR %q", value)
		}
	}
}

func TestDefaultGateways(t *testing.T) {
	// A container on two networks whose gateway is the second's (172.21.0.1).
	table := "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n" +
		"eth1\t00000000\t010015AC\t0003\t0\t0\t0\t00000000\t0\t0\t0\n" +
		"eth0\t000014AC\t00000000\t0001\t0\t0\t0\t0000FFFF\t0\t0\t0\n" +
		"eth1\t000015AC\t00000000\t0001\t0\t0\t0\t0000FFFF\t0\t0\t0\n"
	got := DefaultGateways(strings.NewReader(table))
	if len(got) != 1 || got[0].String() != "172.21.0.1/32" {
		t.Fatalf("got %v", got)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "172.21.0.1:5555"
	r.Header.Set("X-Real-IP", "114.114.114.114")
	if ip := ClientIP(r, got); ip != "114.114.114.114" {
		t.Fatalf("gateway not trusted: %s", ip)
	}
	r.RemoteAddr = "172.21.0.3:5555"
	if ip := ClientIP(r, got); ip != "172.21.0.3" {
		t.Fatalf("another container trusted: %s", ip)
	}
	if got := DefaultGateways(strings.NewReader("garbage\n\n")); len(got) != 0 {
		t.Fatalf("got %v from garbage", got)
	}
}
