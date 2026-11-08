package taskwrites

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/paths"
)

const founderBackKey = "founder_tasks_back"

// ReturnFounderTasks runs at daemon start, once: a task answers to the founder, it is not handed to them, so each open task an agent handed
// to them goes back to that agent (the last handoff to human, else its opener), with the founder's turn on top asking the note (else the
// task's title). A task opened by the founder or the daemon (AskFounder, a notice), or whose agent is gone, stays with the founder.
// It answers a line per task moved; one that cannot be moved is left, and the step runs again at the next start until none is left.
func ReturnFounderTasks(d *sql.DB, home string) (said []string, err error) {
	if done, _, _ := db.Meta(d, founderBackKey); done == "done" {
		return nil, nil
	}
	rows, err := d.Query("SELECT t.id, t.opened_by, t.title, " +
		"COALESCE((SELECT e.payload FROM events e WHERE e.kind = 'task_handoff' AND json_valid(e.payload) AND json_extract(e.payload, '$.task_id') = t.id " +
		"AND json_extract(e.payload, '$.to') = 'human' AND json_extract(e.payload, '$.from') NOT IN ('human', 'gate', 'daemon') ORDER BY e.id DESC LIMIT 1), '') " +
		"FROM tasks t WHERE t.owner = 'human' AND t.done_evidence IS NULL AND t.discarded_by IS NULL")
	if err != nil {
		return nil, err
	}
	type back struct {
		id                      int64
		to, note, opener, title string
	}
	var all []back
	for rows.Next() {
		var b back
		var payload string
		if err := rows.Scan(&b.id, &b.opener, &b.title, &payload); err != nil {
			rows.Close()
			return nil, err
		}
		b.to = b.opener
		if payload != "" {
			var p struct {
				From string `json:"from"`
				Note string `json:"note"`
			}
			if err := json.Unmarshal([]byte(payload), &p); err == nil && p.From != "" {
				b.to, b.note = p.From, p.Note
			}
		}
		all = append(all, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	agents := paths.ListAgents(home)
	var failed error
	left := false
	for _, b := range all {
		if !contains(agents, b.to) {
			continue // a daemon notice or a retired agent's: it stays the founder's
		}
		tx, err := d.Begin()
		if err != nil {
			return said, err
		}
		err = func() error {
			// it becomes the agent's under the founder's turn, which asks the agent's note, else the task's title: the founder still
			// acts on it, so nobody is typed at
			ask := strings.TrimSpace(b.note)
			if ask == "" {
				ask = b.title
			}
			if err := db.EditTask(tx, b.id, "daemon", db.TaskEdit{Owner: &b.to}); err != nil {
				return err
			}
			if err := db.PushTurn(tx, b.id, db.Turn{Agent: human, Ask: ask, By: b.to, Since: time.Now().Unix()}); err != nil {
				return err
			}
			_, err := db.SetTaskQueued(tx, b.id, "daemon")
			return err
		}()
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			tx.Rollback()
			failed = errors.Join(failed, fmt.Errorf("T%d: %w", b.id, err))
			left = true
			continue
		}
		said = append(said, fmt.Sprintf("T%d goes back to %s, the founder's turn on it", b.id, b.to))
	}
	if !left {
		err = db.SetMeta(d, founderBackKey, "done")
	}
	return said, errors.Join(failed, err)
}
