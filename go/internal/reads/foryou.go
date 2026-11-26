package reads

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/internal/prefs"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
)

func forYouItem(kind, id string, agent any, title, asks string, since any) map[string]any {
	return map[string]any{"kind": kind, "id": id, "agent": agent, "title": title, "asks": asks, "since": since}
}

// ForYou is the open items waiting on the founder, oldest first, whatever each is made of, so a badge, a push and Later share one set;
// and the count of those not deferred.
func (e *Env) ForYou() ([]map[string]any, int, error) {
	var items []map[string]any
	have := map[string]bool{}
	add := func(it map[string]any) {
		key := it["kind"].(string) + "\x00" + it["id"].(string)
		if !have[key] {
			have[key] = true
			items = append(items, it)
		}
	}
	asks := func(task int64) (string, error) {
		r, err := queryOne(e.DB, "SELECT 1 AS n FROM task_tags WHERE task_id = ? AND tag = 'review'", task)
		if err != nil || r != nil {
			return "review", err
		}
		return "decide", nil
	}

	// Asks to the founder, one item for a task whatever number of requests are open on it. A say asks nothing.
	mail, err := e.mailboxRowsKept(false)
	if err != nil {
		return nil, 0, err
	}
	for _, m := range mail {
		if m.kind == "say" {
			continue
		}
		linked, err := queryOne(e.DB, "SELECT t.id, t.title FROM tasks t JOIN task_requests l ON l.task_id = t.id WHERE l.request_id = ? ORDER BY t.id LIMIT 1", m.id)
		if err != nil {
			return nil, 0, err
		}
		kind, id, title, task := "comment", m.ref, firstLine(withoutLead(m.body)), int64(0)
		if linked != nil {
			task = linked.i("id")
		}
		switch {
		case m.memo != 0:
			id = strconv.FormatInt(m.memo, 10)
		case task != 0:
			kind, id, title = "task", strconv.FormatInt(task, 10), linked.str("title")
		}
		ask := "decide"
		if task != 0 {
			if ask, err = asks(task); err != nil {
				return nil, 0, err
			}
		}
		add(forYouItem(kind, id, m.from, title, ask, ts(m.ts)))
	}

	// Tasks given to the founder that no ask above already stands for.
	b, err := e.loadBoard()
	if err != nil {
		return nil, 0, err
	}
	owned, err := query(e.DB, "SELECT *, "+taskActing+", "+
		"COALESCE((SELECT since FROM task_turns WHERE task_id = tasks.id ORDER BY pos DESC LIMIT 1), created_at) AS turn_since "+
		"FROM tasks WHERE "+db.ActingSQL("tasks")+" = ? AND "+openTask+" ORDER BY id", e.person())
	if err != nil {
		return nil, 0, err
	}
	for _, t := range owned {
		if s := b.state(t); s != "queued" && s != "doing" {
			continue
		}
		ask, err := asks(t.i("id"))
		if err != nil {
			return nil, 0, err
		}
		from := t.str("reporter")
		if from == "" {
			from = t.str("opened_by")
		}
		add(forYouItem("task", strconv.FormatInt(t.i("id"), 10), from, t.str("title"), ask, ts(t["turn_since"])))
	}

	// Drafts only the founder can run: queued work waits on them, or their agent left them behind.
	owner, err := e.personIsOwner()
	if err != nil {
		return nil, 0, err
	}
	if owner {
		runs, err := e.runItems(b)
		if err != nil {
			return nil, 0, err
		}
		for _, it := range runs {
			add(it)
		}
	}

	// The prompts agents wait on, while the screen watch still sees them; an owner answers them, as an owner types into terminals.
	var prompts []row
	if owner {
		if prompts, err = query(e.DB, "SELECT * FROM prompt_blocks WHERE gone_at IS NULL AND last_seen >= ? ORDER BY id", time.Now().Unix()-promptStaleS); err != nil {
			return nil, 0, err
		}
	}
	for _, r := range prompts {
		p := promptOf(r.str("agent"), r)
		title := "needs your answer"
		for _, k := range []string{"command", "tool", "reason"} {
			if s, _ := p[k].(string); s != "" {
				title = s
				break
			}
		}
		ask := "decide"
		if p["kind"] == "permission" {
			ask = "allow"
		}
		add(forYouItem("prompt", p["id"].(string), p["agent"], title, ask, p["since"]))
	}

	// proposals that wait for the founder (a client counts the dot from open and apply_failed, never applying).
	for _, p := range e.proposals() {
		if owner && (p.state == "open" || p.state == "apply_failed") {
			v := e.proposalView(p)
			add(forYouItem("proposal", fmt.Sprint(p.wireID), v["agent"], p.title(), "allow", ts(p.created)))
		}
	}

	// What the daemon needs and cannot do itself.
	var warnings []founderWarning
	if owner {
		if warnings, err = e.founderWarnings(); err != nil {
			return nil, 0, err
		}
	}
	for _, w := range warnings {
		add(forYouItem("machine", w.id, nil, w.title, "do", w.since))
	}

	deferred, err := prefs.LaterOf(e.DB, e.person())
	if err != nil {
		return nil, 0, err
	}
	now := time.Now()
	count := 0
	for _, it := range items {
		l, ok := deferred[it["kind"].(string)+":"+it["id"].(string)]
		if ok && !l.Over(now, e.taskIsOpen) {
			it["later"] = laterView(l)
		} else {
			count++
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if as, bs := fmt.Sprint(a["since"]), fmt.Sprint(b["since"]); as != bs {
			return as < bs
		}
		if a["kind"] != b["kind"] {
			return a["kind"].(string) < b["kind"].(string)
		}
		return a["id"].(string) < b["id"].(string)
	})
	return items, count, nil
}

// laterView is a deferral as the contract's Later.
func laterView(l prefs.Later) map[string]any {
	if l.AfterTask > 0 {
		return map[string]any{"after_task": l.AfterTask}
	}
	return map[string]any{"until": l.Until}
}

// taskIsOpen says whether the task exists and is neither done nor discarded.
func (e *Env) taskIsOpen(id int64) bool {
	r, err := queryOne(e.DB, "SELECT 1 AS n FROM tasks WHERE id = ? AND "+openTask, id)
	return err == nil && r != nil
}

// withoutLead is a comment's ask without the leading mention that addressed it (addr.Lead, as the daemon reads whom it is to).
func withoutLead(text string) string {
	t := strings.TrimSpace(text)
	if lead := addr.Lead(t); lead != "" {
		t = strings.TrimPrefix(t, addr.Ref(lead))
	}
	return strings.TrimSpace(t)
}

// personIsOwner is whether the person the reads are for is an owner: the prompts, proposals and machine needs are theirs.
func (e *Env) personIsOwner() (bool, error) {
	if e.person() == actor.OwnerKey {
		return true, nil
	}
	_, id, _ := actor.Parse(e.person())
	var role string
	err := e.DB.QueryRow("SELECT role FROM people WHERE id = ? AND removed_at IS NULL", id).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return role == "owner", err
}
