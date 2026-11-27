package reads

import "strings"

// commentKeyKinds are the stored event kinds whose payload names a task comment under "comment".
var commentKeyKinds = map[string]bool{"comment_add": true, "comment_edit": true, "comment_delete": true, "task_turn_push": true,
	"task_turn_pop": true, "task_turn_ask": true, "task_ready": true, "task_verdict": true}

// wireEvent is a stored event's kind and payload as the wire still names them: a comment's events are memo_* and carry it as "memo".
func wireEvent(kind string, data map[string]any) string {
	if commentKeyKinds[kind] {
		if v, ok := data["comment"]; ok {
			delete(data, "comment")
			data["memo"] = v
		}
	}
	if rest, ok := strings.CutPrefix(kind, "comment_"); ok {
		return "memo_" + rest
	}
	return kind
}
