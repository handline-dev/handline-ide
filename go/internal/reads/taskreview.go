package reads

import "github.com/handline/handline/go/data/db"

// reviewOf is TaskDetail's and a sub-task's review: how a story's sub-task stands with the founder, nil when it was never Ready.
func (e *Env) reviewOf(tid int64) (any, error) {
	r, err := db.TaskReview(e.DB, tid)
	if err != nil || r == nil {
		return nil, err
	}
	return map[string]any{"ready": r.Ready, "land_by": r.LandBy, "memo": r.Memo, "at": ts(r.At), "redo": r.Redo, "ok": r.OK}, nil
}
