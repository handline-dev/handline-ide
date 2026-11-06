package compose

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
)

// tailBytes is how much of a transcript's end is read for its last reply: a reply with a long thinking block or a large tool result is big.
const tailBytes = 8 << 20

// chunkBytes is how much is read at a time, newest first: the last reply is most often in the last few lines.
const chunkBytes = 64 << 10

// lastContext is what the newest reply in a Claude transcript says the context holds: its input, cache-read and cache-write tokens, and the
// model that gave it. ok false when the file has no reply in its tail. It reads back from the end and stops at the first reply, since
// agents.list asks this of every agent on every call.
func lastContext(path string) (used int64, model string, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return 0, "", false
	}
	return newestReply(f, st.Size())
}

// newestReply is lastContext over the first size bytes of r. Each byte of the tail is read once to find the line breaks, and a line that
// runs past the chunk it ends in once more, whole.
func newestReply(r io.ReaderAt, size int64) (used int64, model string, ok bool) {
	floor := max(0, size-tailBytes)
	buf := make([]byte, chunkBytes)
	lineEnd := size // where the line being looked for ends: the next line break back from here starts it
	line := func(from int64, chunk []byte, chunkAt int64) (int64, string, bool) {
		if lineEnd <= chunkAt+int64(len(chunk)) {
			return replyContext(chunk[from-chunkAt : lineEnd-chunkAt])
		}
		whole := make([]byte, lineEnd-from)
		if _, err := r.ReadAt(whole, from); err != nil {
			return 0, "", false
		}
		return replyContext(whole)
	}
	var chunk []byte
	at := size
	for at > floor {
		start := max(floor, at-chunkBytes)
		chunk = buf[:at-start]
		if _, err := r.ReadAt(chunk, start); err != nil {
			return 0, "", false
		}
		for i := bytes.LastIndexByte(chunk, '\n'); i >= 0; i = bytes.LastIndexByte(chunk[:i], '\n') {
			if used, model, ok = line(start+int64(i)+1, chunk, start); ok {
				return used, model, ok
			}
			lineEnd = start + int64(i)
		}
		at = start
	}
	if floor > 0 || size == 0 {
		return 0, "", false // the first line of a cut tail is a fragment
	}
	return line(0, chunk, 0)
}

// replyContext is lastContext for one transcript line, ok false when it is not a main-thread reply with usage.
func replyContext(line []byte) (used int64, model string, ok bool) {
	if !bytes.Contains(line, []byte(`"assistant"`)) {
		return 0, "", false
	}
	var e struct {
		Type    string `json:"type"`
		Side    bool   `json:"isSidechain"`
		Message struct {
			Model string         `json:"model"`
			Raw   map[string]any `json:"usage"`
		} `json:"message"`
	}
	if json.Unmarshal(bytes.TrimSpace(line), &e) != nil || e.Type != "assistant" || e.Side || e.Message.Raw == nil {
		return 0, "", false
	}
	for _, k := range []string{"input_tokens", "cache_read_input_tokens", "cache_creation_input_tokens"} {
		if v, isNum := e.Message.Raw[k].(float64); isNum {
			used += int64(v)
		}
	}
	return used, e.Message.Model, true
}
