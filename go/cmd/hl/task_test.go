package main

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var asAgent = []string{"HANDLINE_AGENT=a.one"}

// sent is each call's method and params, without the request_id a write adds.
func sent(calls []map[string]any) []string {
	var out []string
	for _, c := range calls {
		p, _ := c["params"].(map[string]any)
		delete(p, "request_id")
		b, _ := json.Marshal(p)
		out = append(out, c["method"].(string)+" "+string(b))
	}
	return out
}

func TestEachTaskWriteSendsItsMethodAndParams(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(map[string]string{"task.new": `{"said":"opened","task":42}`})
	real, _ := filepath.EvalSymlinks(r.root)
	cases := []struct {
		stdin string
		args  []string
		want  []string
	}{
		{"", []string{"task", "new", "--assign", "b.two", "-t", "90m", "  A  title ", "  the body  ", "--parent", "T5",
			"--blocked-by=t6", "--priority", "p1", "--expects", "SHA"},
			[]string{`task.new {"blocked_by":6,"body":"the body","owner":"b.two","parent":5,"priority":1,"time_box":"5400s","title":"A  title"}`}},
		{"", []string{"task", "new", "--assign", "b.two", "-t", "1h", "x", "--request", "T9081"},
			[]string{`task.new {"owner":"b.two","request":9081,"time_box":"3600s","title":"x"}`}},
		{"from stdin\n", []string{"task", "new", "--assign", "human", "-t", "2h", "x", "--file", "-"},
			[]string{`task.new {"body":"from stdin","owner":"human","time_box":"7200s","title":"x"}`}},
		{"", []string{"task", "new", "--assign", "b.two", "-t", "1h", "x", "--owner", "human"},
			[]string{`task.new {"owner":"b.two","reporter":"human","time_box":"3600s","title":"x"}`}},
		{"", []string{"task", "new", "--assign", "b.two", "-t", "1h", "x"},
			[]string{`task.new {"owner":"b.two","time_box":"3600s","title":"x"}`}},
		{"", []string{"task", "new", "--assign", "b.two", "x"}, []string{`task.new {"owner":"b.two","title":"x"}`}},
		{"", []string{"task", "new", "--assign", "b.two", "x", "--draft", "--blocked-by", "T6", "--blocked-by", "T7", "--blocked-by", "T8"},
			[]string{`task.new {"blocked_by":[6,7,8],"draft":true,"owner":"b.two","title":"x"}`}},
		{"", []string{"task", "edit", "T3", "--title", "t", "--blocked-by", "T6", "--blocked-by", "T7"},
			[]string{`task.edit {"blocked_by":[6,7],"task":3,"title":"t"}`}},
		{"", []string{"task", "new", "--assign", "b.two", "x", "--draft"}, []string{`task.new {"draft":true,"owner":"b.two","title":"x"}`}},
		{"the plan\n", []string{"task", "plan", "T3", "--file", "-"}, []string{`task.plan {"task":3,"text":"the plan\n"}`}},
		{"", []string{"task", "plan", "T3", "--ok"}, []string{`task.plan_ok {"ok":true,"task":3}`}},
		{"", []string{"task", "plan", "T3", "--not-ok"}, []string{`task.plan_ok {"ok":false,"task":3}`}},
		{"", []string{"task", "new", "--assign", "b.two", "x", "--tag", "fe", "--tag", "be, ops"},
			[]string{`task.new {"owner":"b.two","tags":["fe","be","ops"],"title":"x"}`}},
		{"", []string{"task", "begin", "t", "--chat", "7", "--why", "w"},
			[]string{`task.begin {"expects":"report","message":7,"title":"t","why":"w"}`}},
		{"", []string{"task", "begin", "t", "--chat", "7"},
			[]string{`task.begin {"expects":"report","message":7,"title":"t"}`}},
		{"", []string{"task", "begin", "t", "--chat", "msg_0102030"},
			[]string{`task.begin {"expects":"report","message":"0102030","title":"t"}`}},
		{"", []string{"task", "handoff", "T3", "b.two", "the note"}, []string{`task.handoff {"note":"the note","task":3,"to":"b.two"}`}},
		{"", []string{"task", "handoff", "T3", "b.two", "built", "--attach", "/x/a.png", "--attach", "/x/a.mp4"},
			[]string{`task.handoff {"files":["/x/a.png","/x/a.mp4"],"note":"built","task":3,"to":"b.two"}`}},
		{"piped note", []string{"task", "handoff", "T3", "b.two"}, []string{`task.handoff {"note":"piped note","task":3,"to":"b.two"}`}},
		{"", []string{"task", "start", "T3"}, []string{`task.start {"task":3}`}},
		{"", []string{"task", "note", "T3", "hi"}, []string{`task.comment.add {"task":3,"text":"hi"}`}},
		{"", []string{"task", "comment", "T3", "add", "hi", "--now"}, []string{`task.comment.add {"now":true,"task":3,"text":"hi"}`}},
		{"", []string{"task", "comment", "T3", "add", "hi", "--attach", "/x/a.png", "--attach", "/x/b.md"},
			[]string{`task.comment.add {"files":["/x/a.png","/x/b.md"],"task":3,"text":"hi"}`}},
		{"from stdin", []string{"task", "comment", "T3", "edit", "4", "-"}, []string{`task.comment.edit {"memo":4,"task":3,"text":"from stdin"}`}},
		{"", []string{"task", "comment", "T3", "delete", "4"}, []string{`task.comment.delete {"memo":4,"task":3}`}},
		{"", []string{"task", "comment", "T3", "add", "hi"}, []string{`task.comment.add {"task":3,"text":"hi"}`}},
		{"", []string{"task", "comment", "T3", "add", "hi", "--reply", "7"}, []string{`task.comment.add {"reply":7,"task":3,"text":"hi"}`}},
		{"", []string{"task", "comment", "T3", "add", "A or B?", "--turn", "human"}, []string{`task.comment.add {"task":3,"text":"A or B?","turn":"human"}`}},
		{"", []string{"task", "comment", "T3", "add", "review", "--turn", "a.two", "--done", "PR 9"}, []string{`task.comment.add {"done":"PR 9","task":3,"text":"review","turn":"a.two"}`}},
		{"", []string{"task", "comment", "T3", "add", "log in on the mini", "--later", "human"}, []string{`task.comment.add {"later":"human","task":3,"text":"log in on the mini"}`}},
		{"", []string{"task", "queue", "T1", "T2"}, []string{`task.queue {"tasks":[1]}`, `task.queue {"tasks":[2]}`}},
		{"", []string{"task", "draft", "T1"}, []string{`task.draft {"tasks":[1]}`}},
		{"", []string{"task", "edit", "T3", "--title", "t", "--priority", "3", "--timeout", "2h", "--unblock", "T4"},
			[]string{`task.edit {"priority":3,"task":3,"time_box":"7200s","title":"t","unblock":4}`}},
		// past the daemon's limit: it is the daemon's to refuse, with the limit named, not a parse failure here
		{"", []string{"task", "edit", "T3", "--timeout", "6h"}, []string{`task.edit {"task":3,"time_box":"21600s"}`}},
		{"new brief", []string{"task", "edit", "T3", "--body", "-", "--assign", "b.two", "--owner", "a.one", "--blocked-by", "T9"},
			[]string{`task.edit {"blocked_by":9,"body":"new brief","owner":"b.two","reporter":"a.one","task":3}`}},
		{"", []string{"task", "edit", "T3", "--tag", "fe", "--untag", "be"}, []string{`task.edit {"add_tags":["fe"],"remove_tags":["be"],"task":3}`}},
		{"", []string{"task", "edit", "T3", "--pr", "#412", "--repo", "o/r"}, []string{`task.edit {"pr":412,"pr_repo":"o/r","task":3}`}},
		{"", []string{"task", "edit", "T3", "--estimate", "5"}, []string{`task.edit {"estimate":5,"task":3}`}},
		{"", []string{"task", "edit", "T3", "--parent", "T5"}, []string{`task.edit {"parent":5,"task":3}`}},
		{"", []string{"task", "edit", "T3", "--no-parent"}, []string{`task.edit {"no_parent":true,"task":3}`}},
		{"", []string{"task", "edit", "T3", "--estimate", "none"}, []string{`task.edit {"estimate":null,"task":3}`}},
		{"", []string{"task", "new", "--assign", "b.two", "--estimate", "3", "x"}, []string{`task.new {"estimate":3,"owner":"b.two","title":"x"}`}},
		{"", []string{"task", "merge", "T1", "T2", "T3"}, []string{`task.merge {"into":1,"tasks":[2,3]}`}},
		{"", []string{"task", "done", "T3", "abc", "--reason", "--repo", "./a//b/"},
			[]string{`task.done {"evidence":"abc","reason":true,"repo":"a/b","task":3}`}},
		{"merged as abc\nall green\n", []string{"task", "done", "T3", "-", "--repo", "/r"},
			[]string{`task.done {"evidence":"merged as abc\nall green","repo":"/r","task":3}`}},
		{"abc1234\n", []string{"task", "done", "T3", "-", "--repo", "/r"}, []string{`task.done {"evidence":"abc1234","repo":"/r","task":3}`}},
		{"", []string{"task", "done", "T3", "--request", "9"}, []string{`task.done {"repo":"` + real + `","request":9,"task":3}`}},
		{"", []string{"task", "done", "T3", "--request", "3412789"}, []string{`task.done {"repo":"` + real + `","request":"3412789","task":3}`}},
		{"", []string{"task", "done", "T3", "w", "--repo", "/r", "--commit", "abc1234", "--shot", "a/../b.png", "--shot=/s/c.png"},
			[]string{`task.done {"commit":"abc1234","evidence":"w","repo":"/r","shots":["` + real + `/b.png","/s/c.png"],"task":3}`}},
		{"", []string{"task", "reopen", "T3", "why"}, []string{`task.reopen {"note":"why","task":3}`}},
		{"", []string{"task", "discard", "T3", "why"}, []string{`task.discard {"reason":"why","task":3}`}},
		{"", []string{"task", "discard", "T1", "T2", "--why", "done", "--why-each", "T2=merged", "--why-each=T1=moot"},
			[]string{`task.discard {"reason":"done","reasons":{"1":"moot","2":"merged"},"tasks":[1,2]}`}},
		{"", []string{"task", "discard", "T1", "T2", "--why-each", "T1=a", "--why-each", "T2=b"},
			[]string{`task.discard {"reason":"","reasons":{"1":"a","2":"b"},"tasks":[1,2]}`}},
		{"", []string{"task", "release", "T3", "why"}, []string{`task.release {"reason":"why","task":3}`}},
		{"", []string{"task", "release", "T3"}, []string{`task.release {"task":3}`}},
		{"", []string{"task", "request", "msg_1a2b3c4", "Do it"}, []string{`task.request {"request":"1a2b3c4","title":"Do it"}`}},
		{"", []string{"task", "request", "msg_1a2b3c4", "--owner", "pc"}, []string{`task.request {"owner":"pc","request":"1a2b3c4"}`}},
		{"", []string{"task", "reopen", "T3"}, []string{`task.reopen {"task":3}`}},
		{"", []string{"task", "handoff", "T3", "b.two"}, []string{`task.handoff {"note":"","task":3,"to":"b.two"}`}},
		{"", []string{"task", "handoff", "T3", "b.two", "-"}, []string{`task.handoff {"note":"","task":3,"to":"b.two"}`}},
		{"", []string{"task", "wait", "T3", "--pr", "#412", "--repo", "o/r"}, []string{
			`task.wait {"task":3,"watch":{"target":"o/r#412","vendor":"github"}}`}},
		{"", []string{"task", "wait", "T3", "--on", "github:o/r#7", "--until", "closed", "-t", "4h"}, []string{
			`task.wait {"task":3,"time_box":"14400s","watch":{"target":"o/r#7","until":"closed","vendor":"github"}}`}},
		{"", []string{"task", "wait", "T3", "--pr", "412", "--repo", "o/r", "--until", "landed", "--then", "done"}, []string{
			`task.wait {"task":3,"then":"done","watch":{"target":"o/r#412","until":"landed","vendor":"github"}}`}},
	}
	seen := 0
	for _, c := range cases {
		out, errs, code := r.runIn(c.stdin, asAgent, c.args...)
		all := sent(calls())
		got := all[seen:]
		seen = len(all)
		if code != 0 || errs != "" || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v: exit %d, out %q, err %q\n got  %v\n want %v", c.args, code, out, errs, got, c.want)
		}
	}
}

