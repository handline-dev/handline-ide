package reads

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/addr"
)

// founderWarning is what the daemon needs of the founder and cannot do itself, read from state, so it leaves when the state does.
type founderWarning struct {
	id, title string
	since     any
	task      int64  // the task it is about; 0 for none
	worse     string // login: why the account cannot be used, so a warning grown worse is not hidden by its dismissal
	view      map[string]any
	// review: a step-down offer at a level row under a model, with its looks there; Not now hides it for db.StepAfter more looks
	review bool
	level  int64
	model  string
	looks  int
	// dismissKey, when set, is what a dismissal keeps in place of the since (see sinceKey)
	dismissKey string
}

// founderWarnings is PendingWarning's rows, less those the founder dismissed (warning.dismiss).
func (e *Env) founderWarnings() ([]founderWarning, error) {
	all, err := e.allFounderWarnings()
	if err != nil {
		return nil, err
	}
	dismissed, err := e.dismissed()
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(all, func(w founderWarning) bool {
		kept, ok := dismissed[w.id]
		if !ok || !w.review {
			return ok && kept == sinceKey(w)
		}
		at := strings.SplitN(kept, " ", 4) // review <level row> <looks> <model>
		if len(at) != 4 || at[0] != "review" || at[1] != strconv.FormatInt(w.level, 10) || at[3] != w.model {
			return false
		}
		looks, err := strconv.Atoi(at[2])
		return err == nil && w.looks < looks+db.StepAfter
	}), nil
}

// dismissed is the since of each dismissed warning, by its id.
func (e *Env) dismissed() (map[string]string, error) {
	rows, err := query(e.DB, "SELECT name, value FROM meta WHERE name LIKE ?", db.DismissedPrefix+"%")
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, r := range rows {
		out[strings.TrimPrefix(r.str("name"), db.DismissedPrefix)] = r.str("value")
	}
	return out, nil
}

// sinceKey is what a dismissal keeps of the warning it hides: its since, and how bad it is; for a review notice, its level and looks.
func sinceKey(w founderWarning) string {
	if w.dismissKey != "" {
		return w.dismissKey
	}
	if w.review {
		return fmt.Sprintf("review %d %d %s", w.level, w.looks, w.model)
	}
	return fmt.Sprint(w.since, " ", w.worse)
}

// WarningSinces is the since of each warning the founder has now, dismissed or not, as a dismissal keeps it, by id.
func (e *Env) WarningSinces() (map[string]string, error) {
	all, err := e.allFounderWarnings()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(all))
	for _, w := range all {
		out[w.id] = sinceKey(w)
	}
	return out, nil
}

// allFounderWarnings is every warning the state holds now.
func (e *Env) allFounderWarnings() ([]founderWarning, error) {
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
	reviews, err := e.reviewNotices()
	if err != nil {
		return nil, err
	}
	return append(append(append(append(out, lost...), refused...), notices...), reviews...), nil
}

// reviewNotices is, for each agent and kind of work whose record says it has learned enough (db.LearnedEnough), the founder's offer to
// look at it one step less: Yes is agent.review_set with the offered level, Not now its dismissal. Only the founder lowers a level.
func (e *Env) reviewNotices() ([]founderWarning, error) {
	agents, err := query(e.DB, "SELECT DISTINCT agent FROM events WHERE kind = ? ORDER BY agent", db.ReadyKind)
	if err != nil {
		return nil, err
	}
	roster := team.NewRoster(e.Home)
	var out []founderWarning
	for _, r := range agents {
		a := r.str("agent")
		model, err := roster.Model(a)
		if err != nil {
			return nil, err
		}
		works, err := db.AgentWorks(e.DB, a)
		if err != nil {
			return nil, err
		}
		for _, w := range works {
			l, err := db.ReviewLevel(e.DB, a, w, model)
			if err != nil {
				return nil, err
			}
			rec, err := db.ReviewRecord(e.DB, a, model, l)
			if err != nil {
				return nil, err
			}
			if yes, _ := db.LearnedEnough(l, rec); !yes {
				continue
			}
			next := db.LevelDown(l.Level)
			line := fmt.Sprintf("Its last %d %s results were OK at first look.", db.StepAfter, w)
			out = append(out, founderWarning{id: "review:" + a + ":" + w, title: fmt.Sprintf("%s's %s results: look at %s?", addr.Ref(a), w, levelWords[next]),
				since: ts(rec[len(rec)-1].At), review: true, level: l.Row, model: model, looks: len(rec),
				view: map[string]any{"kind": "review", "agent": a, "work": w, "line": line, "level": next}})
		}
	}
	return out, nil
}

