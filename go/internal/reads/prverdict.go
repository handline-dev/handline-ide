package reads

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/taskboard/watch"
)

// unreadMax is how much of what gh said is shown when a pull request could not be read.
const unreadMax = 200

const (
	branchPRWait   = 20 * time.Second
	branchPRAtOnce = 8
)

// reader is the reader of repo's pull requests (watch.ReaderOf): the daemon's, which the gates read through too, unless the Env was given
// its own; nil for a host none reads.
func (e *Env) reader(ctx context.Context, repo string) watch.Reader {
	gh, gl := e.GitHub, e.GitLab
	if gh == nil {
		gh, _ = watch.Vendors["github"].(*watch.GitHub)
	}
	if gl == nil {
		gl, _ = watch.Vendors["gitlab"].(*watch.GitLab)
	}
	return watch.ReaderOf(ctx, repo, gh, gl)
}

// pullVerdict is the worktrees.list `pr` of a pull request: its number, url and what holds it. A check is failing by its latest run, as
// a gate counts it: a failed run that was run again and passed holds nothing.
func pullVerdict(p *watch.Pull) map[string]any {
	failing := len(p.Failed())
	out := map[string]any{"number": p.Number, "url": p.URL}
	switch {
	case p.State == watch.PullMerged:
		out["verdict"] = "merged"
	case p.Draft:
		out["verdict"] = "draft"
	case failing > 0:
		out["verdict"], out["failing"] = "checks_failing", failing
	case p.Review == watch.ReviewChanges:
		out["verdict"] = "changes_requested"
	case p.Review == watch.ReviewRequired:
		out["verdict"] = "waiting_review"
	default:
		out["verdict"] = "ready"
	}
	return out
}

// attachPRs sets "pr" on each worktree row (as worktrees.list builds them): its branch's pull request, nil for none, for a branch that is
// its base, for a repo on a host no reader reads and for a reader that cannot answer. Lookups run a few at a time under one wait. It
// answers why gh cannot be used on this machine (the reason and the words), "" when it can or when no lookup failed.
func (e *Env) attachPRs(rows []map[string]any) (reason, why string) {
	ctx, cancel := context.WithTimeout(context.Background(), branchPRWait)
	defer cancel()
	var wg sync.WaitGroup
	var failed sync.Once
	slots := make(chan struct{}, branchPRAtOnce)
	for _, row := range rows {
		row["pr"] = nil
		branch, _ := row["branch"].(string)
		base, _ := row["base"].(string)
		path, _ := row["path"].(string)
		if branch == "" || branch == "HEAD" || branch == base {
			continue
		}
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer func() { <-slots; wg.Done() }()
			ok, url := git(path, "remote", "get-url", "origin")
			repo := watch.OriginRepo(url)
			if !ok || repo == "" {
				return
			}
			r := e.reader(ctx, repo)
			if r == nil {
				return
			}
			pr, err := r.BranchPull(ctx, repo, branch)
			if err != nil {
				if gh, isGH := r.(*watch.GitHub); isGH {
					failed.Do(func() { reason, why = gh.Unready() })
				}
				return
			}
			if pr != nil { // a nil map in the row would not be a JSON null to every encoder
				row["pr"] = pullVerdict(pr)
			}
		}()
	}
	wg.Wait()
	return reason, why
}

// unavailable is why a pull request's facts are not shown: a reason a screen can switch on, and words a person can act on.
func unavailable(reason, text string) map[string]any {
	return map[string]any{"reason": reason, "text": text}
}

