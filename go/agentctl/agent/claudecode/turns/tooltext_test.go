package turns

import "testing"

// A tool call reads as its first non-empty field in ToolText's order.
func TestToolTextTakesTheFirstFieldInItsOrder(t *testing.T) {
	keys := []string{"command", "file_path", "path", "pattern", "url", "query", "description", "prompt"}
	for i, k := range keys {
		input := map[string]any{"other": "x"}
		for _, later := range keys[i:] {
			input[later] = later + " value"
		}
		if i > 0 {
			input[keys[i-1]] = ""
		}
		if got := ToolText(input); got != k+" value" {
			t.Errorf("with %v: %q, want %q", input, got, k+" value")
		}
	}
	if got := ToolText(map[string]any{"other": "x", "command": 3}); got != "" {
		t.Errorf("nothing it reads: %q", got)
	}
}

// The doing line reads a call's description first, as Bash's says what the command does; without one it reads as ToolText.
func TestTheDoingLineReadsTheDescriptionFirst(t *testing.T) {
	line := func(input string) string {
		d := toolCall([]byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":` + input + `}]}}`))
		if d == nil {
			t.Fatalf("no call in %s", input)
		}
		return d.Text
	}
	if got := line(`{"command":"cd /x && go test ./...","description":"Run the tests\nall of them"}`); got != "Bash Run the tests" {
		t.Errorf("with a description: %q", got)
	}
	if got := line(`{"command":"go test ./...","description":""}`); got != "Bash go test ./..." {
		t.Errorf("with an empty description: %q", got)
	}
}
