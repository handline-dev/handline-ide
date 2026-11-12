package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWithdrawReadAndInboxAnswerWithoutPython(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(map[string]string{
		"request.withdraw": `{"accepted":true,"said":"#7 withdrawn"}`,
		"request.read": `{"id":7,"name":"msg_0102030","requester":"a.one","target":"b.two","kind":"say","state":"landed","created":"2026-10-16T00:00:00Z","parent":null,
			"chain_depth":0,"body":"hello","answer":"fine","parts":[],"refs":[{"path":"/w/a.md","folder":false},{"path":"/w/docs","folder":true}]}`,
		"inbox.queue": `{"newest":9,"head":{"id":9,"name":"msg_0405060","requester":"a.one","chain_depth":1,"body":"do it","more_waiting_line":"","how_to_close":"answer with hl say"},"history":[]}`,
	})
	if out, _, code := r.run(asAgent, "withdraw", "7", "--reason", "oops"); code != 0 || out != "#7 withdrawn\n" {
		t.Errorf("withdraw: %d %q", code, out)
	}
	out, _, code := r.run(asAgent, "read", "7")
	if code != 0 || !strings.Contains(out, "msg_0102030  a.one → b.two  say  landed") || !strings.Contains(out, "hello") ||
		!strings.Contains(out, "hello\n[file 1: /w/a.md]\n[folder 2: /w/docs]\n") || !strings.Contains(out, "--- reply from b.two ---\nfine") {
		t.Errorf("read: %d %q", code, out)
	}
	out, _, code = r.run(asAgent, "inbox")
	if code != 0 || !strings.Contains(out, "msg_0405060 from a.one  depth=1\n\ndo it\n") {
		t.Errorf("inbox: %d %q", code, out)
	}
	want := []string{`request.withdraw {"id":7,"reason":"oops"}`, `request.read {"id":7,"wait_s":0}`,
		`inbox.queue {"all":false,"whose":"a.one"}`, `inbox.mark_read {"ids":[9]}`}
	if got := sent(calls()); !reflect.DeepEqual(got, want) {
		t.Errorf("calls\n got  %v\n want %v", got, want)
	}
}

func TestReadTakesAMessageByNameOrHashAndPassesAHashMarkOnToBeRefused(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(map[string]string{"request.read": "error:#7 is PR 7; a message is named msg_<hash>"})
	// a hash may be all digits: only digits shorter than a printed name are an id
	for _, arg := range []string{"MSG_0102030", "0a1b2c3", "3412789", "39568", "#7"} {
		r.run(asAgent, "read", arg)
	}
	want := []string{`request.read {"id":"0102030","wait_s":0}`, `request.read {"id":"0a1b2c3","wait_s":0}`,
		`request.read {"id":"3412789","wait_s":0}`, `request.read {"id":39568,"wait_s":0}`, `request.read {"id":"#7","wait_s":0}`}
	if got := sent(calls()); !reflect.DeepEqual(got, want) {
		t.Errorf("calls\n got  %v\n want %v", got, want)
	}
}

func TestHrProposesThroughTheDaemonAndTakesOneBack(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(map[string]string{"proposal.propose": `{"accepted":true,"proposal":3,"said":"proposal 00000003 raised."}`,
		"proposal.withdraw": `{"accepted":true,"said":"proposal 0000000a withdrawn"}`})
	file := r.home + "/role.md"
	if err := os.WriteFile(file, []byte("the role"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"propose", "add", "c.three", "--why", "w", "--repo", "/r", "--effort", "high"},
		{"propose", "retire", "c.three", "--why", "w"},
		{"propose", "context", "b.two", "--role-file", file, "--why", "w"},
		{"propose", "withdraw", "a"},
	} {
		if out, errs, code := r.run(asAgent, args...); code != 0 || errs != "" || out == "" {
			t.Errorf("%v: %d %q %q", args, code, out, errs)
		}
	}
	want := []string{
		`proposal.propose {"args":{"effort":"high","name":"c.three","repo":"/r"},"kind":"add","why":"w"}`,
		`proposal.propose {"args":{"name":"c.three"},"kind":"retire","why":"w"}`,
		`proposal.propose {"args":{"name":"b.two","sets":{"ROLE.md":"the role"}},"kind":"context","why":"w"}`,
		`proposal.withdraw {"proposal":10}`,
	}
	if got := sent(calls()); !reflect.DeepEqual(got, want) {
		t.Errorf("calls\n got  %v\n want %v", got, want)
	}
}