func TestTaskWritesSayWhatTheDaemonSaid(t *testing.T) {
	r := newRig(t)
	r.daemon(map[string]string{"task.new": `{"said":"task T42 opened","task":42}`, "task.start": "error:T3 is not yours", // local-ref-ok
		"task.queue": `{"said":"queued"}`})
	if out, _, code := r.run(asAgent, "task", "new", "--assign", "b.two", "-t", "1h", "x", "y"); code != 0 || out != "task T42 opened\nT42\n" { // local-ref-ok
		t.Errorf("new: %q %d", out, code)
	}
	if out, _, code := r.run(asAgent, "task", "start", "T3"); code != 1 || out != "T3 is not yours\n" {
		t.Errorf("refused: %q %d", out, code)
	}
	if out, _, code := r.run(asAgent, "task", "queue", "T1", "T2"); code != 0 || out != "queued\nqueued\n" {
		t.Errorf("queue: %q %d", out, code)
	}
}

func TestTaskRefusalsBeforeAnyCall(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(nil)
	cases := []struct {
		env  []string
		args []string
		want string
	}{
		{asAgent, []string{"task", "new", "--owner", "b.two", "-t", "1h", "x"},
			"a task needs --assign: who works it (--owner is who answers for it, you when not given). Try:\n  hl task new --assign b.two -t 60m x\n"},
		{asAgent, []string{"task", "new", "-t", "1h", "x"},
			"a task needs --assign: who works it (--owner is who answers for it, you when not given). Try:\n  hl task new --assign <agent> -t 60m x\n"},
		{asAgent, []string{"task", "new", "--assign", "b.two", "-t", "90s", " ", "y"},
			"a task needs a title. Try:\n  hl task new --assign b.two -t 90s '<title>' y\n"},
		{asAgent, []string{"task", "done", "T3", "-"}, "evidence is required, and stdin (-) was empty\n"},
		{asAgent, []string{"task", "handoff", "T3", "b.two", "n", "--file", "f"}, "give note inline or with --file, not both\n"},
		{asAgent, []string{"task", "edit", "T3", "--owner", "b.two"}, "--owner is who answers for a task, not who works it, so it needs --assign too. To move the work:\n" +
			"  hl task edit T3 --assign b.two\nTo change only who answers for it, name the assignee it keeps:\n  hl task edit T3 --owner b.two --assign <its assignee>\n"},
		{asAgent, []string{"task", "edit", "T3"}, "edit needs a new assignee, a new owner, a new title, --body, --priority, --estimate, --blocked-by, --unblock, --timeout, --tag, --untag, --pr, --parent or --no-parent\n"},
		{asAgent, []string{"task", "comment", "T3", "add"}, "add takes the comment text\n"},
		{asAgent, []string{"task", "plan", "T3"}, "give the plan with --file PATH, or mark it with --ok or --not-ok:\n  hl task plan T3 --file plan.md\n"},
		{asAgent, []string{"task", "plan", "T3", "--ok", "--file", "p"}, "give the plan with --file PATH, or mark it with --ok or --not-ok:\n  hl task plan T3 --file plan.md\n"},
		{asAgent, []string{"task", "comment", "T3", "delete", "4", "--now"}, "--now goes with add\n"},
		{asAgent, []string{"task", "comment", "T3", "edit", "4"}, "edit takes the comment id, then the new text\n"},
		{nil, []string{"task", "start", "T3"}, "cannot tell who is start a tasking this: HANDLINE_AGENT is unset and this is not an " +
			"interactive shell. Agents are started by the daemon, which sets it.\n"},
	}
	for _, c := range cases {
		if out, _, code := r.run(c.env, c.args...); code != 1 || out != c.want {
			t.Errorf("%v: exit %d\n got  %q\n want %q", c.args, code, out, c.want)
		}
	}
	if got := calls(); len(got) != 0 {
		t.Fatalf("calls %v", got)
	}
}

