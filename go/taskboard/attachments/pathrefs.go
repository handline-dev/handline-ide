package attachments

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/protocol/rpc"
)

// A path ref (contract x-attachments, refs) is a file or folder on the daemon's machine that the founder names by absolute path in
// chat.send: nothing is copied, and the agent is told the path as it is.

// PathRef is one ref of a message, as sent, and whether it was a folder when it was sent.
type PathRef struct {
	Path   string `json:"path"`
	Folder bool   `json:"folder,omitempty"`
}

// CheckPathRefs is chat.send's refs once they are a list of strings: each absolute with no `..` part, and on the machine.
func CheckPathRefs(paths []string) ([]PathRef, error) {
	out := make([]PathRef, 0, len(paths))
	for _, p := range paths {
		if !strings.HasPrefix(p, "/") || textsafe.Oneline(p) != p || hasDotDot(p) {
			return nil, badParam("refs are absolute paths on one line with no .. part", "refs")
		}
		st, err := os.Stat(p)
		if os.IsNotExist(err) {
			e := rpc.NotFoundError("no file or folder at " + p)
			e.Data = map[string]any{"what": "path", "name": p}
			return nil, e
		}
		if err != nil {
			return nil, badParam(p+" cannot be read: "+err.Error(), "refs")
		}
		out = append(out, PathRef{Path: p, Folder: st.IsDir()})
	}
	return out, nil
}

func hasDotDot(p string) bool {
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func pathRefsKey(rid int64) string { return fmt.Sprintf("refs:%d", rid) }

// RecordPathRefs keeps the refs message rid was sent with, in order.
func RecordPathRefs(q db.Q, rid int64, refs []PathRef) error {
	b, err := json.Marshal(refs)
	if err != nil {
		return err
	}
	return db.SetMeta(q, pathRefsKey(rid), string(b))
}

// PathRefsOf is the refs message rid was sent with; none for a message with none.
func PathRefsOf(q db.Q, rid int64) []PathRef {
	v, ok, err := db.Meta(q, pathRefsKey(rid))
	if err != nil || !ok {
		return nil
	}
	var refs []PathRef
	_ = json.Unmarshal([]byte(v), &refs)
	return refs
}

// PathRefLine is how an agent is told of ref n: `[file N: /path]` or `[folder N: /dir]`.
func PathRefLine(n int, r PathRef) string {
	word := "file"
	if r.Folder {
		word = "folder"
	}
	return fmt.Sprintf("[%s %d: %s]", word, n, r.Path)
}

// PathRefLines is the lines of refs, numbered from after the first n.
func PathRefLines(refs []PathRef, n int) []string {
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = PathRefLine(n+i+1, r)
	}
	return out
}

// Map is the ref as a member of a reply.
func (r PathRef) Map() map[string]any { return map[string]any{"path": r.Path, "folder": r.Folder} }