// levelWords are the levels as the founder reads them.
var levelWords = map[string]string{db.LevelEvery: "every one", db.LevelOneInTwo: "1 in 2", db.LevelOneInThree: "1 in 3",
	db.LevelOneInFive: "1 in 5", db.LevelNever: "none"}

// loginWarnings is one row per account the login watch says wants a login or stands stalled, or whose agents have no other account
// to go to (db.NoRoom), from their rows alone: a read asks no keychain. Each leaves when the daemon clears its rows.
func (e *Env) loginWarnings() ([]founderWarning, error) {
	rows, err := query(e.DB, "SELECT name, value FROM meta WHERE name LIKE ? ORDER BY name", db.LoginNeedPrefix+"%")
	if err != nil {
		return nil, err
	}
	rooms, err := db.NoRooms(e.DB)
	if err != nil {
		return nil, err
	}
	needs := map[string]db.LoginNeed{}
	for _, r := range rows {
		if need, ok := db.ReadLoginNeed(r.str("value")); ok {
			needs[strings.TrimPrefix(r.str("name"), db.LoginNeedPrefix)] = need
		}
	}
	accts := make([]string, 0, len(needs)+len(rooms))
	for a := range needs {
		accts = append(accts, a)
	}
	for a := range rooms {
		if _, ok := needs[a]; !ok {
			accts = append(accts, a)
		}
	}
	sort.Strings(accts)
	out := make([]founderWarning, 0, len(accts))
	for _, a := range accts {
		need, hasNeed := needs[a]
		room, hasRoom := rooms[a]
		title, since, worse := accountTrouble(a, need, hasNeed, room), need.Since, need.Unusable
		if !hasNeed {
			since = room.Since
		}
		if hasRoom {
			worse = strings.TrimSpace(worse + " noroom:" + room.Kind)
		}
		out = append(out, founderWarning{id: "login:" + a, title: title, since: ts(since), worse: worse, view: map[string]any{"kind": "login", "account": a}})
	}
	return out, nil
}

// accountTrouble is the line for an account's warning: what is wrong with it, and where its agents stand when no other account has room.
func accountTrouble(a string, need db.LoginNeed, hasNeed bool, room db.NoRoom) string {
	title := fmt.Sprintf("Account %s cannot be used.", a)
	switch {
	case hasNeed && need.Unusable != "":
		title = fmt.Sprintf("Account %s cannot be used: %s.", a, need.Unusable)
	case hasNeed:
		title = fmt.Sprintf("Account %s needs a login.", a)
	case room.Kind == "ended":
		title = fmt.Sprintf("Account %s's subscription has ended.", a)
	case room.Kind == "paused":
		title = fmt.Sprintf("Account %s is paused.", a)
	}
	switch room.Kind {
	case "":
		return title
	case "login":
		return fmt.Sprintf("%s Its agents are stopped: no other account has room (%s).", title, room.Room)
	}
	return fmt.Sprintf("%s Its agents stay on it: no other account has room (%s).", title, room.Room)
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
		w := founderWarning{id: strings.TrimPrefix(name, "founder_"), task: n.Task, since: ts(n.At), view: view, title: n.Text}
		if n.Task == 0 && n.Reason == "start_stall" {
			// The daemon's retry at its start clears the stall and a failed retry raises a new notice: a dismissal holds until the agent
			// has really started a session since.
			run, _, err := db.LastEventID(e.DB, n.Agent, "SessionStart")
			if err != nil {
				return nil, err
			}
			w.dismissKey = fmt.Sprint("start_stall after session ", run)
			view["stall"] = true
		}
		out = append(out, w)
	}
	return out, nil
}