func TestWhatTheTaskPortDoesNotParseIsRefused(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(nil)
	for _, args := range [][]string{
		{"task"}, {"task", "frob"}, {"task", "memo", "T1", "add", "hi"}, {"task", "show", "12"}, {"task", "show", "T1", "x"}, {"task", "show", "T1", "-h"},
		{"task", "new", "--assi", "b.two", "-t", "1h", "x"}, {"task", "new", "--assign", "b", "-t", "five", "x"},
		{"task", "new", "--assign", "b", "-t", "1h", "x", "--reply", "1", "hi"}, {"task", "new", "--assign", "b", "-t", "1h", "x", "--", "y"},
		{"task", "list", "--state", "nope"}, {"task", "begin", "x", "--chat", "a", "--why", "w"}, {"task", "begin", "x", "--why", "w"}, {"task", "begin", "x"},
		{"task", "comment", "T1", "bogus"}, {"task", "comment", "T1", "delete", "abc"}, {"task", "done", "T1", "--request", "x"},
		{"task", "edit", "T1", "--title"}, {"task", "queue"}, {"task", "merge", "T1"},
		{"task", "edit", "T1", "--estimate", "big"}, {"task", "new", "--assign", "b", "--estimate", "none", "x"},
		{"task", "edit", "T1", "--parent", "T2", "--no-parent"},
	} {
		out, errs, code := r.run(asAgent, args...)
		if code != 2 || out != "" || !strings.Contains(errs, "arguments not understood") {
			t.Errorf("%v: %q %q %d", args, out, errs, code)
		}
	}
	if got := calls(); len(got) != 0 {
		t.Fatalf("calls %v", got)
	}
}

