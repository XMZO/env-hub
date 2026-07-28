package main

import (
	"net/http/httptest"
	"testing"
)

func TestRealIPTrust(t *testing.T) {
	orig := trustedProxies
	defer func() { trustedProxies = orig }()
	trustedProxies = parseTrustedProxies("") // default: private + cloudflare

	tests := []struct {
		name    string
		remote  string
		headers map[string]string
		want    string
	}{
		{
			name:    "direct public client cannot spoof",
			remote:  "203.0.113.5:4321",
			headers: map[string]string{"X-Real-IP": "6.6.6.6", "CF-Connecting-IP": "7.7.7.7"},
			want:    "203.0.113.5",
		},
		{
			name:    "private proxy is trusted",
			remote:  "172.18.0.1:4321",
			headers: map[string]string{"X-Real-IP": "6.6.6.6"},
			want:    "6.6.6.6",
		},
		{
			name:    "cloudflare edge is trusted, CF header wins",
			remote:  "104.16.1.1:443",
			headers: map[string]string{"CF-Connecting-IP": "9.9.9.9", "X-Real-IP": "6.6.6.6"},
			want:    "9.9.9.9",
		},
		{
			name:    "garbage header falls back to peer",
			remote:  "127.0.0.1:1000",
			headers: map[string]string{"CF-Connecting-IP": "not-an-ip"},
			want:    "127.0.0.1",
		},
		{
			name:   "no headers uses peer",
			remote: "198.51.100.7:2222",
			want:   "198.51.100.7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://example.com/", nil)
			r.RemoteAddr = tt.remote
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			if got := realIP(r); got != tt.want {
				t.Fatalf("realIP = %q, want %q", got, tt.want)
			}
		})
	}

	// With trust disabled, even private peers are not trusted.
	trustedProxies = parseTrustedProxies("none")
	r := httptest.NewRequest("GET", "http://example.com/", nil)
	r.RemoteAddr = "127.0.0.1:1000"
	r.Header.Set("X-Real-IP", "6.6.6.6")
	if got := realIP(r); got != "127.0.0.1" {
		t.Fatalf("realIP with trust=none = %q, want 127.0.0.1", got)
	}
}

func TestParseTrustedProxies(t *testing.T) {
	all := parseTrustedProxies("*")
	if !all.all {
		t.Fatal("* should trust all")
	}
	none := parseTrustedProxies("none")
	if none.all || len(none.nets) != 0 {
		t.Fatal("none should trust nothing")
	}
	custom := parseTrustedProxies("192.0.2.1, 10.0.0.0/8")
	if len(custom.nets) != 2 {
		t.Fatalf("custom nets = %d, want 2", len(custom.nets))
	}
}

func TestRequestScheme(t *testing.T) {
	tests := []struct {
		proto string
		want  string
	}{
		{"", "http"},
		{"http", "http"},
		{"https", "https"},
		{"HTTPS", "https"},
		{"https, http", "https"},
	}
	for _, tt := range tests {
		r := httptest.NewRequest("GET", "http://example.com/", nil)
		if tt.proto != "" {
			r.Header.Set("X-Forwarded-Proto", tt.proto)
		}
		if got := requestScheme(r); got != tt.want {
			t.Fatalf("requestScheme(X-Forwarded-Proto=%q) = %q, want %q", tt.proto, got, tt.want)
		}
	}
}
