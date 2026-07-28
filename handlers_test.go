package main

import "testing"

func TestSafeReturnPath(t *testing.T) {
	tests := map[string]string{
		"":                                  "/",
		"/admin":                            "/admin",
		"/admin?tab=keys":                   "/admin?tab=keys",
		"https://host/admin":                "/admin",
		"https://host/admin?tab=keys":       "/admin?tab=keys",
		"https://host/":                     "/",
		"https://host":                      "/",
		"//evil.com/x":                      "/x", // host part is dropped
		"https://host//evil.com/x":          "/",
		"https://host/\\evil.com":           "/%5Cevil.com",
		"http://[::1]:9800/admin":           "/admin",
		"not a url ::":                      "/",
		"javascript:alert(1)":               "/",
		"https://evil.com@host2//evil.com/": "/",
	}
	for ref, want := range tests {
		if got := safeReturnPath(ref); got != want {
			t.Fatalf("safeReturnPath(%q) = %q, want %q", ref, got, want)
		}
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("hello", 10); got != "hello" {
		t.Fatalf("short string changed: %q", got)
	}
	if got := truncateRunes("hello world", 5); got != "hello..." {
		t.Fatalf("truncate ascii = %q", got)
	}
	// Multi-byte characters must not be cut mid-rune.
	if got := truncateRunes("中文备注测试", 2); got != "中文..." {
		t.Fatalf("truncate cjk = %q", got)
	}
}
