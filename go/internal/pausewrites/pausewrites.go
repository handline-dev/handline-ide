// Package pausewrites serves daemon.pause and daemon.resume, the founder's switch over new work (package pause).
package pausewrites

import (
	"database/sql"
	"time"

	"github.com/handline/handline/go/kit/timefmt"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/pause"
)

// Register adds daemon.pause and daemon.resume. changed runs after each one that changed the pause: the daemon tells the status topic.
// now is the clock; nil is time.Now.
func Register(s *rpc.Server, now func() time.Time, changed func()) {
	if now == nil {
		now = time.Now
	}
	s.Register("daemon.pause", rpc.Method{Handler: func(c *rpc.Conn, p map[string]any) (any, error) {
		at := now()
		var until int64
		if raw, ok := p["until"]; ok {
			str, _ := raw.(string)
			t, err := time.Parse(time.RFC3339, str)
			if err != nil {
				return nil, rpc.BadRequestError("until is an RFC 3339 time", "until", rpc.InvalidParams)
			}
			if until = t.Unix(); until <= at.Unix() { // whole seconds, as the pause is kept
				return nil, rpc.BadRequestError("until is not in the future", "until", rpc.InvalidParams)
			}
		}
		res, err := c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			if _, err := pause.Set(tx, at, until); err != nil {
				return nil, err
			}
			said := "New work is paused until it is resumed."
			if until != 0 {
				said = "New work is paused until " + timefmt.Stamp(until, timefmt.Clock) + "."
			}
			return map[string]any{"accepted": true, "said": said}, nil
		})
		if err == nil && changed != nil {
			changed()
		}
		return res, err
	}})
	s.Register("daemon.resume", rpc.Method{Handler: func(c *rpc.Conn, _ map[string]any) (any, error) {
		was := false
		res, err := c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			st, err := pause.Get(tx, now())
			if err != nil {
				return nil, err
			}
			if was = st != nil; !was {
				return map[string]any{"accepted": true, "said": "New work was not paused."}, nil
			}
			if err := pause.Clear(tx); err != nil {
				return nil, err
			}
			return map[string]any{"accepted": true, "said": "New work goes on."}, nil
		})
		if err == nil && was && changed != nil {
			changed()
		}
		return res, err
	}})
}
