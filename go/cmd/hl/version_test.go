package main

import (
	"strings"
	"testing"
)

func TestVersionNamesHlAndTheRunningDaemon(t *testing.T) {
	r := newRig(t)
	if out, _, code := r.run(nil, "version"); code != 0 || !strings.HasPrefix(out, "hl ") || !strings.HasSuffix(out, "\nhandlined not running\n") {
		t.Fatalf("no daemon: %q %d", out, code)
	}
	r.daemon(map[string]string{"hello": `{"daemon":"d123abc"}`})
	if out, _, code := r.run(nil, "version"); code != 0 || !strings.HasSuffix(out, "\nhandlined d123abc\n") {
		t.Fatalf("daemon: %q %d", out, code)
	}
	if _, _, code := r.run(nil, "version", "x"); code != 2 {
		t.Fatalf("extra argument: %d", code)
	}
}
