package reads

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/addr"
)

// founderWarning is what the daemon needs of the founder and cannot do itself, read from state, so it leaves when the state does.
type founderWarning struct {
	id, title string
	since     any
	task      int64 // the task it is about; 0 for none
	view      map[string]any
}

// founderWarnings is PendingWarning's rows.
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
	notices, err := e.founderNotices()
	if err != nil {
		return nil, err
	}
	return append(append(append(out, lost...), refused...), notices...), nil
}

// loginWarnings is each account the login watch says wants a login or stands stalled, from its rows alone: a read asks no keychain.
func (e *Env) loginWarnings() ([]founderWarning, error) {
	rows, err := query(e.DB, "SELECT name, value FROM meta WHERE name LIKE ? ORDER BY name", db.LoginNeedPrefix+"%")
	if err != nil {
		return nil, err
	}
	var out []founderWarning
	for _, r := range rows {
		a := strings.TrimPrefix(r.str("name"), db.LoginNeedPrefix)
		need, ok := db.ReadLoginNeed(r.str("value"))
		if !ok {
			continue
		}
		title := fmt.Sprintf("Account %s needs a login.", a)
		if need.Unusable != "" {
			title = fmt.Sprintf("Account %s cannot be used: %s.", a, need.Unusable)
		}
		out = append(out, founderWarning{id: "login:" + a, title: title, since: ts(need.Since), view: map[string]any{"kind": "login", "account": a}})
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
// of theirs to that agent is written and not lost too, or lost and since arrived, which is what the warning asks them to do.
func (e *Env) lostMessages() ([]founderWarning, error) {
	var out []founderWarning
	err := e.undelivered(db.LostMessagePrefix, func(id int64, u db.Undelivered) error {
		r, err := queryOne(e.DB, "SELECT r.ref, r.body FROM requests r WHERE r.id = ? AND "+db.NeverArrived("r.")+" "+
			"AND NOT EXISTS (SELECT 1 FROM requests n WHERE n.requester = r.requester AND n.target = r.target AND n.id > r.id AND n.typed_at IS NOT NULL "+
			"AND ("+db.Arrived("n.")+" OR NOT EXISTS (SELECT 1 FROM meta WHERE name = ? || n.id)))", id, db.LostMessagePrefix)
		if err != nil || r == nil {
			return err
		}
		ref := db.MsgPrefix + r.str("ref")
		out = append(out, founderWarning{id: "delivery:" + ref, since: ts(u.At), view: map[string]any{"kind": "delivery", "agent": u.Agent, "ref": ref},
			title: lostTitle(u.Agent, u.Why, head(firstLine(r.str("body")), 80))})
		return nil
	})
	return out, err
}

// lostTitle is the warning for a lost message. A ! line that was written may have run, and sending it again runs it again, so its warning says
// to look first.
func lostTitle(agent, outcome, line string) string {
	if strings.HasPrefix(line, "!") && lostWord(outcome) != "was not read" {
		return fmt.Sprintf("Your command to %s may or may not have run: look at its terminal before running it again: %s", addr.Ref(agent), line)
	}
	return fmt.Sprintf("Your message to %s %s: %s", addr.Ref(agent), lostWord(outcome), line)
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
			title: fmt.Sprintf("T%d could not be delivered to %s: %s", id, addr.Ref(u.Agent), u.Why)})
		return nil
	})
	return out, err
}

// founderNotices is each notice the board kept for the founder (db.FounderNotice), until db.FounderNoticeOver.
func (e *Env) founderNotices() ([]founderWarning, error) {
	all, err := db.FounderNotices(e.DB)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(all))
	for name := range all {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []founderWarning
	for _, name := range names {
		n := all[name]
		over, err := db.FounderNoticeOver(e.DB, n)
		if err != nil {
			return nil, err
		}
		if over {
			continue
		}
		view := map[string]any{"kind": "notice", "agent": n.Agent}
		out = append(out, founderWarning{id: strings.TrimPrefix(name, "founder_"), task: n.Task, since: ts(n.At), view: view, title: n.Text})
	}
	return out, nil
}
