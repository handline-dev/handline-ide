package rulesfold

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/handline/handline/go/accounts/livetest"
	"github.com/handline/handline/go/data/db"
)

// TestLiveFoldStep runs one step against a real CLI on a scratch board: a person's rule, a screen task with a correction, chat with a
// standard and an ack. The answer must validate, keep the person's rule, and write the cursors.
// HANDLINE_LIVE=1 go test -run TestLiveFoldStep -v ./taskboard/rulesfold
func TestLiveFoldStep(t *testing.T) {
	cfg := livetest.Pick(t).ConfigDir
	path, err := exec.LookPath("claude")
	if err != nil {
		t.Fatal(err)
	}
	d := board(t)
	hand, _ := db.AddLesson(d, person, "Write to the reviewer in Korean.", 0, person)
	tid := task(t, d, "Settings tab")
	comment(t, d, tid, "a1", "Here is the settings tab, with a hint under each field.", 100)
	comment(t, d, tid, person, "힌트나 부제는 빼자. 문구가 필요하다면 UX가 문제야.", 110)
	chat(t, d, "a2", person, "Merged; the bug is gone in my run.", 120)
	chat(t, d, person, "a2", "근본 원인을 고쳤니? 직접 재현해서 확인하고 보고해.", 130)
	chat(t, d, person, "a2", "좋아", 140)
	res, err := Step(context.Background(), d, person, Claude(path, cfg, nil), func(int64) string { return db.AreaScreen }, true)
	if err != nil || !res.Written {
		t.Fatal(res, err)
	}
	ls := live(t, d)
	for _, l := range ls {
		t.Log(l.Text)
	}
	if len(ls) < 2 || !strings.Contains(ls[0].Text, "Korean") && ls[0].ID != hand {
		t.Fatal("the person's rule must stay")
	}
	if cur, _ := ReadCursors(d, person); cur.Comments == 0 || cur.Chat == 0 {
		t.Fatal(cur)
	}
}
