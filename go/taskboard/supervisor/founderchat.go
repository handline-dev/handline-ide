package supervisor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/textsafe"
)

// The founder's own words, natively (FounderTyped): a message the founder sends an agent is typed into its terminal as its person's prompt,
// with no envelope and no instructions. The agent answers with hl say: its terminal text never reaches the chat. Everything else still
// reaches the agent as a peer's message, so a typed prompt is always the founder's.

// typedRow is whether row is a founder's message to an agent that takes them typed: one the founder's own connection sent
// (db.FounderAuthored), never one that only names the founder as its requester.
func (e *Env) typedRow(q db.Q, row *db.Request) bool {
	return e.FounderTyped != nil && actor.IsPerson(row.Requester) && row.Kind.String == sayKind && !actor.IsPerson(row.Target) && e.FounderTyped(row.Target) &&
		must(db.FounderAuthored(q, row.ID))
}

func (e *Env) typeable(agent string) string {
	if e.Typeable == nil {
		return ""
	}
	return e.Typeable(agent)
}

// TypedOp is whether the message op to agent is typed, not sent to its inbox: an op that tells it one typed row.
func (e *Env) TypedOp(q db.Q, agent, op string) bool {
	rids := ridsOfOp(op)
	if len(rids) != 1 || e.sayingNow(rids[0]) {
		return false
	}
	row, err := db.GetRequest(q, rids[0])
	return err == nil && row != nil && row.Target == agent && e.typedRow(q, row)
}

// sayInstead marks the founder's message rid to be said through the inbox, as the agent cannot be typed at now. The send in hand reads
// the mark before it is committed, so it is noted here until done is called.
func (e *Env) sayInstead(q db.Q, rid int64) (done func()) {
	check(db.MarkFounderSaid(q, rid))
	e.init()
	e.mu.Lock()
	e.saying[rid] = true
	e.mu.Unlock()
	return func() {
		e.mu.Lock()
		delete(e.saying, rid)
		e.mu.Unlock()
	}
}

func (e *Env) sayingNow(rid int64) bool {
	e.init()
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.saying[rid]
}

// founderText is what is typed for a founder's message: the message it replies to, when it is a reply, and the words it quotes, then the
// words as written, then the paths of its pictures. A message that starts with `!` is a command typed as written: no reference line or
// quote, so the `!` stays first.
func founderText(q db.Q, row *db.Request, images string) string {
	if strings.HasPrefix(row.Body, "!") {
		return strings.TrimSpace(strings.TrimSpace(textsafe.Lines(row.Body)) + "\n" + images)
	}
	reply := inReplyTo(q, row)
	if reply != "" {
		reply = "(" + reply + ")\n"
	}
	return strings.TrimSpace(reply + quoted(row) + strings.TrimSpace(textsafe.Lines(row.Body)) + "\n" + images)
}

// quoted is the words of its parent a message quotes as a > block and a blank line, for above its body; "" with no quote.
func quoted(r *db.Request) string {
	if !r.Quote.Valid {
		return ""
	}
	return "> " + textsafe.Oneline(r.Quote.String) + "\n\n"
}

// toTemp is how the founder's message ref becomes a task a temp takes, after "To give <what> to a temp: ".
func toTemp(q db.Q, ref string) string {
	return fmt.Sprintf("`hl task request %s \"<title>\"`", db.MsgNameOf(q, ref))
}

// untaskedWindowS is how far back a founder's message is one the agent may still be acting on.
const untaskedWindowS = 3600

// selfAskReason is the refusal of agent asking itself: the command that gives its newest untasked founder message of the last hour to a
// temp, or how a temp gets work when there is none.
func selfAskReason(q db.Q, agent string, now int64) string {
	if ref := must(db.UntaskedFounderMessage(q, agent, now-untaskedWindowS)); ref != "" {
		return "an agent cannot ask itself; to give the founder's message to a temp: " + toTemp(q, ref)
	}
	return "an agent cannot ask itself; a temp takes a task someone else assigns you, or the founder's message you give it with `hl task request <msg> \"<title>\"`"
}

// besideLine is the line after a founder's message to agent while it is mid-turn or a seat of it is at work, "" otherwise: how the message
// becomes work of its own that a seat takes, the tasks its seats hold, and where a word for one goes.
func besideLine(q db.Q, agent string, row *db.Request) string {
	var held []string
	for _, seat := range must(team.TempsOf(q, agent)) {
		if tid := must(team.HiredFor(q, seat)); tid != 0 {
			title := ""
			if t := must(db.GetTask(q, tid)); t != nil {
				title = " " + strconv.Quote(textsafe.Oneline(t.Title))
			}
			held = append(held, fmt.Sprintf("T%d%s", tid, title))
		}
	}
	if st := stateOf(q, agent); len(held) == 0 && (st == nil || st.State != states.Running) {
		return ""
	}
	line := "To give this to a temp: " + toTemp(q, row.Ref) + "."
	if len(held) > 0 {
		line += fmt.Sprintf(" Your temps are on %s; a word for one goes there: `hl task comment T<n> add \"...\"`.", strings.Join(held, ", "))
	}
	return line
}
