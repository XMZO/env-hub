package main

import (
	"log"
	"net"
	"net/http"
	"strings"
)

// Client-IP headers (CF-Connecting-IP, X-Real-IP) are honored only when the
// direct peer (RemoteAddr) is a trusted proxy, so that clients connecting
// directly cannot spoof another IP to bypass rate limiting or poison the
// __CLIENT_IP__ script placeholders.

// privateCIDRs cover loopback, RFC1918, link-local and ULA ranges — the
// typical addresses a local reverse proxy (nginx, Caddy, docker network)
// connects from.
var privateCIDRs = []string{
	"127.0.0.0/8",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"169.254.0.0/16",
	"::1/128",
	"fc00::/7",
	"fe80::/10",
}

// cloudflareCIDRs are Cloudflare's published edge ranges
// (https://www.cloudflare.com/ips/).
var cloudflareCIDRs = []string{
	"173.245.48.0/20",
	"103.21.244.0/22",
	"103.22.200.0/22",
	"103.31.4.0/22",
	"141.101.64.0/18",
	"108.162.192.0/18",
	"190.93.240.0/20",
	"188.114.96.0/20",
	"197.234.240.0/22",
	"198.41.128.0/17",
	"162.158.0.0/15",
	"104.16.0.0/13",
	"104.24.0.0/14",
	"172.64.0.0/13",
	"131.0.72.0/22",
	"2400:cb00::/32",
	"2606:4700::/32",
	"2803:f800::/32",
	"2405:b500::/32",
	"2405:8100::/32",
	"2a06:98c0::/29",
	"2c0f:f248::/32",
}

type ipTrust struct {
	all  bool
	nets []*net.IPNet
}

// trustedProxies is set once at startup from TRUSTED_PROXIES.
// Default: private networks + Cloudflare edges.
var trustedProxies = parseTrustedProxies("")

// parseTrustedProxies parses the TRUSTED_PROXIES spec: a comma-separated
// list of CIDRs/IPs and the keywords "private" and "cloudflare".
// "" = private + cloudflare (default), "none" = trust nothing, "*" = trust all.
func parseTrustedProxies(spec string) *ipTrust {
	t := &ipTrust{}
	switch strings.ToLower(strings.TrimSpace(spec)) {
	case "":
		t.addCIDRs(privateCIDRs)
		t.addCIDRs(cloudflareCIDRs)
		return t
	case "none":
		return t
	case "*", "all":
		t.all = true
		return t
	}
	for _, tok := range strings.Split(spec, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		switch strings.ToLower(tok) {
		case "private":
			t.addCIDRs(privateCIDRs)
		case "cloudflare":
			t.addCIDRs(cloudflareCIDRs)
		default:
			cidr := tok
			if !strings.Contains(cidr, "/") {
				if strings.Contains(cidr, ":") {
					cidr += "/128"
				} else {
					cidr += "/32"
				}
			}
			_, n, err := net.ParseCIDR(cidr)
			if err != nil {
				log.Fatalf("TRUSTED_PROXIES: invalid entry %q", tok)
			}
			t.nets = append(t.nets, n)
		}
	}
	return t
}

func (t *ipTrust) addCIDRs(cidrs []string) {
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		t.nets = append(t.nets, n)
	}
}

func (t *ipTrust) contains(ip net.IP) bool {
	if t.all {
		return true
	}
	for _, n := range t.nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// realIP returns the client IP used for rate limiting, Turnstile verification
// and script placeholders.
func realIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer := net.ParseIP(host)
	if peer != nil && trustedProxies.contains(peer) {
		for _, h := range [...]string{"CF-Connecting-IP", "X-Real-IP"} {
			if v := strings.TrimSpace(r.Header.Get(h)); v != "" {
				if ip := net.ParseIP(v); ip != nil {
					return ip.String()
				}
			}
		}
	}
	if peer != nil {
		return peer.String()
	}
	return host
}

// requestScheme returns "http" or "https" for building absolute URLs,
// honoring the value of X-Forwarded-Proto when present.
func requestScheme(r *http.Request) string {
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		first, _, _ := strings.Cut(proto, ",")
		if strings.EqualFold(strings.TrimSpace(first), "https") {
			return "https"
		}
		return "http"
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
