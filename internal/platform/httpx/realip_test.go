package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

func TestRealIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("172.30.0.10/32"), netip.MustParsePrefix("fd00::/8")}
	cases := []struct {
		name   string
		peer   string
		xff    []string
		wantIP string
	}{
		{"direct client, no header", "203.0.113.7:5000", nil, "203.0.113.7"},
		{"direct client spoofing the header", "203.0.113.7:5000", []string{"1.2.3.4"}, "203.0.113.7"},
		{"trusted proxy", "172.30.0.10:41000", []string{"198.51.100.9"}, "198.51.100.9"},
		{"trusted proxy, client-supplied chain", "172.30.0.10:41000", []string{"6.6.6.6, 198.51.100.9"}, "198.51.100.9"},
		{"trusted proxy, multiple header lines", "172.30.0.10:41000", []string{"6.6.6.6", "198.51.100.9"}, "198.51.100.9"},
		{"trusted proxy chain skipped", "172.30.0.10:41000", []string{"198.51.100.9, 172.30.0.10"}, "198.51.100.9"},
		{"trusted proxy, IPv6 client", "172.30.0.10:41000", []string{"2001:db8::5"}, "2001:db8::5"},
		{"trusted proxy, mapped IPv4", "172.30.0.10:41000", []string{"::ffff:198.51.100.9"}, "198.51.100.9"},
		{"trusted proxy, malformed entry", "172.30.0.10:41000", []string{"198.51.100.9, garbage"}, "172.30.0.10"},
		{"trusted proxy, no header", "172.30.0.10:41000", nil, "172.30.0.10"},
		{"trusted proxy, only trusted hops", "172.30.0.10:41000", []string{"fd00::1"}, "172.30.0.10"},
		{"same subnet but not the proxy", "172.30.0.11:41000", []string{"1.2.3.4"}, "172.30.0.11"},
	}
	for _, c := range cases {
		var got string
		h := httpx.RealIP(trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			ip, _ := httpx.ClientIP(r)
			got = ip.String()
		}))
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		r.RemoteAddr = c.peer
		for _, v := range c.xff {
			r.Header.Add("X-Forwarded-For", v)
		}
		h.ServeHTTP(httptest.NewRecorder(), r)
		if got != c.wantIP {
			t.Errorf("%s: client = %s, want %s", c.name, got, c.wantIP)
		}
		if r.RemoteAddr != c.peer {
			t.Errorf("%s: caller's request was mutated", c.name)
		}
	}
}

func TestRealIPDisabledWithoutTrustedProxies(t *testing.T) {
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	if h := httpx.RealIP(nil)(next); h == nil {
		t.Fatal("nil handler")
	}
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.RemoteAddr = "172.30.0.10:1"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	var got string
	httpx.RealIP(nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ip, _ := httpx.ClientIP(r)
		got = ip.String()
	})).ServeHTTP(httptest.NewRecorder(), r)
	if got != "172.30.0.10" {
		t.Fatalf("header trusted without configuration: %s", got)
	}
}
