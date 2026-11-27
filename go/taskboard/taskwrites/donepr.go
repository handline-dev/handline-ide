package taskwrites

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/taskboard/watch"
)

// mergeWatch is the watch an agent's done waits on in place of closing task tid: its words link to exactly one pull request (or GitLab
// merge request) of the origin of the folder the roster keeps for the agent acting on the task, and carry no commit or screenshots the
// later close would lose. The merge then closes the task with its commit, and red CI, changes asked or a close give it back (package
// gates). nil for any other done, which closes as it always did. unread is why the done closes now though it links to its request: the
// origin's host is one no reader here reads (watch.UnreadHost).
func mergeWatch(q *sql.DB, home, by string, tid int64, in DoneIn) (w *db.Watch, unread string, err error) {
	if actor.IsPerson(by) || in.Words == "" || in.Commit != "" || len(in.Shots) > 0 {
		return nil, "", nil
	}
	links := watch.PullLinks(in.Words)
	if len(links) > 1 || len(links) == 0 && !strings.Contains(in.Words, "://") {
		return nil, "", nil
	}
	t, err := db.GetTask(q, tid)
	if err != nil || t == nil || t.Ended() {
		return nil, "", err
	}
	repo := team.NewRoster(home).RepoOf(t.Acting) // the folder the gate fetches when the merge comes
	if repo == "" {
		return nil, "", nil
	}
	r := gitrun.Run(repo, "remote", "get-url", "origin")
	origin := watch.OriginRepo(r.Stdout)
	if r.RC != 0 || origin == "" {
		return nil, "", nil
	}
	var linked db.Watch
	if len(links) == 1 {
		if linked, err = watch.Landing(links[0]); err != nil {
			return nil, "", nil
		}
		if pr, _, _ := watch.PullOf(linked); !watch.SameRepo(origin, pr) {
			return nil, "", nil
		}
	}
	if !watch.Reads(context.Background(), origin) {
		if host := watch.HostOf(origin); len(links) == 1 || strings.Contains(in.Words, "://"+host+"/") {
			return nil, watch.UnreadHost(host), nil
		}
		return nil, "", nil
	}
	if len(links) == 0 {
		return nil, "", nil
	}
	linked.Closes = tid
	return &linked, "", nil
}

// mergeWaitTitle is the gate's title when a done waits on its pull request's merge, as task.wait names one.
func mergeWaitTitle(tid int64, w *db.Watch) string {
	return fmt.Sprintf("T%d waits on %s, then is done", tid, w.Label)
}
