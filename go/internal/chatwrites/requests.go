package chatwrites

import (
	"database/sql"
	"strings"

	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/taskboard/requestref"
	"github.com/handline/handline/go/taskboard/supervisor"
)

// registerRequests adds what `hl withdraw` and the app's cancel call.
func (w *writes) registerRequests(s *rpc.Server) {
	s.Register("request.withdraw", rpc.Method{Handler: w.withdrawRequest})
	// Unkept and OwnEvent: a dry run writes and records nothing, and a cancel that happened is judged again by a retry (already closed).
}

// withdrawRequest is the caller taking back a request of its own before it is delivered. A refusal is a reply, with ok false.
func (w *writes) withdrawRequest(c *rpc.Conn, p map[string]any) (any, error) {
	rid, err := requestref.Resolve(w.e.DB, p["id"], requestref.Viewer(c), "id")
	if name, missing := requestref.Missing(err); missing {
		return map[string]any{"ok": false, "said": "request " + name + " does not exist"}, nil
	}
	if err != nil {
		return nil, err
	}
	reason := ""
	if v := p["reason"]; v != nil {
		var ok bool
		if reason, ok = v.(string); !ok || strings.TrimSpace(reason) == "" {
			return nil, badParam("reason is text and not empty", "reason")
		}
	}
	after := &supervisor.After{}
	out, err := c.Effect(func(tx *sql.Tx) (map[string]any, error) {
		ok, msg, err := w.e.WithdrawWrite(tx, after, rid, c.Caller.Actor(), reason, nil)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": ok, "said": msg}, nil
	})
	if err == nil {
		after.Run()
	}
	return out, err
}