func TestTaskShowReadsAPausedClock(t *testing.T) {
	box := obj{"length_s": float64(7200), "default": false, "left_s": float64(4800)}
	held := obj{"blockers": []any{obj{"task": float64(9)}}, "subtasks": []any{obj{"id": float64(8), "state": "doing"}, obj{"id": float64(6), "state": "done"}}}
	for _, c := range []struct {
		task obj
		want string
	}{
		{held, "timeout paused: waiting on T9, T8"},
		{obj{"subtasks": held["subtasks"]}, "timeout paused: waiting on T8"},
		{obj{}, "paused, 1h20m left"},
		{obj{"to": "human"}, "timeout paused: waiting on human"},
		{obj{"blockers": []any{obj{"task": float64(9), "owner": "human"}}}, "timeout paused: waiting on T9 (human)"},
	} {
		if got := timeoutLines(box, 0, waitsOn(c.task)); len(got) != 2 || got[0] != "timeout 2h00m" || got[1] != c.want {
			t.Errorf("%q, want %q", got, c.want)
		}
	}
	running := obj{"length_s": float64(7200), "default": false, "start": "2026-10-01T10:00:00Z", "end": "2026-10-01T12:00:00Z", "left_s": nil}
	if got := timeoutLines(running, float64(epoch(running["end"])-600), ""); !strings.HasPrefix(got[1], "ends ") || !strings.HasSuffix(got[1], "   10m left") {
		t.Errorf("a running clock: %q", got)
	}
}

