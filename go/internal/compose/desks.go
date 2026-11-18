package compose

import (
	"errors"
	"fmt"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/scheduler/lifecycle"
)

// deskSweep is the worktree pool's pass: each workspace folder's worktrees already there become desks as they are (worktreepool.Adopt),
// retried until that worked, and renamed desk-<n> once free (worktreepool.Rename); then each closed agent's desk is given back once its
// session has let go of it.
type deskSweep struct {
	lc      *Lifecycle
	home    string
	log     func(string, ...any)
	adopted map[string]bool
}

func (d *deskSweep) Run(now int64) error {
	if d.lc.Desks == nil {
		return nil
	}
	if d.adopted == nil {
		d.adopted = map[string]bool{}
	}
	roster := team.NewRoster(d.home)
	for _, agent := range paths.ListAgents(d.home) {
		repo := lifecycle.ExpandRepo(roster.RepoOf(agent))
		if repo == "" || d.adopted[repo] || team.CheckoutProblem(repo) != "" {
			continue
		}
		n, err := d.lc.Desks.Adopt(repo)
		if err != nil {
			d.log("  desks: %s: %v", repo, err)
			continue
		}
		d.adopted[repo] = true
		if n > 0 {
			d.log("  desks: %d folder(s) of %s joined the pool as they are", n, repo)
		}
	}
	for repo := range d.adopted {
		moved, err := d.lc.Desks.Rename(repo)
		for _, m := range moved {
			d.log("  desks: %s is now %s", m[0], m[1])
		}
		if err != nil {
			d.log("  desks: %s: %v", repo, err)
		}
	}
	var errs []error
	tell := func(agent, text string) {
		if _, err := d.lc.Env.Notice(d.lc.Env.DB, agent, text, "desk_untracked"); err != nil {
			errs = append(errs, fmt.Errorf("%s: not told of its untracked files: %w", agent, err))
		}
	}
	return errors.Join(d.lc.Desks.Sweep(now, tell), errors.Join(errs...))
}
