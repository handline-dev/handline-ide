package supervisor

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
)

// `ask` and `say` as the daemon's own verbs (daemon/sends.py): the daemon runs the same checks and the same Ask/Say the CLI used to, writes the
// row, returns its id at once, and then - on a goroutine of its own, so the caller never waits for a wake - types the notice. Who is asking
// travels in the call (the requester, and the speaker, the caller's own name): the daemon has no environment to read it from.

const noWho = "cannot tell who is asking: the request named nobody. Agents are started by the daemon, which sets HANDLINE_AGENT."

// Handed is what a push the daemon took over answers.
const Handed = "handed to the daemon; it is typed as soon as the agent can take it"

// tell is what follows the reply: a task keeps the one-at-a-time head rule (PushHere), everything else is typed at once (TellNow).
func (e *Env) tell(rid int64, target string) {
	var out string
	defer e.guard(fmt.Sprintf("telling %s about request %d", target, rid), &out)
	q := db.Q(e.DB)
	row := must(db.GetRequest(q, rid))
	if row == nil || row.State != "queued" {
		return
	}
	if row.Kind.String == "task" {
		e.PushHere(q, target, 0)
	} else {
		must(e.TellNow(q, rid))
	}
}

// RunInline makes the telling run before the verb returns, so a test sees its effect; the daemon never does.
var RunInline = false

func (e *Env) background(name string, fn func()) {
	run := func() {
		var out string
		defer e.guard(name, &out)
		fn()
	}
	if RunInline {
		run()
		return
	}
	e.bg.Add(1)
	go func() {
		defer e.bg.Done()
		run()
	}()
}

// Wait returns once every telling started off the caller has ended: the daemon closes the database, and a test its home, only after.
func (e *Env) Wait() { e.bg.Wait() }

func (e *Env) after(rid int64, target string) {
	if rid == 0 || actor.IsPerson(target) {
		return
	}
	e.background(fmt.Sprintf("telling %s about request %d", target, rid), func() { e.tell(rid, target) })
}

// shellQuote is shlex.quote.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("@%+=:,./-_", c)) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// isReportOf: is target one of me's reports, by reports_to in the roster? An unreadable roster refuses nothing here.
func (e *Env) isReportOf(me, target string) bool {
	t, err := team.LoadTeam(e.Home)
	return err == nil && t.SupervisorOf(target) == me
}

// sayCmd is a `hl say` line ready to paste: the body as sent, quoted, with the flags that would have worked.
func sayCmd(q db.Q, target, body string, parent int64, flags ...string) string {
	parts := []string{"hl say", target, shellQuote(body)}
	if parent != 0 {
		parts = append(parts, "--parent "+db.MsgName(q, parent))
	}
	return strings.Join(append(parts, flags...), " ")
}

// SayVerbIn is the `say` verb's arguments.
type SayVerbIn struct {
	Requester string
	Speaker   *string
	Target    string
	Body      string
	Parent    int64
	// Effect is the daemon's record of the write, made in the transaction that writes the message and what is written with it.
	Effect func(q db.Q, message string, rid int64) error
}

// SayVerbOut is the verb's answer; After runs once the writes have committed (the telling).
type SayVerbOut struct {
	OK    bool
	Msg   string
	After func()
}

// TaskSayRefusal is the answer to a say that names a task: a say is chat, and what is written about a task is a comment on it.
func TaskSayRefusal(task string) string {
	return "not sent: a say is chat and carries no task. On a task, write a comment:\n" +
		"  news for the founder: hl task comment " + task + " add '@human <the news>'\n" +
		"  the founder decides:  hl task comment " + task + " add '<the decision and what you recommend>' --turn human\n" +
		"  to an agent:          hl task comment " + task + " add '@<agent> <text>'\n" +
		"A reply to the founder's message needs no task: take the work with hl task begin '<title>' --chat <msg>, or give it with hl task request <msg>."
}