func TestTaskShowListAndBoardRead(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(map[string]string{
		"task.get": `{"id":7,"title":"Fix it","state":"done","to":"b.two","from":"a.one","priority":1,"parent":3,"tags":["be","fe"],
			"created":"2026-10-01T10:00:00Z","time":{"length_s":3600,"default":false,"start":null,"end":null,"overdue_since":null,"told":0},
			"evidence":"abc123","done":"2026-10-02T11:30:00Z","discarded":null,"body":"the brief",
			"result":{"commit":"abc1234def","branch":"develop","landed":false,"shots":[{"n":1,"bytes":2048},{"n":2,"bytes":10}]},
			"memos":[{"id":5,"author":"a.one","at":"2026-10-01T12:00:00Z","text":"a comment","edited_at":null}],
			"blockers":[{"task":9,"owner":"c","title":"first"}],"subtasks":[{"id":8,"owner":"d","state":"doing","title":"child"}],
			"handoffs":[{"at":"2026-10-01T13:00:00Z","from":"a.one","to":"b.two","note":"over to you"}],
			"assignments":[{"at":"2026-10-01T14:00:00Z","by":"c","what":"owner","from":"a.one","to":"c"}]}`,
		"tasks.list": `{"items":[{"id":7,"title":"Fix it","to":"b.two","state":"doing","parent":null,"edited_by":null,"evidence":null,
			"discarded":null,"block_reason":"waiting: x","last_activity":"2026-10-01T10:00:00Z","priority":2,"asker":"a.one","tags":["fe"],
			"mismatch":"","hold_reason":"","requests":[{"id":11,"name":"msg_0000011","from":"a.one","to":"b.two","state":"queued","at":"2026-10-01T10:00:00Z"}]}],
			"more":false,"reason_gaps":{"retirements_blank":1,"retirements":4,"closures_sha":2,"closures_request":0,"closures_reason":3}}`,
		"task.board": `{"mine":[{"task":{"id":7,"title":"Fix it","to":"b.two","asker":"a.one","priority":2,"parent":3,"hold_reason":"","mismatch":""},
			"now":{"kind":"held_as_sub_task","tasks":[8],"agents":["d"]}}]}`,
	})
	show := `T7  Fix it
state done   assignee b.two   owner a.one   P1   parent T3   #be #fe   created 2026-10-01 10:00
timeout 1h00m
timeout paused: waiting on T9, T8
done 2026-10-02 11:30: abc123
commit abc1234def (not on develop)
shot ` + r.home + `/shots/T7/1.png (2048 bytes)
shot ` + r.home + `/shots/T7/2.png (10 bytes)

brief
the brief

comments
  comment 5  a.one  2026-10-01 12:00
    a comment

waiting on
  T9 (c)  first

child tasks
  T8 (d, doing)  child

handoffs
  2026-10-01 13:00  a.one -> b.two
    over to you

assignee and owner changes
  2026-10-01 14:00  owner a.one -> c, by c
`
	if out, _, code := r.run(asAgent, "task", "show", "t7"); code != 0 || out != show {
		t.Errorf("show: %d\n%s", code, out)
	}
	list := "T7    Fix it  [b.two] doing  P2  owner a.one  #fe  waiting: x  active 2026-10-01 10:00\n" +
		"        request msg_0000011 a.one → b.two queued 2026-10-01 10:00\n" +
		"reason (last 30d): 1/4 retirements carry no reason, 2 closures by sha, 0 by request, 3 by reason\n"
	if out, _, code := r.run(asAgent, "task", "list", "--assign", "b.two", "--owner", "a.one", "--tag", "fe", "--all"); code != 0 || out != list {
		t.Errorf("list: %d\n%s", code, out)
	}
	board := "T7    Fix it (under T3)\n      assignee b.two  owner a.one  now: held as sub-task T8 (d)\n"
	if out, _, code := r.run(asAgent, "task", "board"); code != 0 || out != board {
		t.Errorf("board: %d\n%q", code, out)
	}
	got := sent(calls())
	want := []string{`task.get {"task":7}`, `tasks.list {"all":true,"everyone":false,"limit":200,"owner":"b.two","reason_gaps":true,"reporter":"a.one","tags":["fe"]}`,
		`task.board {"everyone":false}`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls\n got  %v\n want %v", got, want)
	}
}

