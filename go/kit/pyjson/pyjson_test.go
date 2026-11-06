package pyjson

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestPyJSONWritesWhatPythonsDumpsWrites(t *testing.T) {
	v := map[string]any{"b": []any{json.Number("1"), true, nil}, "a": "é\n\"\x7f😀", "c": map[string]any{}, "d": []string{"x"}}
	if got, want := PyJSON(v, true, false), "{\"a\": \"é\\n\\\"\x7f😀\", \"b\": [1, true, null], \"c\": {}, \"d\": [\"x\"]}"; got != want {
		t.Errorf("%s\nwant %s", got, want)
	}
	if got, want := PyJSON("\u00e9\x7f\U0001F600<", false, true), "\"\\u00e9\\u007f\\ud83d\\ude00<\""; got != want {
		t.Errorf("%s want %s", got, want)
	}
}

// Stored hashes (hr fields, tag keys) are sha256 of this output: the bytes must not drift. Only sorted output is pinned: unsorted keys come in map order.
func TestPyJSONBytesArePinnedByHash(t *testing.T) {
	v := map[string]any{"b": []any{json.Number("1"), 2.5, true, nil}, "a": "é\n\"\x7f😀", "c": map[string]any{}, "d": []string{"x"}}
	sum := sha256.Sum256([]byte(PyJSON(v, true, true) + PyJSON(v, true, false)))
	if got := hex.EncodeToString(sum[:]); got != "b8f4c0fd778c4643e6600d654539bff015cadec799f100b22c1ed125849d7239" {
		t.Errorf("sha256 %s", got)
	}
}