// taskPR is task.pr: the task's pull request as its host has it now, read with this machine's gh login (glab's for a GitLab host) and
// kept about a minute; null for a task that names none. What could not be read is said (unavailable), never left as an empty card.
func (e *Env) taskPR(_ *rpc.Conn, params map[string]any) (any, error) {
	tid, err := optInt(params, "task")
	if err != nil || tid == nil {
		return nil, badParam("task is a task number", "task")
	}
	t, err := queryOne(e.DB, "SELECT pr, COALESCE(pr_repo, '') AS pr_repo, "+db.ActingSQL("tasks")+" AS acting FROM tasks WHERE id = ?", *tid)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, noTask(*tid)
	}
	if !t.set("pr") {
		return nil, nil
	}
	number, repo := t.i("pr"), t.str("pr_repo")
	if repo == "" { // a bare number is of the repository the task's holder works in
		repo = originOf(e.repoOf(t.str("acting")))
	}
	// an open gate the task still waits on, asked to close it when the pull request merges (task.wait then done)
	closes, err := queryOne(e.DB, "SELECT g.id FROM tasks g JOIN task_blocks b ON b.blocks_on = g.id AND b.task_id = g.watch_closes AND b.dormant = 0 "+
		"WHERE g.watch_closes = ? AND g.owner = ? AND g.done_evidence IS NULL AND g.discarded_by IS NULL LIMIT 1", *tid, db.Gate)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"number": number, "repo": nil, "url": nil, "state": nil, "draft": false, "base": nil, "review": nil,
		"failing": []string{}, "waiting": 0, "passing": 0, "head_sha": nil, "merge_sha": nil, "closes_task": closes != nil, "unavailable": nil,
		"host": nil}
	if repo == "" {
		out["unavailable"] = unavailable("no_repo", fmt.Sprintf("The repository of this pull request is not known on this machine. "+
			"Name it with: hl task edit T%d --pr %d --repo owner/name", *tid, number))
		return out, nil
	}
	out["repo"] = repo
	ctx, cancel := context.WithTimeout(context.Background(), branchPRWait)
	defer cancel()
	r := e.reader(ctx, repo)
	if r == nil {
		out["host"], out["unavailable"] = watch.HostOf(repo), unavailable("unread_host", watch.UnreadHost(watch.HostOf(repo)))
		return out, nil
	}
	out["host"], out["url"] = r.Name(), pullURL(repo, number)
	p, err := r.Pull(ctx, repo, number)
	if err != nil {
		out["unavailable"] = unreadPull(r, err)
		return out, nil
	}
	out["url"], out["state"], out["draft"], out["failing"] = p.URL, p.State, p.Draft, append([]string{}, p.Failed()...)
	out["waiting"], out["passing"] = p.Count(watch.CheckWaiting), p.Count(watch.CheckPassed)
	for key, v := range map[string]string{"base": p.Base, "review": p.Review, "head_sha": p.HeadSha, "merge_sha": p.MergeSha} {
		if v != "" {
			out[key] = v
		}
	}
	return out, nil
}

// pullURL is the page of pull request number of repo on its host.
func pullURL(repo string, number int64) string {
	if watch.RepoName(repo) {
		return fmt.Sprintf("https://github.com/%s/pull/%d", repo, number)
	}
	return fmt.Sprintf("https://%s/-/merge_requests/%d", repo, number)
}

// unreadPull is why a read of a pull request failed, for the card: gh missing or logged out on this machine first, since that is what a
// person can fix; then a pull request this login cannot see; then a read that ran out of time; then whatever the CLI said. A gh that is
// only slow, or a GitHub that is down, is never said to be logged out: Unready answers nothing for those.
func unreadPull(r watch.Reader, err error) map[string]any {
	if reason, why := r.Unready(); reason != "" {
		return unavailable(reason, why)
	}
	if errors.Is(err, watch.ErrGone) {
		if _, gh := r.(*watch.GitHub); gh {
			return unavailable("gone", "GitHub has no such pull request, or this machine's gh login cannot see it.")
		}
		return unavailable("failed", fmt.Sprintf("%s has no such merge request, or this machine's glab login cannot see it.", r.Name()))
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return unavailable("failed", fmt.Sprintf("%s did not answer within %s.", r.Name(), branchPRWait))
	}
	said := []rune(strings.Join(strings.Fields(textsafe.Oneline(err.Error())), " "))
	if len(said) > unreadMax {
		said = append(said[:unreadMax], '…')
	}
	return unavailable("failed", string(said))
}
