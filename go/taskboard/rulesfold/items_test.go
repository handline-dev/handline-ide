package rulesfold

import (
	"strings"
	"testing"
)

// Items come from both sources in the order they were said, a picture's marks as their words, pasted text and acks left out but read
// past; a batch stops at its budget and says more waits.
func TestABatchMergesBothSourcesInOrderUpToItsBudget(t *testing.T) {
	d := board(t)
	tid := task(t, d, "Usage page")
	c1 := comment(t, d, tid, person, "move it up ```marks [{\"n\":1,\"text\":\"this line\"}]``` ![shot.png](attachment:ab12)", 100)
	m1 := chat(t, d, person, "a2", strings.Repeat("pasted log ", 200), 110)
	m2 := chat(t, d, person, "a2", "use the same modal everywhere", 120)
	c2 := comment(t, d, tid, person, "OK", 130)
	comment(t, d, tid, "a1", "not the person's", 140)
	b, err := ReadBatch(d, person, Cursors{}, Budget, func(int64) string { return "screen" })
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Items) != 2 || b.Items[0].Said != "move it up [mark: this line] [picture]" || b.Items[0].Kind != "screen" ||
		b.Items[0].Task != "Usage page" || b.Items[1].Ref != "msg_"+ref(t, d, m2) || b.Full {
		t.Fatalf("%+v", b)
	}
	if b.Next != (Cursors{Comments: c2, Chat: m2}) || m1 >= m2 || c1 >= c2 {
		t.Fatalf("next %+v", b.Next)
	}
	small, _ := ReadBatch(d, person, Cursors{}, 10, nil)
	if len(small.Items) != 1 || !small.Full || small.Next != (Cursors{Comments: c1, Chat: m1}) {
		t.Fatalf("a batch past its budget: %+v", small)
	}
}
