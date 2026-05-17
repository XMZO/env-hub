package main

import (
	"strings"
	"testing"
)

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

func TestLimitSimulationOutput(t *testing.T) {
	short := "hello"
	if got := limitSimulationOutput(short); got != short {
		t.Fatalf("short output changed: %q", got)
	}

	long := strings.Repeat("x", 20*1024+1)
	got := limitSimulationOutput(long)
	if len(got) <= 20*1024 {
		t.Fatal("truncated output should include marker")
	}
	if !strings.Contains(got, "output truncated") {
		t.Fatal("truncated output should include marker")
	}
}

func TestScriptSimulationSandboxSummary(t *testing.T) {
	offline := scriptSimulationSandboxSummary(false)
	if !strings.Contains(offline, "network=disabled") {
		t.Fatalf("offline summary missing network status: %q", offline)
	}
	if !strings.Contains(offline, "read-only rootfs") || !strings.Contains(offline, "cap-drop=ALL") {
		t.Fatalf("offline summary missing hardening: %q", offline)
	}

	online := scriptSimulationSandboxSummary(true)
	if !strings.Contains(online, "network=enabled") {
		t.Fatalf("online summary missing network status: %q", online)
	}
	if !strings.Contains(online, "no host mounts") || !strings.Contains(online, "no-new-privileges") {
		t.Fatalf("online summary missing hardening: %q", online)
	}
}
