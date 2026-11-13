package states

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/handline/handline/go/kit/addr"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
)

const (
	// AskedShown is how much of the original question an answer line quotes.
	AskedShown = 40
	// Ready is the reason of a task nothing is in the way of.
	Ready = "nothing is in the way - the daemon sends it on its next tick"
)

var handoffBody = regexp.MustCompile(`(?s)\AT(\d+) handed to you by (\S+?): (.*?)\n\n(.*)\z`)

// HandoffParts is (task id, who handed it, title, note) of a task handoff's body (actions.tasks.task_handoff); ok false for any other.
func HandoffParts(body string) (tid int64, by, title, note string, ok bool) {
	m := handoffBody.FindStringSubmatch(body)
	if m == nil {
		return 0, "", "", "", false
	}
	fmt.Sscan(m[1], &tid)
	return tid, m[2], strings.TrimSpace(m[3]), strings.TrimSpace(m[4]), true
}

// TaskOf is the task a kind-task request is about: the one it is linked to, else the one its body names; nil for none.
func TaskOf(q db.Q, r *db.Request) (*db.Task, error) {
	linked, err := db.TasksForRequest(q, r.ID)
	if err != nil {
		return nil, err
	}
	if len(linked) > 0 {
		return linked[len(linked)-1], nil
	}
	if tid, _, _, _, ok := HandoffParts(r.Body); ok {
		return db.GetTask(q, tid)
	}
	return nil, nil
}

// AnswerVerb is how the agent closes the request it reads.
func AnswerVerb(q db.Q, head *db.Request) (string, error) {
	if head.Kind.String == "task" {
		t, err := TaskOf(q, head)
		if err != nil {
			return "", err
		}
		if t != nil {
			return fmt.Sprintf("you own T%d now: hand it on with `hl task handoff T%d <agent> \"<note>\"`, or `hl task done T%d` if you are its last step", t.ID, t.ID, t.ID), nil
		}
	}
	if actor.IsPerson(head.Requester) {
		return fmt.Sprintf("answer the founder now, before other work: `hl say human --parent %s \"...\"` - if it is work, one line on what you will do or who has it (`hl task begin \"<title>\" --chat <msg>` takes it, `hl task request <msg>` gives it to another); a decision you need from the founder goes on the task it is about with `--turn human`, not here", head.Name(q)), nil
	}
	if head.Kind.String == "say" {
		return "it asks nothing", nil
	}
	return fmt.Sprintf("answer with `hl say %s --parent %s \"<answer>\"`", addr.Address(q, head.Target, head.Requester), head.Name(q)), nil
}

// MoreWaiting is what an agent is told about the queue behind the request it reads, "" for nothing.
func MoreWaiting(kind string, more int) string {
	if more == 0 || kind == "say" {
		return ""
	}
	if kind == "task" {
		return fmt.Sprintf("%d more waiting - the next comes when you hand this on or finish it.", more)
	}
	return fmt.Sprintf("%d more waiting - the next comes when you answer this one.", more)
}
