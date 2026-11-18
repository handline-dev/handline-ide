package compose

import (
	"errors"
	"fmt"
	"sync"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/scheduler/lifecycle"
	"github.com/handline/handline/go/scheduler/worktreelock"
)

// deskSweep is the worktree pool's pass: on its first run every workspace folder's worktrees already there become desks as they are
// (worktreepool.Adopt), then each closed agent's desk is given back once its session has let go of it.
type deskSweep struct {
	lc      *Lifecycle
	home    string
	log     func(string, ...any)
	adopted sync.Once
}

func (d *deskSweep) Run(now int64) error {
	if d.lc.Desks == nil {
		return nil
	}
	d.adopted.Do(func() {
		roster := team.NewRoster(d.home)
		seen := map[string]bool{}
		for _, agent := range paths.ListAgents(d.home) {
			repo := lifecycle.ExpandRepo(roster.RepoOf(agent))
			if repo == "" || seen[repo] || team.CheckoutProblem(repo) != "" {
				continue
			}
			seen[repo] = true
			if n, err := d.lc.Desks.Adopt(repo); err != nil {
				d.log("  desks: %s: %v", repo, err)
			} else if n > 0 {
				d.log("  desks: %d folder(s) of %s joined the pool as they are", n, repo)
			}
		}
	})
	running := func(repo, desk string) bool { return worktreelock.Held(repo, desk, worktreelock.Running) != "" }
	var errs []error
	tell := func(agent, text string) {
		if _, err := d.lc.Env.Notice(d.lc.Env.DB, agent, text, "desk_untracked"); err != nil {
			errs = append(errs, fmt.Errorf("%s: not told of its untracked files: %w", agent, err))
		}
	}
	return errors.Join(d.lc.Desks.Sweep(now, running, tell), errors.Join(errs...))
}
