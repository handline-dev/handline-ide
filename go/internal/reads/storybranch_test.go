package reads

import (
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team/teamtest"
)

// A story's sub-task (one opened as a draft) is committed on <assignee>-T<n>, the branch its delivery names; a plain task has none.
func TestAStorysSubTaskBranchIsNamedForItsAssignee(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "wb", "")
	key := teamtest.Key(t, e.Home, "wb")
	run(t, e, "INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(7, 'part', '"+key+"', 'p1', 1), (8, 'plain', '"+key+"', 'p1', 1)")
	run(t, e, `INSERT INTO events(ts, agent, kind, payload) VALUES(1, 'p1', 'task_new', '{"task_id": 7, "draft": true}'), (1, 'p1', 'task_new', '{"task_id": 8}')`)
	for tid, want := range map[int64]string{7: "wb-T7", 8: ""} {
		if got, err := db.StoryBranch(e.DB, tid); err != nil || got != want {
			t.Errorf("T%d: %q %v", tid, got, err)
		}
	}
}
