package transcript

import (
	"encoding/json"
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
	if err != nil || len(tools) != 2 || tools[0].Name != "Bash" || tools[0].Text != "ls\nwc -l" || tools[0].Output == nil || *tools[0].Output != "a\nb" {
		t.Fatalf("the call with its result: %+v %v", tools, err)
	}
	if !tools[1].Thinking || tools[1].Text != "..." || tools[1].Output != nil {
		t.Fatalf("the thinking block, after the call: %+v", tools[1])
	}
	var n int
	r.DB.QueryRow("SELECT count(*) FROM transcript_events WHERE agent = 'bob' AND session = 's1'").Scan(&n)
	if n != 4 {
		t.Fatalf("one row each for the text, the thinking, the call and the result: %d", n)
	}
}

func TestASubagentsLinesAreStoredUnderTheCallThatStartedItAndItsTextIsNoReply(t *testing.T) {
	r := newRows(t)
	sub := `{"type":"assistant","isSidechain":true,"message":{"content":[{"type":"text","text":"found it"},{"type":"tool_use","id":"tu9","name":"Grep","input":{}}]}}`
	if err := RecordUnder(r.DB, "bob", "s1", "tuTask", []byte(sub)); err != nil {
		t.Fatal(err)
	}
	if err := RecordUnder(r.DB, "bob", "s1", "tuTask", []byte(replyLine)); err != nil { // the session's own line is not the subagent's
		t.Fatal(err)
	}
	if replies, _ := Replies(r.DB, "bob", 10); len(replies) != 0 {
		t.Fatalf("a subagent's text is not a reply: %+v", replies)
	}
	var n int
	r.DB.QueryRow("SELECT count(*) FROM transcript_events WHERE agent = 'bob' AND json_extract(body, '$.parent') = 'tuTask'").Scan(&n)
	if n != 2 {
		t.Fatalf("its text and its call, each under the parent: %d", n)
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

// A subagent's call and thinking name the item of the call that started it; the subagent's text stays out.
func TestASubagentsStepsNameTheItemOfTheCallThatRanIt(t *testing.T) {
	r := newRows(t)
	Record(r.DB, "bob", "s1", []byte(`{"type":"assistant","timestamp":"2026-11-05T01:00:00Z","message":{"content":[{"type":"tool_use","id":"tuTask","name":"Agent","input":{}}]}}`))
	sub := `{"type":"assistant","isSidechain":true,"timestamp":"2026-11-05T01:00:05Z","message":{"content":[{"type":"thinking","thinking":"look"},{"type":"text","text":"found it"},{"type":"tool_use","id":"tu9","name":"Grep","input":{}}]}}`
	RecordUnder(r.DB, "bob", "s1", "tuTask", []byte(sub))
	tools, err := Tools(r.DB, "bob", ToolWindow{Limit: 10})
	if err != nil || len(tools) != 3 {
		t.Fatalf("the call, the subagent's thinking and its call, not its text: %+v %v", tools, err)
	}
	if tools[0].Parent != 0 || tools[1].Parent != tools[0].ID || tools[2].Parent != tools[0].ID || !tools[1].Thinking || tools[2].Name != "Grep" {
		t.Fatalf("each subagent step under the Agent call's item: %+v", tools)
	}
}

// An edit's result line carries its change: the hunks as stored, cut at the start past ResultMax, and the lines it adds and removes.
func TestAnEditsResultCarriesItsDiff(t *testing.T) {
	r := newRows(t)
	Record(r.DB, "bob", "s", []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu1","name":"Edit","input":{}}]}}`))
	result := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tu1","content":"updated"}]},` +
		`"toolUseResult":{"filePath":"a.go","structuredPatch":[{"oldStart":3,"oldLines":2,"newStart":3,"newLines":2,"lines":[" a","-b","+c"]},` +
		`{"oldStart":9,"oldLines":1,"newStart":9,"newLines":2,"lines":["+` + strings.Repeat("x", ResultMax) + `","+e"]}]}}`
	Record(r.DB, "bob", "s", []byte(result))
	tools, _ := Tools(r.DB, "bob", ToolWindow{Limit: 10})
	if len(tools) != 1 || tools[0].Added != 3 || tools[0].Removed != 1 {
		t.Fatalf("counted over the whole change: %+v", tools)
	}
	b, _ := json.Marshal(tools[0].Diff)
	if want := `{"dropped_lines":2,"hunks":[{"lines":[" a","-b","+c"],"new_start":3,"old_start":3}]}`; string(b) != want {
		t.Fatalf("diff %s, want %s", b, want)
	}
	Record(r.DB, "bob", "s", []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu2","name":"Write","input":{}}]}}`))
	Record(r.DB, "bob", "s", []byte(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tu2","content":"created"}]},"toolUseResult":{"type":"create","structuredPatch":[]}}`))
	if tools, _ := Tools(r.DB, "bob", ToolWindow{Limit: 10}); tools[1].Diff != nil {
		t.Fatalf("a new file has no diff: %+v", tools[1])
	}
}

// An error's or an MCP tool's result line, whose toolUseResult is a string or a list, keeps its result.
func TestAResultWhoseToolUseResultIsNoObjectIsKept(t *testing.T) {
	r := newRows(t)
	for i, extra := range []string{`"Error: boom"`, `[{"type":"text","text":"x"}]`} {
		id := string(rune('a' + i))
		Record(r.DB, "bob", "s", []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"`+id+`","name":"Bash","input":{}}]}}`))
		Record(r.DB, "bob", "s", []byte(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"`+id+`","content":"failed","is_error":true}]},"toolUseResult":`+extra+`}`))
	}
	tools, _ := Tools(r.DB, "bob", ToolWindow{Limit: 10})
	if len(tools) != 2 {
		t.Fatalf("%+v", tools)
	}
	for _, tl := range tools {
		if tl.Output == nil || *tl.Output != "failed" || tl.Diff != nil {
			t.Fatalf("%+v", tools)
		}
	}
}

// A subagent step whose Agent call is older than the page still names that call's item.
func TestASubagentStepNamesItsCallFromBeforeThePage(t *testing.T) {
	r := newRows(t)
	Record(r.DB, "bob", "s1", []byte(`{"type":"assistant","timestamp":"2026-11-05T01:00:00Z","message":{"content":[{"type":"tool_use","id":"tuTask","name":"Agent","input":{}}]}}`))
	RecordUnder(r.DB, "bob", "s1", "tuTask", []byte(`{"type":"assistant","isSidechain":true,"timestamp":"2026-11-05T01:00:09Z","message":{"content":[{"type":"tool_use","id":"tu9","name":"Grep","input":{}}]}}`))
	all, _ := Tools(r.DB, "bob", ToolWindow{Limit: 10})
	page, _ := Tools(r.DB, "bob", ToolWindow{Limit: 1})
	if len(page) != 1 || page[0].Name != "Grep" || page[0].Parent != all[0].ID {
		t.Fatalf("%+v %+v", page, all)
	}
}
