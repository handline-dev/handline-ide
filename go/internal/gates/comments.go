package gates

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/taskboard/watch"
)

// What people write on a task's pull request reaches the agent acting on the task: each comment, review and line comment once, as a
// comment of the daemon's on the task, which is told to its holder as any comment is. The task and the pull request are one
// conversation that way; nothing is written back to the pull request.

// maxCommentRunes is how much of one comment's words the task's comment carries.
const maxCommentRunes = 4000

// Commenter reads what people wrote on a pull request (watch.GitHub).
type Commenter interface {
	Comments(ctx context.Context, repo string, number int64) ([]watch.Comment, error)
}

// relayComments tells each open task's holder what the repository's people wrote on its pull request since the task was opened, each
// once.
func (s *Sweep) relayComments(ctx context.Context, vendors map[string]watch.Vendor) error {
	c, ok := vendors["github"].(Commenter)
	if !ok {
		return nil
	}
	rows, err := s.DB.Query("SELECT id, pr, COALESCE(pr_repo, ''), created_at, "+db.ActingSQL("tasks")+" FROM tasks "+
		"WHERE pr IS NOT NULL AND owner != ? AND done_evidence IS NULL AND discarded_by IS NULL ORDER BY id", db.Gate)
	if err != nil {
		return err
	}
	defer rows.Close()
	type prTask struct {
		id, pr, created int64
		repo, acting    string
	}
	var tasks []prTask
	for rows.Next() {
		var t prTask
		if err := rows.Scan(&t.id, &t.pr, &t.repo, &t.created, &t.acting); err != nil {
			return err
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	if s.unread == nil {
		s.unread = map[int64]string{}
	}
	for _, t := range tasks {
		if ctx.Err() != nil {
			return nil
		}
		repo := t.repo
		if repo == "" {
			repo = s.originOf(t.acting)
		}
		if !watch.RepoName(repo) { // none, or on a host other than GitHub, whose comments are not relayed
			continue
		}
		rctx, cancel := context.WithTimeout(ctx, ReadWait)
		cs, err := c.Comments(rctx, repo, t.pr)
		cancel()
		if err != nil {
			// said once while the reason stays: a pull request that is gone, or a gh logged out, is read again every pass
			if why := err.Error(); ctx.Err() == nil && s.unread[t.id] != why {
				s.unread[t.id] = why
				s.logf("  T%d: the comments on %s %s could not be read: %v", t.id, repo, prWord(t.pr), err)
			}
			continue
		}
		delete(s.unread, t.id)
		for _, cm := range cs {
			// only what the repository's own people write: anyone else's words never reach an agent
			if cm.Bot || !cm.Member || cm.At.Before(time.Unix(t.created, 0)) {
				continue
			}
			if err := s.relay(t.id, t.pr, cm); err != nil {
				s.logf("  T%d: comment %s on %s: %v", t.id, cm.Key, prWord(t.pr), err)
			}
		}
	}
	return nil
}

// relay writes comment c on task tid's pull request pr as the daemon's comment on the task, unless it was already, and tells its holder.
func (s *Sweep) relay(tid, pr int64, c watch.Comment) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var one int
	switch err := tx.QueryRow("SELECT 1 FROM pr_comments_told WHERE task_id = ? AND comment = ?", tid, c.Key).Scan(&one); {
	case err == nil:
		return nil
	case err != sql.ErrNoRows:
		return err
	}
	text := commentText(pr, c)
	mid, err := db.AddComment(tx, tid, daemonAuthor, text)
	if err != nil {
		return err
	}
	if _, err := tx.Exec("INSERT INTO pr_comments_told(task_id, comment, task_comment, told_at) VALUES(?, ?, ?, ?)", tid, c.Key, mid, time.Now().Unix()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if s.Tell != nil {
		s.Tell.CommentChanged(tid, mid, daemonAuthor, "added", text, false, nil)
	}
	return nil
}

// commentText is how comment c on pull request pr reads on the task: what it is and where, then its words.
func commentText(pr int64, c watch.Comment) string {
	head := prWord(pr) + " comment"
	switch c.Kind {
	case watch.CommentReview:
		head = prWord(pr) + " review"
		if v := reviewWord(c.Review); v != "" {
			head += " (" + v + ")"
		}
	case watch.CommentLine:
		head = prWord(pr) + " line comment on " + c.Path
		if c.Line > 0 {
			head += fmt.Sprintf(":%d", c.Line)
		}
	}
	// a GitHub @name is no handle here: written so that no mention rule reads it as one of the team's
	body := strings.ReplaceAll(strings.TrimSpace(c.Body), "@", "＠")
	if utf8.RuneCountInString(body) > maxCommentRunes {
		body = string([]rune(body)[:maxCommentRunes]) + "…"
	}
	return head + ":\n" + body
}

// prWord names pull request n as its host does.
func prWord(n int64) string { return fmt.Sprintf("PR #%d", n) }

func reviewWord(state string) string {
	switch state {
	case "CHANGES_REQUESTED":
		return "changes requested"
	case "APPROVED":
		return "approved"
	}
	return ""
}

// originOf is the GitHub owner/name of the origin of the folder the roster keeps for agent, "" when there is none.
func (s *Sweep) originOf(agent string) string {
	repo := team.NewRoster(s.Home).RepoOf(agent)
	if repo == "" {
		return ""
	}
	r := gitrun.Run(repo, "remote", "get-url", "origin")
	if r.RC != 0 {
		return ""
	}
	if origin := watch.OriginRepo(r.Stdout); watch.RepoName(origin) {
		return origin
	}
	return ""
}
