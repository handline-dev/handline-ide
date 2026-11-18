package main

import (
	"fmt"
	"slices"
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

func TestUseGivesAPortLeaseToTheCommandAsHLPort(t *testing.T) {
	r := newRig(t)
	r.daemon(map[string]string{"lease.take": `{"lease":6,"waited_s":0,"port":4123}`, "lease.release": `{"released":1}`})
	if out, _, code := r.run(asAgent, "use", "port", "--", "sh", "-c", "echo $HL_PORT"); code != 0 || out != "4123\n" {
		t.Fatalf("%q %d", out, code)
	}
	if out, _, code := r.run(asAgent, "use", "port", "--hold"); code != 0 || !strings.Contains(out, "holding port 4123 (lease 6)") {
		t.Fatalf("hold: %q %d", out, code)
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
	for _, bad := range [][]string{{"use", "add", "x", "--cmd", "y"}, {"use", "add", "x", "--why", "w"}, {"use", "remove"}, {"use", "list", "x"}, {"use", "browser"}, {"use", "--hold"}, {"use", "browser", "--hold", "--", "x"}, {"use", "--extend", "--hold"}, {"use", "--extend", "--", "x"}} {
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

func TestUseTakesSeveralKindsInTheFixedOrderAndGivesAllBack(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(map[string]string{"lease.take:heavy": `{"lease":1,"waited_s":0}`, "lease.take:port": `{"lease":2,"waited_s":0,"port":4100}`,
		"lease.release": `{"released":1}`})
	out, _, code := r.run(asAgent, "use", "port", "heavy", "port", "--", "sh", "-c", "echo $HL_PORT")
	if code != 0 || out != "4100\n" {
		t.Fatalf("%q %d", out, code)
	}
	var seen []string
	for _, c := range calls() {
		p := c["params"].(map[string]any)
		if c["method"] == "lease.take" {
			seen = append(seen, "take "+p["kind"].(string))
		} else {
			seen = append(seen, fmt.Sprintf("release %v", p["lease"]))
		}
	}
	if want := []string{"take heavy", "take port", "release 1", "release 2"}; !slices.Equal(seen, want) {
		t.Fatalf("calls %q", seen)
	}
}

// Shared runs come after the kinds, by name, so two agents asking for the same two never wait on each other.
func TestUseTakesSharedRunsAfterTheKindsByName(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(map[string]string{"lease.take": `{"lease":1,"waited_s":0}`, "lease.release": `{"released":1}`})
	if _, _, code := r.run(asAgent, "use", "zz", "port", "aa", "zz", "--", "true"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	var seen []string
	for _, c := range calls() {
		if c["method"] == "lease.take" {
			seen = append(seen, c["params"].(map[string]any)["kind"].(string))
		}
	}
	if want := []string{"port", "aa", "zz"}; !slices.Equal(seen, want) {
		t.Fatalf("asked %q", seen)
	}
}

func TestUseAddRemoveAndListSharedRuns(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(map[string]string{"run.set": `{"accepted":true,"said":"xcode now takes turns"}`,
		"run.remove": `{"accepted":true,"said":"xcode no longer takes turns"}`,
		"run.list": `{"runs":[{"name":"xcode","scope":"w3","cmds":["xcodebuild","@simulator"],"at_once":1,"held":[{"agent":"a1"}],` +
			`"waiting":[],"set_by":"a2","why":"the fans"}],"presets":["@build-test","@simulator","@browser"]}`})
	if out, _, code := r.run(asAgent, "use", "add", "xcode", "--cmd", "xcodebuild", "--cmd", "@simulator", "--at-once", "2", "--mac",
		"--why", "the fans"); code != 0 || !strings.Contains(out, "xcode now takes turns") {
		t.Fatalf("add: %q %d", out, code)
	}
	if out, _, code := r.run(asAgent, "use", "remove", "xcode"); code != 0 || !strings.Contains(out, "no longer") {
		t.Fatalf("remove: %q %d", out, code)
	}
	out, _, code := r.run(asAgent, "use", "list")
	if code != 0 || !strings.Contains(out, "xcode (workspace w3): 1 at once, xcodebuild, @simulator; 1 held, 0 waiting\n    set by a2: the fans\n") ||
		!strings.Contains(out, "Presets a run may cover: @build-test, @simulator, @browser") {
		t.Fatalf("list: %q %d", out, code)
	}
	var set, rm, list map[string]any
	for _, c := range calls() {
		switch c["method"] {
		case "run.set":
			set = c["params"].(map[string]any)
		case "run.remove":
			rm = c["params"].(map[string]any)
		case "run.list":
			list = c["params"].(map[string]any)
		}
	}
	if fmt.Sprint(set["cmds"]) != "[xcodebuild @simulator]" || set["at_once"] != float64(2) || set["scope"] != "mac" || set["why"] != "the fans" ||
		set["name"] != "xcode" {
		t.Fatalf("set %v", set)
	}
	if rm["name"] != "xcode" || rm["scope"] != "workspace" {
		t.Fatalf("remove %v", rm)
	}
	if list["agent"] == nil {
		t.Fatalf("list %v", list)
	}
}

func TestUseWhoseSecondKindWaitsGivesTheFirstBackAndExits75(t *testing.T) {
	r := newRig(t)
	why := "No browser is free: T20 now waits for one (gate T1001) and comes back by itself once one is reserved for it; then run this hl use again. Take your next task. Nothing ran."
	calls := r.daemon(map[string]string{"lease.take:simulator": `{"lease":3,"waited_s":0}`, "lease.take:browser": "error[lease_wait]:" + why,
		"lease.release": `{"released":1}`})
	out, _, code := r.run(asAgent, "use", "browser", "simulator", "--", "sh", "-c", "echo ran")
	if code != BlockedExit || out != why+"\n" {
		t.Fatalf("%q %d", out, code)
	}
	got := calls()
	if len(got) != 3 || got[2]["method"] != "lease.release" || got[2]["params"].(map[string]any)["lease"] != float64(3) {
		t.Fatalf("calls %v", got)
	}
}

func TestUseHoldsOneKindOnly(t *testing.T) {
	r := newRig(t)
	if _, _, code := r.run(asAgent, "use", "simulator", "browser", "--hold"); code != 2 {
		t.Fatalf("hold of two kinds: %d", code)
	}
}

func TestUseExtendSaysWhereEachLeaseNowRuns(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(map[string]string{"lease.extend": `{"extended":[{"lease":5,"kind":"simulator","max_hold_at":"2026-11-12T16:00:00Z"}]}`})
	out, _, code := r.run(asAgent, "use", "--extend", "simulator")
	if code != 0 || out != "lease 5 (simulator) now runs to 2026-11-12T16:00:00Z\n" {
		t.Fatalf("%q %d", out, code)
	}
	if got := calls(); len(got) != 1 || got[0]["params"].(map[string]any)["kind"] != "simulator" {
		t.Fatalf("calls %v", got)
	}
}
