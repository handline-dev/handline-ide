package compose

import (
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/scheduler/lifecycle"
)

// deskSweep is the worktree pass: each workspace folder's worktrees already there join the pool as they are (worktreepool.Adopt),
// retried until that worked; then, with the setting on, each closed agent's worktree is deleted once its session has let go of it.
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
	return d.lc.Desks.Sweep(now)
}
