package db

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/handline/handline/go/data/rowstate"
	"github.com/handline/handline/go/kit/actor"
)

// NoticeWatchS is how long the one told about a task has to move it before the founder is told once.
const NoticeWatchS = 3600

const noticeWatchPrefix = "notice_watch:"

// NoticeTo is who hears that task t went wrong: the agent that opened it, unless it is the agent the notice is about; else the founder.
func NoticeTo(t *Task, about string) string {
	if t != nil && t.OpenedBy != "" && t.OpenedBy != about && !actor.IsPerson(t.OpenedBy) {
		return t.OpenedBy
	}
	return actor.OwnerKey
}

// AgentNoticeTo is who hears that agent went wrong with no task of its own in the notice: NoticeTo of the oldest open task waiting for
// it, with that task's number; the founder and 0 when none waits.
func AgentNoticeTo(q Q, agent string) (string, int64, error) {
	tasks, err := taskRows(q, "SELECT "+taskCols+" FROM tasks WHERE owner = ? AND done_evidence IS NULL AND discarded_by IS NULL "+
		"AND closed_at IS NULL ORDER BY created_at, id LIMIT 1", agent)
	if err != nil || len(tasks) == 0 {
		return actor.OwnerKey, 0, err
	}
	return NoticeTo(tasks[0], agent), tasks[0].ID, nil
}

// NoticeWatch is a notice about a task whose receiver has NoticeWatchS to move the task.
type NoticeWatch struct {
	Task int64  `json:"-"`
	To   string `json:"to"`
	Text string `json:"text"`
	At   int64  `json:"at"`
}

// WatchNotice keeps that to was told text about task tid at at, in place of an older watch on it. A watch already on to keeps its time,
// so a notice told again within the hour does not put the founder's hour off.
func WatchNotice(q Q, tid int64, to, text string, at int64) error {
	key := noticeWatchPrefix + strconv.FormatInt(tid, 10)
	prev, ok, err := Meta(q, key)
	if err != nil {
		return err
	}
	var old NoticeWatch
	if ok && json.Unmarshal([]byte(prev), &old) == nil && old.To == to {
		at = old.At
	}
	raw, err := json.Marshal(NoticeWatch{To: to, Text: text, At: at})
	if err != nil {
		return err
	}
	return SetMeta(q, key, string(raw))
}

// DueNoticeWatches takes out every watch NoticeWatchS old at now and answers those whose task has not moved since its notice: no event
// about it and no comment on it from the next second on (what the notice itself wrote is in its own second).
func DueNoticeWatches(q Q, now int64) ([]NoticeWatch, error) {
	names, err := MetaNames(q, noticeWatchPrefix)
	if err != nil {
		return nil, err
	}
	var due []NoticeWatch
	for _, name := range names {
		raw, _, err := Meta(q, name)
		if err != nil {
			return nil, err
		}
		var w NoticeWatch
		if json.Unmarshal([]byte(raw), &w) != nil {
			if err := ClearMeta(q, name); err != nil {
				return nil, err
			}
			continue
		}
		if now-w.At < NoticeWatchS {
			continue
		}
		if err := ClearMeta(q, name); err != nil {
			return nil, err
		}
		if w.Task, err = strconv.ParseInt(strings.TrimPrefix(name, noticeWatchPrefix), 10, 64); err != nil {
			continue
		}
		moved, err := TaskMovedSince(q, w.Task, w.At)
		if err != nil {
			return nil, err
		}
		if !moved {
			due = append(due, w)
		}
	}
	return due, nil
}

// FounderNoticePrefix keys a notice the founder is to see as a warning: founder_notice:T<n> for one about a task,
// founder_notice:<agent>:<reason> for one about an agent.
const FounderNoticePrefix = "founder_notice:"

// FounderNotice is a notice kept for the founder's warnings until FounderNoticeOver.
type FounderNotice struct {
	Agent  string `json:"agent"`
	Task   int64  `json:"task,omitempty"`
	Reason string `json:"reason"`
	Text   string `json:"text"`
	At     int64  `json:"at"`
}

// NoteFounderNotice keeps n for the founder, in place of an older one about the same task or agent.
func NoteFounderNotice(q Q, n FounderNotice) error {
	key := FounderNoticePrefix + n.Agent + ":" + n.Reason
	if n.Task != 0 {
		key = FounderNoticePrefix + "T" + strconv.FormatInt(n.Task, 10)
	}
	raw, err := json.Marshal(n)
	if err != nil {
		return err
	}
	return SetMeta(q, key, string(raw))
}

// boardTellKinds are the events the board writes when it tells someone about a task: telling is not the task moving.
const boardTellKinds = "('child_result_told', 'queued_blocker_alarm', 'task_deliver_refused', 'task_timeout_exceeded')"

// TaskMovedSince is whether task tid has moved from the second after at: an event about it that is not the board telling, or a comment.
func TaskMovedSince(q Q, tid, at int64) (bool, error) {
	var moved bool
	err := q.QueryRow("SELECT EXISTS(SELECT 1 FROM events WHERE ts >= ? AND kind NOT IN "+boardTellKinds+" AND "+
		"CASE WHEN json_valid(payload) THEN json_extract(payload, '$.task_id') END = ?) "+
		"OR EXISTS(SELECT 1 FROM task_memos WHERE task_id = ? AND created_at >= ?)", at+1, tid, tid, at+1).Scan(&moved)
	return moved, err
}

// FounderNoticeOver is whether what n says is over: its task moved or ended; with no task, the agent's stall cleared (start_stall), its
// repo fixed (bad_repo), or (any other) a message or comment of its own since.
func FounderNoticeOver(q Q, n FounderNotice) (bool, error) {
	if n.Task != 0 {
		t, err := GetTask(q, n.Task)
		if err != nil || t == nil || t.DoneEvidence.String != "" || t.DiscardedBy.String != "" {
			return true, err
		}
		return TaskMovedSince(q, n.Task, n.At)
	}
	switch n.Reason {
	case "start_stall":
		row, err := GetAgentState(q, n.Agent)
		return row == nil || row.State != rowstate.Stalled, err
	case "bad_repo":
		_, last, _, err := LastEvent(q, n.Agent, "bad_repo")
		repo, _ := last["repo"].(string)
		return repo == "", err
	}
	var acted bool
	err := q.QueryRow("SELECT EXISTS(SELECT 1 FROM requests WHERE requester = ?1 AND ts >= ?2) "+
		"OR EXISTS(SELECT 1 FROM task_memos WHERE author = ?1 AND created_at >= ?2)", n.Agent, n.At+1).Scan(&acted)
	return acted, err
}

// FounderNotices is every kept founder notice by its meta name.
func FounderNotices(q Q) (map[string]FounderNotice, error) {
	names, err := MetaNames(q, FounderNoticePrefix)
	if err != nil {
		return nil, err
	}
	out := map[string]FounderNotice{}
	for _, name := range names {
		raw, _, err := Meta(q, name)
		if err != nil {
			return nil, err
		}
		var n FounderNotice
		if json.Unmarshal([]byte(raw), &n) == nil {
			out[name] = n
		}
	}
	return out, nil
}

// ClearOverFounderNotices drops each founder notice that is over, so a later notice of the same kind does not bring back an old one.
func ClearOverFounderNotices(q Q) error {
	all, err := FounderNotices(q)
	if err != nil {
		return err
	}
	for name, n := range all {
		over, err := FounderNoticeOver(q, n)
		if err != nil {
			return err
		}
		if over {
			if err := ClearMeta(q, name); err != nil {
				return err
			}
		}
	}
	return nil
}