func TestTaskShowPrintsWhereTheImagesOfItsTextsAre(t *testing.T) {
	img := func(path string) obj {
		return obj{"image": obj{"id": "0123456789abcdef", "mime": "image/png", "bytes": float64(2048)}, "caption": "x", "path": path}
	}
	shot := "[image 1: /h/attachments/a.png (image/png, 2 KB)]"
	got := strings.Join(detailLines(obj{
		"id": float64(7), "title": "t", "body": "see ![x](attachment:0123456789abcdef)",
		"body_parts": []any{obj{"text": "see "}, img("/h/attachments/a.png"), obj{"image": obj{"id": "1"}, "caption": "gone", "gone": true}},
		"memos":      []any{obj{"id": float64(1), "author": "w", "text": "m", "parts": []any{img("/h/attachments/a.png")}}},
		"timeline":   []any{obj{"kind": "reject", "who": "human", "text": "no", "parts": []any{img("/h/attachments/a.png")}}},
	}, 0, ""), "\n")
	for _, want := range []string{"brief\nsee ![x](attachment:0123456789abcdef)\n" + shot + "\n", "    m\n    " + shot, "sent back\n", "    no\n    " + shot} {
		if !strings.Contains(got, want) {
			t.Fatalf("no %q in\n%s", want, got)
		}
	}
}

