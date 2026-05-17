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