func TestHrBundleSendsTheChangesAsOneProposalAndAWhyWithAMessageNumberIsRefused(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(map[string]string{"proposal.propose": `{"accepted":true,"proposal":3,"said":"proposal 00000003 raised."}`})
	file := r.home + "/changes.json"
	if err := os.WriteFile(file, []byte(`[{"kind":"retire","name":"a.one"},{"kind":"rename","name":"b.two","to":"b.dos"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, errs, code := r.run(asAgent, "propose", "bundle", "--file", file, "--why", "tidy the team"); code != 0 || errs != "" || out == "" {
		t.Errorf("bundle: %d %q %q", code, out, errs)
	}
	if _, errs, _ := r.run(asAgent, "propose", "bundle", "--file", file, "--why", "as asked in msg_8aebff7"); !strings.Contains(errs, "names msg_8aebff7") {
		t.Errorf("a message number in --why: %q", errs)
	}
	for _, args := range [][]string{
		{"propose", "bundle", "--file", file, "--why", "as asked in msg_8aebff7"},
		{"propose", "retire", "a.one", "--why", "per handline-msg 1a2b3c4d"},
		{"propose", "bundle", "--why", "w"},
		{"propose", "bundle", "--file", file},
	} {
		if _, errs, code := r.run(asAgent, args...); code != 2 || !strings.Contains(errs, "arguments not understood") {
			t.Errorf("%v: %d %q", args, code, errs)
		}
	}
	want := []string{`proposal.propose {"args":{"changes":[{"kind":"retire","name":"a.one"},{"kind":"rename","name":"b.two","to":"b.dos"}],"name":"2 changes"},"kind":"bundle","why":"tidy the team"}`}
	if got := sent(calls()); !reflect.DeepEqual(got, want) {
		t.Errorf("calls\n got  %v\n want %v", got, want)
	}
}

func TestReadWithdrawAndSayParentSendARefAsTextAndANumberAsANumber(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(map[string]string{
		"request.withdraw": `{"accepted":true,"said":"withdrawn"}`,
		"request.read": `{"id":7,"name":"msg_0102030","requester":"a.one","target":"b.two","kind":"say","state":"landed","created":"2026-10-16T00:00:00Z","parent":null,
			"relation":"peer","response":"fire_and_forget","chain_depth":0,"body":"hello","answer":null,"dismissal":null,"parts":[]}`,
		"say": `{"accepted":true,"said":"sent"}`,
	})
	r.run(asAgent, "read", "MSG_0100200")
	r.run(asAgent, "read", "7")
	r.run(asAgent, "withdraw", "123456789012")
	r.run(asAgent, "say", "b.two", "hi", "--parent", "c0ffee000001")
	want := []string{`request.read {"id":"0100200","wait_s":0}`, `request.read {"id":7,"wait_s":0}`, `request.withdraw {"id":"123456789012"}`}
	got := sent(calls())
	if len(got) < 4 || !reflect.DeepEqual(got[:3], want) || !strings.Contains(got[3], `"parent":"c0ffee000001"`) {
		t.Errorf("calls\n got  %v\n want %v then a say with the parent ref", got, want)
	}
	for _, bad := range []string{"c0ffe", "nothex", "1234567890123"} {
		if _, errs, code := r.run(asAgent, "read", bad); code != 2 || !strings.Contains(errs, "arguments not understood") {
			t.Errorf("%s: %d %q", bad, code, errs)
		}
	}
}

func TestAPortedVerbRefusesWhatItDoesNotParseInsteadOfRunningPython(t *testing.T) {
	r := newRig(t)
	r.daemon(nil)
	for _, args := range [][]string{{"say", "--bogus"}, {"task"}, {"task", "new", "--bogus"}, {"read"}, {"propose", "approve"}, {"team", "extra"}} {
		out, errs, code := r.run(asAgent, args...)
		if code != 2 || strings.Contains(out+errs, "python") || !strings.Contains(errs, "arguments not understood") {
			t.Errorf("%v: %d %q %q", args, code, out, errs)
		}
	}
}

func TestHrRenameAndTeamGoThroughTheDaemon(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(map[string]string{"proposal.propose": `{"accepted":true,"proposal":3,"said":"proposal 00000003 raised."}`})
	for _, args := range [][]string{
		{"propose", "rename", "a.one", "--to", "a.uno", "--why", "w"},
		{"propose", "team", "zed", "--member", "sea=/s", "--member", "air", "--repo", "/r", "--why", "w"},
	} {
		if out, errs, code := r.run(asAgent, args...); code != 0 || errs != "" || out == "" {
			t.Errorf("%v: %d %q %q", args, code, out, errs)
		}
	}
	want := []string{
		`proposal.propose {"args":{"name":"a.one","to":"a.uno"},"kind":"rename","why":"w"}`,
		`proposal.propose {"args":{"members":["sea","air"],"name":"zed","repo":"/r","repos":{"zed":"/r","zed.sea":"/s"}},"kind":"team","why":"w"}`,
	}
	if got := sent(calls()); !reflect.DeepEqual(got, want) {
		t.Errorf("calls\n got  %v\n want %v", got, want)
	}
	for _, args := range [][]string{{"propose", "rename", "a.one", "--why", "w"}, {"propose", "team", "zed", "--why", "w"}} {
		if _, errs, code := r.run(asAgent, args...); code != 2 || !strings.Contains(errs, "arguments not understood") {
			t.Errorf("%v: %d %q", args, code, errs)
		}
	}
}

func TestProposeUsageNamesScheduleAndTopLevelHelpSaysWhatProposeIs(t *testing.T) {
	if u, _ := usageOf("propose", nil); !strings.Contains(u[0], "schedule") {
		t.Errorf("propose usage omits schedule: %q", u[0])
	}
	_, errs, _ := newRig(t).run(nil)
	if !strings.Contains(errs, "propose a change the founder approves") {
		t.Errorf("top-level help does not say what propose is: %q", errs)
	}
}

func TestProposeListPadsTheKindColumnForTheLongestKind(t *testing.T) {
	r := newRig(t)
	r.daemon(map[string]string{"proposal.list": `{"proposals":[
		{"id":1,"kind":"schedule","agent":"a.one","state":"open","requested_by":"x"},
		{"id":2,"kind":"add","agent":"b.two","state":"open","requested_by":"x"}]}`})
	out, _, _ := r.run(nil, "propose", "list")
	for _, row := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.Contains(row, "scheduleb") || strings.Contains(row, "schedulea.") {
			t.Errorf("kind runs into the next field: %q", row)
		}
	}
	rows := strings.Split(strings.TrimSpace(out), "\n")
	if len(rows) != 2 || strings.Index(rows[0], "a.one") != strings.Index(rows[1], "b.two") {
		t.Errorf("agent column does not align:\n%s", out)
	}
}

// hl propose run asks the founder to run a command for the asking agent, in --cwd or the folder it is typed in; list shows how it runs.
func TestProposeRunSendsTheCommandForTheAskingAgentAndListShowsItsRun(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(map[string]string{"proposal.propose": `{"accepted":true,"proposal":3,"said":"proposal 00000003 raised."}`,
		"proposal.list": `{"proposals":[
		{"id":3,"kind":"run","agent":"a.one","state":"approved","requested_by":"a.one","run":{"command":"rm -r build","cwd":"/w","risks":["delete"],"state":"failed","exit":1}},
		{"id":4,"kind":"run","agent":"a.one","state":"open","requested_by":"a.one","run":{"command":"ls","cwd":"/w","risks":[]}}]}`})
	for _, args := range [][]string{
		{"propose", "run", "rm -r build", "--cwd", "/w", "--why", "frees 2 GB"},
		{"propose", "run", "ls", "--why", "look"},
	} {
		if out, errs, code := r.run(asAgent, args...); code != 0 || errs != "" || out == "" {
			t.Errorf("%v: %d %q %q", args, code, out, errs)
		}
	}
	for _, args := range [][]string{{"propose", "run", "ls"}, {"propose", "run", "--why", "w"}, {"propose", "run", "a", "b", "--why", "w"}} {
		if _, errs, code := r.run(asAgent, args...); code != 2 || !strings.Contains(errs, "arguments not understood") {
			t.Errorf("%v: %d %q", args, code, errs)
		}
	}
	root, _ := filepath.EvalSymlinks(r.root)
	want := []string{
		`proposal.propose {"args":{"command":"rm -r build","cwd":"/w","name":"a.one"},"kind":"run","why":"frees 2 GB"}`,
		`proposal.propose {"args":{"command":"ls","cwd":"` + root + `","name":"a.one"},"kind":"run","why":"look"}`,
	}
	if got := sent(calls()); !reflect.DeepEqual(got, want) {
		t.Errorf("calls\n got  %v\n want %v", got, want)
	}
	out, _, _ := r.run(nil, "propose", "list")
	if !strings.Contains(out, "failed (exit 1) in /w: rm -r build  [delete]") || !strings.Contains(out, "not started in /w: ls\n") {
		t.Errorf("list:\n%s", out)
	}
}
