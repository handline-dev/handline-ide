package reads

import (
	"fmt"
	"testing"
)

// A comment's stored event goes out with the names the wire still uses; any other event goes out as it is stored.
func TestACommentsEventKeepsItsWireNames(t *testing.T) {
	for _, c := range []struct{ kind, out, data string }{
		{"comment_edit", "memo_edit", "map[memo:7 now:x]"},
		{"task_turn_pop", "task_turn_pop", "map[memo:7 now:x]"},
		{"say", "say", "map[comment:7 now:x]"},
	} {
		data := map[string]any{"comment": 7, "now": "x"}
		if got := wireEvent(c.kind, data); got != c.out || fmt.Sprint(data) != c.data {
			t.Errorf("%s: %s %v", c.kind, got, data)
		}
	}
}
