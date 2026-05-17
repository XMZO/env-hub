package main

import "testing"

func TestScriptIsData(t *testing.T) {
	if !scriptIsData(`{"ip":"127.0.0.1"}`) {
		t.Fatal("json object should be data")
	}
	if !scriptIsData("\n [1,2,3]") {
		t.Fatal("json array should be data")
	}
	if scriptIsData("#!/bin/sh\necho hi") {
		t.Fatal("shell script should not be data")
	}
}

func TestScriptShell(t *testing.T) {
	tests := map[string]string{
		"#!/bin/sh\necho hi":               "sh",
		"#!/usr/bin/env bash\necho hi":     "bash",
		"\n#!/usr/local/bin/bash\necho hi": "bash",
		"echo hi":                          "sh",
	}

	for content, want := range tests {
		if got := scriptShell(content); got != want {
			t.Fatalf("scriptShell(%q) = %q, want %q", content, got, want)
		}
	}
}

func TestNormalizeScriptPath(t *testing.T) {
	valid := map[string]string{
		"ip":         "/ip",
		" /ips ":     "/ips",
		"/nodejs":    "/nodejs",
		"/dev-tools": "/dev-tools",
		"/v1.2_3":    "/v1.2_3",
	}
	for input, want := range valid {
		got, err := normalizeScriptPath(input)
		if err != nil {
			t.Fatalf("normalizeScriptPath(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Fatalf("normalizeScriptPath(%q) = %q, want %q", input, got, want)
		}
	}

	invalid := []string{
		"",
		"/",
		"/foo/bar",
		"foo/bar",
		"/foo?bar",
		"/foo#bar",
		"/foo bar",
		"/中文",
		"/admin",
		"/healthz",
		"/lang",
	}
	for _, input := range invalid {
		if got, err := normalizeScriptPath(input); err == nil {
			t.Fatalf("normalizeScriptPath(%q) = %q, want error", input, got)
		}
	}
}
