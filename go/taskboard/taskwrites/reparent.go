package taskwrites

import (
	"database/sql"
	"fmt"

	"github.com/handline/handline/go/kit/pyjson"
)

// reparented is a move setParent made: its message, the holders whose turn the move changes, and the old and new parent's owners.
type reparented struct {
	msg    string
	freed  []string
	owners []string
}

// setParent moves tid under parent (0: to the top): the old parent stops waiting on it, and an open new parent waits on it while it is
// open. It answers the refusal ("" when it moved) and the move.
func setParent(tx *sql.Tx, tid, parent int64, by string) (why string, m reparented, err error) {
	t, err := getTask(tx, tid)
	if err != nil || t == nil {
		return fmt.Sprintf("no task T%d", tid), m, err
	}
	if t.Parent.Int64 == parent {
		return fmt.Sprintf("task T%d already sits there - nothing to move", tid), m, nil
	}
	var p *task
	if parent != 0 {
		if parent == tid {
			return fmt.Sprintf("task T%d cannot be its own parent", tid), m, nil
		}
		if p, err = getTask(tx, parent); err != nil || p == nil {
			return fmt.Sprintf("no task T%d", parent), m, err
		}
		if p.DiscardedBy.Valid && p.DiscardedBy.String != "" {
			return fmt.Sprintf("task T%d is discarded - a task cannot go under it", parent), m, nil
		}
		if under, err := isUnder(tx, parent, tid); err != nil || under {
			return fmt.Sprintf("task T%d cannot go under T%d: T%d sits under it", tid, parent, parent), m, err
		}
	}
	if t.Parent.Valid {
		if old, err := getTask(tx, t.Parent.Int64); err != nil {
			return "", m, err
		} else if old != nil {
			m.owners = append(m.owners, old.Owner)
			if why, err := unblock(tx, old.ID, tid, by); err != nil {
				return "", m, err
			} else if why == "" { // it waited on tid; with nothing else to wait on, its holder may take it up again
				if left, err := taskBlockers(tx, old.ID); err != nil {
					return "", m, err
				} else if len(left) == 0 && old.Acting != "" {
					m.freed = append(m.freed, old.Acting)
				}
			}
		}
	}
	if p != nil {
		m.owners = append(m.owners, p.Owner)
	}
	if p != nil && !p.done() && !t.ended() {
		if why, err := recordBlock(tx, parent, tid, by); err != nil || why != "" {
			return why, m, err
		}
		if holder, err := freeHolder(tx, parent); err != nil {
			return "", m, err
		} else if holder != "" {
			m.freed = append(m.freed, holder)
		}
	}
	newParent := sql.NullInt64{Int64: parent, Valid: parent != 0}
	if _, err := tx.Exec("UPDATE tasks SET parent_task = ? WHERE id = ?", newParent, tid); err != nil {
		return "", m, err
	}
	old, now := parentRef(t.Parent), parentRef(newParent)
	if err := addEvent(tx, by, "edit_task", kv{"task_id", tid}, kv{"parent", pyjson.Pairs{{K: "old", V: old}, {K: "new", V: now}}}); err != nil {
		return "", m, err
	}
	if m.msg = fmt.Sprintf("task T%d moved under %s", tid, now); now == "" {
		m.msg = fmt.Sprintf("task T%d moved from under %s to the top", tid, old)
	}
	return "", m, nil
}

// parentRef is a parent as an edit records it: T<n>, "" for none.
func parentRef(p sql.NullInt64) string {
	if !p.Valid {
		return ""
	}
	return fmt.Sprintf("T%d", p.Int64)
}
