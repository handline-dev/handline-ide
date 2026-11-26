package reads

import "github.com/handline/handline/go/data/db"

// openedGatesMax is how many opened gates a task shows: the newest ones, which say why its last waits ended.
const openedGatesMax = 5

// openedGates is TaskDetail.gates: the gates task tid waited on that have opened, newest first, each with what it watched and the line
// it opened with. blockers holds only what still blocks, and the notice that told the holder a gate opened is in its chat, not on the
// task; so the task itself keeps why a wait ended (the checks that failed, changes requested) for whoever holds it next.
func (e *Env) openedGates(tid int64) ([]any, error) {
	// A gate someone marked done by hand has no result: its watch did not open it, and it is left out.
	rows, err := query(e.DB, "SELECT g.id, g.watch_label, g.watch_result, g.done_evidence, g.done_at FROM tasks g "+
		"JOIN task_blocks b ON b.blocks_on = g.id WHERE b.task_id = ? AND g.owner = ? AND g.watch_vendor IS NOT NULL AND g.watch_result IS NOT NULL "+
		"AND g.done_evidence IS NOT NULL AND g.discarded_by IS NULL ORDER BY g.done_at DESC, g.id DESC LIMIT ?", tid, db.Gate, openedGatesMax)
	if err != nil {
		return nil, err
	}
	out := []any{}
	for _, g := range rows {
		out = append(out, map[string]any{"task": g.i("id"), "label": g.str("watch_label"), "result": g.str("watch_result"),
			"evidence": g.str("done_evidence"), "at": ts(g["done_at"])})
	}
	return out, nil
}
