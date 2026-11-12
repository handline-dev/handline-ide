package main

import (
	"strings"
	"testing"
)

func TestUseWaitsRunsTheCommandAndGivesTheLeaseBack(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(map[string]string{"lease.take": `{"lease":4,"waited_s":75}`, "lease.release": `{"released":1}`})
	out, errs, code := r.run(asAgent, "use", "browser", "--", "sh", "-c", "echo ran; exit 3")
	if code != 3 || out != "ran\n" || !strings.Contains(errs, "waited 1m15s for a browser") {
		t.Fatalf("%q %q %d", out, errs, code)
	}
	got := calls()
	if len(got) != 2 || got[0]["method"] != "lease.take" || got[1]["method"] != "lease.release" {
		t.Fatalf("calls %v", got)
	}
	if p := got[0]["params"].(map[string]any); p["kind"] != "browser" || p["hold"] != false {
		t.Fatalf("take %v", p)
	}
	if p := got[1]["params"].(map[string]any); p["lease"] != float64(4) {
		t.Fatalf("release %v", p)
	}
}

func TestUseRefusedRunsNothing(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(map[string]string{"lease.take": `error:Refused: no browser came free in 30m0s; held by a since 10:02. Nothing ran.`})
	out, errs, code := r.run(asAgent, "use", "browser", "--", "sh", "-c", "echo ran")
	if code != 1 || out != "" || !strings.Contains(errs, "no browser came free") {
		t.Fatalf("%q %q %d", out, errs, code)
	}
	if got := calls(); len(got) != 1 {
		t.Fatalf("calls %v", got)
	}
}

func TestUseWhoseAskWaitsAsABlockerExits75AndRunsNothing(t *testing.T) {
	r := newRig(t)
	why := "No browser is free: T20 now waits for one (gate T1001) and comes back by itself once one is reserved for it; then run this hl use again. Take your next task. Nothing ran."
	calls := r.daemon(map[string]string{"lease.take": "error[lease_wait]:" + why})
	out, _, code := r.run(asAgent, "use", "browser", "--", "sh", "-c", "echo ran")
	if code != BlockedExit || out != why+"\n" {
		t.Fatalf("%q %d", out, code)
	}
	if got := calls(); len(got) != 1 || got[0]["method"] != "lease.take" {
		t.Fatalf("calls %v", got)
	}
}

func TestUseHoldAndRelease(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(map[string]string{"lease.take": `{"lease":5,"waited_s":0}`, "lease.release": `{"released":1}`})
	if out, _, code := r.run(asAgent, "use", "simulator", "--hold"); code != 0 || !strings.Contains(out, "holding a simulator (lease 5)") {
		t.Fatalf("hold: %q %d", out, code)
	}
	if out, _, code := r.run(asAgent, "use", "--release"); code != 0 || out != "gave back 1 lease(s)\n" {
		t.Fatalf("release: %q %d", out, code)
	}
	got := calls()
	if got[0]["params"].(map[string]any)["hold"] != true || got[1]["method"] != "lease.release" || got[1]["params"].(map[string]any)["kind"] != "" {
		t.Fatalf("calls %v", got)
	}
	for _, bad := range [][]string{{"use", "gpu", "--", "x"}, {"use", "browser"}, {"use", "--hold"}, {"use", "browser", "--hold", "--", "x"}} {
		if _, _, code := r.run(asAgent, bad...); code != 2 {
			t.Errorf("%v: %d", bad, code)
		}
	}
}

func TestLeaseText(t *testing.T) {
	got := leaseText(obj{"kinds": []any{
		map[string]any{"kind": "heavy", "max": float64(2), "held": []any{map[string]any{"agent": "d"}}, "queue": []any{}},
		map[string]any{"kind": "simulator", "max": float64(0), "held": []any{}, "queue": []any{}},
		map[string]any{"kind": "browser", "max": float64(1), "held": []any{map[string]any{"agent": "a"}}, "queue": []any{map[string]any{"agent": "b"}, map[string]any{"agent": "c"}}},
	}})
	if !strings.HasPrefix(got, "\ntest or build runs: 1 held of 2 (d)\nbrowsers: 1 held of 1 (a); waiting: b, c\n") || strings.Contains(got, "simulators") {
		t.Fatalf("%q", got)
	}
	if leaseText(obj{"kinds": []any{map[string]any{"kind": "browser", "max": float64(0), "held": []any{}, "queue": []any{}}}}) != "" {
		t.Fatal("nothing to say says something")
	}
}
