package taskwrites

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/taskboard/watch"
)

// mergeWatch is the watch an agent's done waits on in place of closing task tid: its words link to exactly one pull request, of the
// GitHub origin of the folder the roster keeps for the agent acting on the task, and carry no commit or screenshots the later close would
// lose. The merge then closes the task with its commit, and red CI, changes asked or a close give it back (package gates). nil for any
// other done, which closes as it always did.
func mergeWatch(q *sql.DB, home, by string, tid int64, in DoneIn) (*db.Watch, error) {
	if actor.IsPerson(by) || in.Words == "" || in.Commit != "" || len(in.Shots) > 0 {
		return nil, nil
	}
	links := watch.PullLinks(in.Words)
	if len(links) != 1 {
		return nil, nil
	}
	t, err := db.GetTask(q, tid)
	if err != nil || t == nil || t.Ended() {
		return nil, err
	}
	repo := team.NewRoster(home).RepoOf(t.Acting) // the folder the gate fetches when the merge comes
	if repo == "" {
		return nil, nil
	}
	r := gitrun.Run(repo, "remote", "get-url", "origin")
	pr, _, _ := strings.Cut(links[0], "#")
	if r.RC != 0 || !watch.SameRepo(watch.RemoteRepo(r.Stdout), pr) {
		return nil, nil
	}
	w, err := watch.Parse("github", links[0], "landed")
	if err != nil {
		return nil, nil
	}
	w.Closes = tid
	return &w, nil
}

// mergeWaitTitle is the gate's title when a done waits on its pull request's merge, as task.wait names one.
func mergeWaitTitle(tid int64, w *db.Watch) string {
	return fmt.Sprintf("T%d waits on %s, then is done", tid, w.Label)
}
