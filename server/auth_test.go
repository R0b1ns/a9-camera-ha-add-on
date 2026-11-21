package main

import (
	"strings"
	"testing"
)

func TestRenderLoginPlaceholders(t *testing.T) {
	page := renderLogin("/dev/abc/snapshot", "Wrong user")
	for _, bad := range []string{"%!", "{{NEXT}}", "{{ERR}}"} {
		if strings.Contains(page, bad) {
			t.Errorf("login page contains %q", bad)
		}
	}
	if !strings.Contains(page, `value="/dev/abc/snapshot"`) {
		t.Error("login page is missing the next URL")
	}
	if !strings.Contains(page, "Wrong user") {
		t.Error("login page is missing the error message")
	}
}
