package transcript

import (
	"strings"
	"testing"
)

// Lines in the shape Claude Code writes its session transcript.
const (
	replyLine  = `{"type":"assistant","timestamp":"2026-11-05T01:00:00Z","message":{"content":[{"type":"text","text":"Looking."},{"type":"tool_use","id":"tu1","name":"Bash","input":{"command":"ls\nwc -l"}}]}}`
	resultLine = `{"type":"user","timestamp":"2026-11-05T01:00:02Z","message":{"content":[{"type":"tool_result","tool_use_id":"tu1","content":[{"type":"text","text":"a\nb"}],"is_error":false}]}}`
)

func TestATranscriptsRepliesToolCallsAndResultsAreStoredAsChatReadsThem(t *testing.T) {
	r := newRows(t)
	for _, l := range []string{
		replyLine, resultLine,
		`{"type":"user","message":{"content":"a person's prompt"}}`,
		`{"type":"assistant","isSidechain":true,"message":{"content":[{"type":"text","text":"a subagent"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"..."},{"type":"text","text":"  "}]}}`,
		`{"type":"summary"}`, `not json`,
	} {
		if err := Record(r.DB, "bob", "s1", []byte(l)); err != nil {
			t.Fatal(err)
		}
	}
	replies, err := Replies(r.DB, "bob", 10)
	if err != nil || len(replies) != 1 || replies[0].Text != "Looking." || replies[0].At != 1793840400 || replies[0].ID != ItemID(1793840400, ItemReply, 2) {
		t.Fatalf("only the assistant's own text is a reply: %+v %v", replies, err)
	}
	tools, err := Tools(r.DB, "bob", ToolWindow{Limit: 10})
	if err != nil || len(tools) != 1 || tools[0].Name != "Bash" || tools[0].Text != "ls\nwc -l" || tools[0].Output == nil || *tools[0].Output != "a\nb" {
		t.Fatalf("the call with its result: %+v %v", tools, err)
	}
	var n int
	r.DB.QueryRow("SELECT count(*) FROM transcript_events WHERE agent = 'bob' AND session = 's1'").Scan(&n)
	if n != 3 {
		t.Fatalf("one row each for the text, the call and the result: %d", n)
	}
}

func TestAStoredToolOutputIsCutAndSaysHowManyLinesWentAndOldRowsArePruned(t *testing.T) {
	r := newRows(t)
	big := strings.Repeat("x\n", ResultMax) // twice ResultMax bytes
	line := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tu1","content":"` + strings.ReplaceAll(big, "\n", `\n`) + `","is_error":true}]}}`
	Record(r.DB, "bob", "s", []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu1","name":"Read","input":{}}]}}`))
	Record(r.DB, "bob", "s", []byte(line))
	tools, _ := Tools(r.DB, "bob", ToolWindow{Limit: 10})
	if len(tools) != 1 || tools[0].Output == nil || len(*tools[0].Output) != ResultMax || tools[0].Dropped != int64(ResultMax/2) {
		t.Fatalf("cut to ResultMax, the rest counted: %d %d", len(*tools[0].Output), tools[0].Dropped)
	}
	var notOK int
	r.DB.QueryRow("SELECT count(*) FROM transcript_events WHERE kind = 'tool_result' AND json_extract(body, '$.tool_result.ok') = 0").Scan(&notOK)
	if notOK != 1 {
		t.Fatal("an is_error result is stored as not ok")
	}
	for i := 0; i < Keep+500; i++ {
		Record(r.DB, "bob", "s", []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"t"}]}}`))
	}
	var n, low int
	r.DB.QueryRow("SELECT count(*), MIN(seq) FROM transcript_events WHERE agent = 'bob'").Scan(&n, &low)
	if n > Keep+500 || low <= 2 {
		t.Fatalf("the oldest rows go: %d rows from seq %d", n, low)
	}
}
