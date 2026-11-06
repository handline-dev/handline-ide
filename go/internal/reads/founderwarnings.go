package reads

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
)

// founderWarning is what the daemon needs of the founder and cannot do itself, read from state, so it leaves when the state does.
type founderWarning struct {
	id, title string
	since     any
	task      int64 // the task it is about; 0 for none
	view      map[string]any
}

// founderWarnings is PendingWarning's rows but the spare notice, which is a task of its own.
func (e *Env) founderWarnings() ([]founderWarning, error) {
	var out []founderWarning
	for _, n := range e.machineNeeds() {
		need := n.(map[string]any)
		kind := need["kind"].(string)
		out = append(out, founderWarning{id: kind, title: need["text"].(string), since: need["since"], view: map[string]any{"kind": kind}})
	}
	logins, err := e.loginWarnings()
	if err != nil {
		return nil, err
	}
	out = append(out, logins...)
	lost, err := e.lostMessages()
	if err != nil {
		return nil, err
	}
	refused, err := e.refusedTasks()
	if err != nil {
		return nil, err
	}
	return append(append(out, lost...), refused...), nil
}

// loginWarnings is each account the login watch says wants a login or stands stalled.
func (e *Env) loginWarnings() ([]founderWarning, error) {
	rows, err := query(e.DB, "SELECT name, value FROM meta WHERE name LIKE ? ORDER BY name", db.LoginNeedPrefix+"%")
	if err != nil {
		return nil, err
	}
	var out []founderWarning
	for _, r := range rows {
		a := strings.TrimPrefix(r.str("name"), db.LoginNeedPrefix)
		since, _ := strconv.ParseInt(r.str("value"), 10, 64)
		title := fmt.Sprintf("Account %s needs a login.", a)
		if s := (team.Accounts{Home: e.Home}).Stalled(a); s != nil {
			if word := team.StallWord(s.Error); word != team.StallWord("") {
				title = fmt.Sprintf("Account %s cannot be used: %s.", a, word)
			}
		}
		out = append(out, founderWarning{id: "login:" + a, title: title, since: ts(since), view: map[string]any{"kind": "login", "account": a}})
	}
	return out, nil
}

func (e *Env) undelivered(prefix string, each func(id int64, u db.Undelivered) error) error {
	names, err := query(e.DB, "SELECT name, value FROM meta WHERE name LIKE ? ORDER BY name", prefix+"%")
	if err != nil {
		return err
	}
	for _, n := range names {
		var u db.Undelivered
		id, perr := strconv.ParseInt(strings.TrimPrefix(n.str("name"), prefix), 10, 64)
		if perr != nil || json.Unmarshal([]byte(n.str("value")), &u) != nil {
			continue
		}
		if err := each(id, u); err != nil {
			return err
		}
	}
	return nil
}

// lostMessages is each message of the founder's that was never seen to arrive: over once it is (or it is withdrawn), or once a later message
// of theirs to that agent is written and not lost too, which is what the warning asks them to do.
func (e *Env) lostMessages() ([]founderWarning, error) {
	var out []founderWarning
	err := e.undelivered(db.LostMessagePrefix, func(id int64, u db.Undelivered) error {
		r, err := queryOne(e.DB, "SELECT r.ref, r.body FROM requests r WHERE r.id = ? AND "+db.NeverArrived("r.")+" "+
			"AND NOT EXISTS (SELECT 1 FROM requests n WHERE n.requester = r.requester AND n.target = r.target AND n.id > r.id AND n.typed_at IS NOT NULL "+
			"AND NOT EXISTS (SELECT 1 FROM meta WHERE name = ? || n.id))", id, db.LostMessagePrefix)
		if err != nil || r == nil {
			return err
		}
		ref := db.MsgPrefix + r.str("ref")
		out = append(out, founderWarning{id: "delivery:" + ref, since: ts(u.At), view: map[string]any{"kind": "delivery", "agent": u.Agent, "ref": ref},
			title: fmt.Sprintf("Your message to %s %s: %s", u.Agent, lostWord(u.Why), head(firstLine(r.str("body")), 80))})
		return nil
	})
	return out, err
}

// lostWord is the delivery's outcome cut to what the founder can act on.
func lostWord(outcome string) string {
	if strings.HasPrefix(outcome, "was not written") {
		return "was not read"
	}
	return "may not have been read"
}

// refusedTasks is each task queued for an agent with no lead whose delivery was refused: over once it is delivered, given to another or ended.
func (e *Env) refusedTasks() ([]founderWarning, error) {
	var out []founderWarning
	err := e.undelivered(db.RefusedTaskPrefix, func(id int64, u db.Undelivered) error {
		t, err := queryOne(e.DB, "SELECT t.title FROM tasks t WHERE t.id = ? AND "+openTask+" AND "+db.ActingSQL("t")+" = ? AND NOT EXISTS "+
			"(SELECT 1 FROM task_requests l JOIN requests r ON r.id = l.request_id WHERE l.task_id = t.id AND r.target = ? AND r.kind = 'task' AND r.ts >= ?)", id, u.Agent, u.Agent, u.At)
		if err != nil || t == nil {
			return err
		}
		out = append(out, founderWarning{id: fmt.Sprintf("undelivered:%d", id), task: id, since: ts(u.At), view: map[string]any{"kind": "undelivered", "agent": u.Agent},
			title: fmt.Sprintf("T%d could not be delivered to %s: %s", id, u.Agent, u.Why)})
		return nil
	})
	return out, err
}
