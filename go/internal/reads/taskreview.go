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

// branchOf is a story's sub-task's branch (db.StoryBranch) and whether it exists yet in its assignee's worktree repo; nil for another task.
func (e *Env) branchOf(tid int64) (any, error) {
	name, err := db.StoryBranch(e.DB, tid)
	if err != nil || name == "" {
		return nil, err
	}
	row, err := queryOne(e.DB, "SELECT s.worktree FROM tasks t JOIN agent_state s ON s.agent = t.owner WHERE t.id = ?", tid)
	if err != nil {
		return nil, err
	}
	exists := false
	if root := worktreePath(row.str("worktree")); root != "" {
		exists, _ = git(root, "rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	}
	return map[string]any{"name": name, "exists": exists}, nil
}
