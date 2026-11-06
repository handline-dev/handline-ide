package reads

import (
	"encoding/json"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/internal/housekeeping"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/loadgate"
	"github.com/handline/handline/go/scheduler/pause"
)

func (e *Env) status(c *rpc.Conn, params map[string]any) (any, error) { return e.Status(c.Server()) }

// Status is daemon.status as srv serves it: what status.changed carries.
func (e *Env) Status(srv *rpc.Server) (map[string]any, error) {
	st, _, err := e.StatusHealth(srv)
	return st, err
}

// StatusHealth is Status, and the health in it.
func (e *Env) StatusHealth(srv *rpc.Server) (map[string]any, Health, error) {
	stored, err := e.storedViews()
	if err != nil {
		return nil, Health{}, err
	}
	names := e.listAgents()
	byState := map[string]int{}
	for _, n := range names {
		state := noView
		if v, ok := stored[n]; ok {
			var view struct {
				State string `json:"state"`
			}
			if json.Unmarshal(v, &view) == nil && view.State != "" {
				state = view.State
			}
		}
		byState[state]++
	}
	items, err := e.founderItems()
	if err != nil {
		return nil, Health{}, err
	}
	out := map[string]any{
		"alive": true, "version": srv.Sha, "exe": srv.Exe, "protocol": rpc.Protocol, "schema": srv.Schema,
		"started_at": ts(e.Started.Unix()),
		// This daemon drives no terminal host yet.
		"terminal_host": map[string]any{"name": "none", "health": "down", "detail": "this daemon does not drive a terminal host yet"},
		"agents":        map[string]any{"total": len(names), "by_state": byState},
		"inbox":         map[string]any{"unread": len(items)},
		"resources":     e.resources(),
		"paused":        e.paused(),
	}
	// The machine's own needs-you items, none of an agent's: a disk too full for builds.
	if needs := e.machineNeeds(); len(needs) > 0 {
		out["needs_you"] = needs
	}
	h, err := e.Health()
	if err != nil {
		return nil, Health{}, err
	}
	out["health"] = h.view()
	return out, h, nil
}

// paused is DaemonStatus.paused: the founder's pause of new work, nil when there is none.
func (e *Env) paused() any {
	p, err := pause.Get(e.DB, time.Now())
	if err != nil || p == nil {
		return nil
	}
	var until any
	if p.Until != 0 {
		until = ts(p.Until)
	}
	return map[string]any{"since": ts(p.Since), "until": until}
}

// machineNeeds are the items daemon.status carries: what the machine needs of the founder, oldest first.
func (e *Env) machineNeeds() []any {
	var out []any
	if since, free, ok := housekeeping.DiskNeed(e.DB); ok {
		out = append(out, map[string]any{"kind": "disk", "since": ts(since), "text": housekeeping.DiskNeedText(free)})
	}
	return out
}

// resources is the latest measure for daemon.status: null until the sampler has measured, or on a daemon that does not.
func (e *Env) resources() map[string]any {
	if e.Resources == nil {
		return nil
	}
	r := e.Resources()
	if r == nil {
		return nil
	}
	// Whether new starts are held now, and why: the load gate's own answer on the same measure and limits.
	var hold any
	if e.Machine != nil {
		by, _ := (&usage.Env{Home: e.Home}).ActiveByAccount(e.DB)
		active := 0
		for _, agents := range by {
			active += len(agents)
		}
		if why := loadgate.Held(loadgate.LimitsOf(usage.LoadConfig(e.Home)), active, e.Machine()); why != "" {
			hold = why
		}
	}
	r["start_hold"] = hold
	return r
}
