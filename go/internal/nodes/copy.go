package nodes

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/handline/handline/go/internal/boardcopy"
	"github.com/handline/handline/go/protocol/rpc"
)

// CopyChunk bounds the data one node.copy.files answer carries, base64 in JSON well under the link's frame (rpc.MaxLineBytes).
const CopyChunk = 512 << 10

// Copy is the board's copy as the master serves it to its owner's runners (boardcopy): the folder Litestream writes, and when each
// runner's mirror last held all of it.
type Copy struct {
	Dir string

	mu      sync.Mutex
	matched map[string]time.Time
}

func (c *Copy) set(node string, at *time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.matched == nil {
		c.matched = map[string]time.Time{}
	}
	if at == nil {
		delete(c.matched, node)
	} else {
		c.matched[node] = *at
	}
}

// CopyOf is node.status's copy of node: when its mirror last held all of the master's copy and how many seconds ago that was; nil when
// the master keeps no copy or node holds none (another person's runner, or one that has not caught up yet).
func (r *Registry) CopyOf(node string) any {
	if r.Copy == nil {
		return nil
	}
	r.Copy.mu.Lock()
	defer r.Copy.mu.Unlock()
	at, ok := r.Copy.matched[node]
	if !ok {
		return nil
	}
	return map[string]any{"matched_at": at.UTC().Format("2006-01-02T15:04:05Z"), "behind_s": int64(time.Since(at).Seconds())}
}

// copyFiles is node.copy.files: what the calling runner's mirror lacks of the board's copy, in pieces. Only the owner's runners hold
// one (gate owner), and another person's runner drops its copy when told so.
func (r *Registry) copyFiles(c *rpc.Conn, p map[string]any) (any, error) {
	node := r.NodeOf(c.Root())
	if node == "" {
		return nil, rpc.Refused("node.copy.files is a runner's own call, on its link", "node", "")
	}
	if r.Copy == nil {
		return nil, rpc.Refused("this master keeps no copy of its board now", "copy", "")
	}
	var role string
	err := r.DB.QueryRow("SELECT p.role FROM nodes n JOIN people p ON p.id = n.person WHERE n.id = ?", node).Scan(&role)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if role != "owner" {
		r.Copy.set(node, nil)
		return nil, rpc.Refused("only the owner's machines keep a copy of the board", "owner", "")
	}
	var have, parts []boardcopy.File
	for key, into := range map[string]*[]boardcopy.File{"have": &have, "parts": &parts} {
		if v, ok := p[key]; ok {
			b, err := json.Marshal(v)
			if err != nil || json.Unmarshal(b, into) != nil {
				return nil, rpc.BadRequestError(key+" is a list of {path, size}", key, rpc.InvalidParams)
			}
		}
	}
	send, gone, more, err := boardcopy.Changes(r.Copy.Dir, have, parts, CopyChunk)
	if err != nil {
		return nil, err
	}
	if len(send) == 0 && len(gone) == 0 { // it already holds all of the copy: only this says so for certain
		now := time.Now()
		r.Copy.set(node, &now)
	}
	return map[string]any{"send": send, "gone": gone, "more": more}, nil
}
