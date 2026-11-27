package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitSetsUpTheHomeOnce(t *testing.T) {
	r := newRig(t)
	env := []string{"HOME=" + r.root, "HANDLINE_BIN=/x/hl"}
	out, errs, code := r.run(env, "init")
	if code != 0 || errs != "" {
		t.Fatalf("%q %q %d", out, errs, code)
	}
	for _, want := range []string{"installed Handline event hooks\n", "sandbox: agents may write under " + r.home + "\n",
		"initialized " + r.home + "\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	for _, p := range []string{"agents", "accounts", "config.yaml", "shared/.claude/skills"} {
		if _, err := os.Stat(filepath.Join(r.home, p)); err != nil {
			t.Error(err)
		}
	}
	if _, err := os.Stat(filepath.Join(r.home, ".git")); err == nil {
		t.Error("init made the home a git repo")
	}
	settings, err := os.ReadFile(filepath.Join(r.home, "shared", ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"/x/hl hook Stop"`, `"/x/hl hook PreToolUse"`, filepath.Join(r.home, "daemon.sock")} {
		if !strings.Contains(string(settings), want) {
			t.Errorf("settings lack %s:\n%s", want, settings)
		}
	}

	out, errs, code = r.run(env, "init")
	if code != 0 || errs != "" || strings.Contains(out, "sandbox:") {
		t.Fatalf("second init: %q %q %d", out, errs, code)
	}
	again, _ := os.ReadFile(filepath.Join(r.home, "shared", ".claude", "settings.json"))
	if string(again) != string(settings) {
		t.Errorf("second init changed the settings:\n%s\nthen\n%s", settings, again)
	}
}

func TestInitTakesNoArguments(t *testing.T) {
	r := newRig(t)
	out, errs, code := r.run(nil, "init", "extra")
	if code != 2 || out != "" || errs != "usage: hl init\n" {
		t.Fatalf("%q %q %d", out, errs, code)
	}
}