func TestTaskShowSaysWhatTheTaskSpentByAgentMostFirst(t *testing.T) {
	spend := obj{"usd": 4.2, "minutes": 37.4, "by_agent": obj{"b": 1.1, "a": 3.1}, "by_model": obj{"opus": 4.2}}
	if got := spendLine(obj{"spend": spend}); got != "spent about $4.20 over 37 min: a $3.10, b $1.10" {
		t.Errorf("%q", got)
	}
	if got := spendLine(obj{"spend": nil}); got != "" {
		t.Errorf("nothing charged prints nothing: %q", got)
	}
}

func TestTaskShowPrintsThePlanAndWhereItsAsksStand(t *testing.T) {
	task := obj{"id": float64(7), "title": "t", "to": "w", "state": "draft",
		"plan":     obj{"text": "change x", "by": "w", "at": "2026-11-16T10:00:00Z", "version": float64(2), "ok": obj{"by": "human", "at": "2026-11-16T11:00:00Z"}},
		"core_ask": obj{"to": "w", "at": "2026-11-16T12:00:00Z", "sent_at": nil, "note": "the parser"},
		"subtasks": []any{obj{"id": float64(8), "owner": "x", "state": "draft", "plan_state": "written", "title": "a"},
			obj{"id": float64(9), "owner": "y", "state": "draft", "plan_state": "none", "title": "b"}},
	}
	got := strings.Join(detailLines(task, 0, ""), "\n")
	for _, want := range []string{"\nplan (version 2, by w ", "; OK by human ", ")\nchange x\n", "\ncore logic asked of w ", ": the parser",
		"  T8 (x, draft, plan to read)  a", "  T9 (y, draft)  b"} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
	asked := obj{"id": float64(7), "title": "t", "to": "w", "state": "draft", "plan_ask": obj{"to": "w", "at": "2026-11-16T10:00:00Z", "sent_at": "2026-11-16T10:05:00Z"}}
	if got := strings.Join(detailLines(asked, 0, ""), "\n"); !strings.Contains(got, "plan asked of w, sent ") || !strings.Contains(got, ": being written") {
		t.Errorf("%s", got)
	}
}

func TestTaskShowOpensWithWhereItStandsNow(t *testing.T) {
	memo := func(id float64, author string) obj { return obj{"id": id, "author": author, "text": "x"} }
	open := obj{"id": float64(7), "title": "t", "to": "w", "acting": "human",
		"turns":    []any{obj{"agent": "human", "by": "w", "memo": float64(3)}},
		"memos":    []any{memo(1, "human"), memo(3, "w")},
		"subtasks": []any{obj{"id": float64(8), "owner": "x", "state": "doing"}, obj{"id": float64(9), "owner": "y", "state": "done"}},
		"blockers": []any{obj{"task": float64(5), "owner": "z"}},
	}
	got := strings.Join(detailLines(open, 0, ""), "\n")
	want := "\n\nnow\n  holder w   turn human\n  open question to human from w: comment 3\n  open children: T8 (x, doing)\n  waiting on: T5 (z)\n\nbrief"
	if !strings.Contains(got, want) {
		t.Fatalf("no %q in\n%s", want, got)
	}
	// answered: the founder's comments since the holder's last one
	answered := obj{"id": float64(7), "title": "t", "to": "w", "memos": []any{memo(1, "human"), memo(3, "w"), memo(4, "human"), memo(5, "human")}}
	got = strings.Join(detailLines(answered, 0, ""), "\n")
	if want := "now\n  holder w   turn w\n  founder since: comment 4, 5\n\nbrief"; !strings.Contains(got, want) {
		t.Fatalf("no %q in\n%s", want, got)
	}
}