// SayVerb is `hl say TARGET BODY`: a message that needs no answer, typed at once. It is chat only: what is written on a task is a comment.
func (e *Env) SayVerb(q *sql.Tx, in SayVerbIn) (out *SayVerbOut, err error) {
	defer Catch(&err)
	fail := func(msg string) (*SayVerbOut, error) { return &SayVerbOut{Msg: msg, After: func() {}}, nil }
	if in.Requester == "" {
		return fail(noWho)
	}
	me, target, body := in.Requester, in.Target, in.Body
	if actor.IsPerson(target) && !actor.IsPerson(me) {
		if named := db.MessageNamedIn(q, body); named != "" {
			return fail(fmt.Sprintf("not sent: this names a message (%s), and the founder sees no message ids - say in words what that message was about", named))
		}
	}
	if in.Parent == 0 && e.isReportOf(me, target) {
		return fail("work cannot reach a report by message: make it a task, or comment on the task it has. Try:\n  " +
			fmt.Sprintf("hl task new --assign %s -t 2h '<title>' %s\n  hl task comment T<n> add %s", target, shellQuote(body), shellQuote("@"+target+" "+body)))
	}
	within := in.Effect
	var parent *db.Request
	if in.Parent != 0 {
		parent = must(db.GetRequest(q, in.Parent))
	}
	if parent != nil && actor.IsPerson(parent.Requester) && parent.State == "queued" && parent.Target == me &&
		parent.Requester == target && parent.Kind.String == "say" {
		// an answer to the founder's question on a task joins that task
		asked := must(db.TasksForRequest(q, parent.ID))
		within = func(c db.Q, note string, rid int64) error {
			if rid != 0 {
				for _, t := range asked {
					if err := db.LinkRequest(c, t.ID, rid); err != nil {
						return err
					}
				}
			}
			if in.Effect != nil {
				return in.Effect(c, note, rid)
			}
			return nil
		}
	}
	res, serr := e.Say(q, SayIn{Target: target, Body: body, Requester: me, ParentID: in.Parent, Speaker: in.Speaker, Within: within})
	if serr != nil {
		return nil, serr
	}
	return &SayVerbOut{OK: res.OK, Msg: res.Message, After: func() {
		res.After.Run()
		if res.OK {
			e.after(res.RID, target)
		}
	}}, nil
}

// PushCheck is why target cannot be pushed to, "" for none.
func (e *Env) PushCheck(target string) string {
	if actor.IsPerson(target) || !e.registered(target) {
		return target + " is not an agent"
	}
	return ""
}

// PushAct tells target about what was queued for it: request rid when one is named (a message is typed at once, a task keeps the one-at-a-time
// rule), else its next task.
func (e *Env) PushAct(target string, rid int64) string {
	if rid != 0 {
		e.after(rid, target)
	} else {
		e.background("push-"+target, func() { e.PushHere(e.DB, target, 0) })
	}
	return Handed
}

// PushVerb is the `push` verb: (ok, message).
func (e *Env) PushVerb(target string, rid int64) (bool, string) {
	if why := e.PushCheck(target); why != "" {
		return false, why
	}
	return true, e.PushAct(target, rid)
}

// NotifyCancelledCheck is the request NotifyCancelledVerb would tell about and why not: (row, "") or (nil, why). Pure reads.
func (e *Env) NotifyCancelledCheck(q db.Q, rid int64) (row *db.Request, why string, err error) {
	defer Catch(&err)
	row = must(db.GetRequest(q, rid))
	if row == nil {
		return nil, "no such message", nil
	}
	if actor.IsPerson(row.Target) || !e.registered(row.Target) {
		return nil, row.Target + " is not an agent", nil
	}
	if row.State != "landed" || !row.DeliveredAt.Valid || !must(db.ClosedUnder(q, row.ID)) {
		return nil, fmt.Sprintf("%s was not cancelled after it was delivered", db.MsgName(q, row.ID)), nil
	}
	return row, "", nil
}

// NotifyCancelledAct types the cancel at the agent on a goroutine of its own.
func (e *Env) NotifyCancelledAct(row *db.Request) string {
	e.background(fmt.Sprintf("notify-cancelled-%d", row.ID), func() {
		if _, _, err := e.NotifyCancelled(e.DB, row, row.Reason.String); err != nil {
			e.log("  notify-cancelled-%d failed: %v", row.ID, err)
		}
	})
	return Handed
}

// NotifyCancelledVerb tells an agent a request it was handed was closed under it. Only a delivered request already closed as rejected, and only
// with the verdict on its row: what the daemon types is never the caller's words.
func (e *Env) NotifyCancelledVerb(q db.Q, rid int64) (bool, string, error) {
	row, why, err := e.NotifyCancelledCheck(q, rid)
	if err != nil || why != "" {
		return false, why, err
	}
	return true, e.NotifyCancelledAct(row), nil
}
