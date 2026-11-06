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
