package db

import (
	"encoding/json"
	"strconv"
)

// SeenFloorMeta holds the comment number at or below which a task with no marker of its own counts as read.
const SeenFloorMeta = "task_seen_floor"

// ResultSeenFloorMeta holds the time at or before which a request that ended counts as seen.
const ResultSeenFloorMeta = "result_seen_floor"

// ResultSeenPrefix is the cursors holding when the founder last saw a task that had ended; a task ended after that is unseen again.
const ResultSeenPrefix = "result_seen:"

func ResultSeenKey(tid int64) string { return ResultSeenPrefix + strconv.FormatInt(tid, 10) }

func SeenKey(tid int64) string { return "task_seen:" + Human + ":" + strconv.FormatInt(tid, 10) }

// LoginNeedPrefix is the meta rows holding a LoginNeed for each account that wants a login or stands stalled.
const LoginNeedPrefix = "founder_login_need:"

// LoginNeed is since when an account has wanted a login, and Unusable, when its stall is not a login's, the word for why it cannot be used.
type LoginNeed struct {
	Since    int64  `json:"since"`
	Unusable string `json:"unusable,omitempty"`
}

// ReadLoginNeed is the need a meta value holds; ok is false for a value that is not one.
func ReadLoginNeed(value string) (n LoginNeed, ok bool) {
	return n, json.Unmarshal([]byte(value), &n) == nil && n.Since > 0
}

// SetLoginNeed keeps need for acct, writing only when it changed.
func SetLoginNeed(q Q, acct string, need LoginNeed) error {
	raw, err := json.Marshal(need)
	if err != nil {
		return err
	}
	if v, _, err := Meta(q, LoginNeedPrefix+acct); err != nil || v == string(raw) {
		return err
	}
	return SetMeta(q, LoginNeedPrefix+acct, string(raw))
}

// A founder warning kept in meta until its state says it is over. Each is one JSON Undelivered, by the request or the task it is about.
const (
	LostMessagePrefix = "founder_message_lost:"
	RefusedTaskPrefix = "founder_task_refused:"
)

// Undelivered is what did not reach whom, for a warning on the founder's turn.
type Undelivered struct {
	Agent string `json:"agent"`
	Why   string `json:"why"`
	At    int64  `json:"at"`
}

// NoteUndelivered records one warning under its prefix and id.
func NoteUndelivered(q Q, prefix string, id, at int64, agent, why string) error {
	raw, err := json.Marshal(Undelivered{Agent: agent, Why: why, At: at})
	if err != nil {
		return err
	}
	return SetMeta(q, prefix+strconv.FormatInt(id, 10), string(raw))
}
