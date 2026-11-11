package db

import (
	"encoding/json"
	"time"

	"github.com/handline/handline/go/kit/actor"
)

// PauseKey is the meta row the founder's pause of new work is kept in (package pause writes it).
const PauseKey = "daemon_pause"

// Pause is a pause: since when, and when it ends by itself (0 for only at a resume). Unix seconds.
type Pause struct {
	Since int64 `json:"since"`
	Until int64 `json:"until,omitempty"`
}

// GetPause is the pause in force at now, nil when there is none. A pause whose until has come is none.
func GetPause(q Q, now time.Time) (*Pause, error) {
	v, ok, err := Meta(q, PauseKey)
	if err != nil || !ok || v == "" {
		return nil, err
	}
	var s Pause
	if err := json.Unmarshal([]byte(v), &s); err != nil {
		return nil, err
	}
	if s.Until != 0 && now.Unix() >= s.Until {
		return nil, nil
	}
	return &s, nil
}

// PausePassed is the event task.send_now records on a task's delivery (request_id): it goes to its agent while new work is paused.
const PausePassed = "pause_passed"

// PassesPause is whether r goes while new work is paused: the founder sent it, or task.send_now let it go.
func PassesPause(q Q, r *Request) (bool, error) {
	if r.Requester == actor.OwnerKey {
		return true, nil
	}
	return RequestHasEvent(q, PausePassed, r.ID)
}

// PauseHeldTasks are the tasks queued for agent that the pause keeps from it at now, oldest first: the agent owns them, neither they nor
// their delivery is told, the delivery does not pass the pause, and nothing else holds them (a blocker, a person's or a gate's turn).
// None when not paused.
func PauseHeldTasks(q Q, agent string, now time.Time) ([]int64, error) {
	if p, err := GetPause(q, now); p == nil || err != nil {
		return nil, err
	}
	rows, err := QueuedAwaiting(q, agent)
	if err != nil {
		return nil, err
	}
	var out []int64
	for _, r := range rows {
		if r.Kind.String != "task" || r.DeliveredAt.Valid {
			continue
		}
		if ok, err := PassesPause(q, r); ok || err != nil {
			if err != nil {
				return nil, err
			}
			continue
		}
		ts, err := TasksForRequest(q, r.ID)
		if err != nil {
			return nil, err
		}
		for _, t := range ts {
			if t.Owner != agent || t.DeliveredAt.Valid || t.Acting == actor.OwnerKey || t.Acting == Gate {
				continue
			}
			if bs, err := TaskBlockers(q, t.ID); err != nil || len(bs) > 0 {
				if err != nil {
					return nil, err
				}
				continue // its blockers hold it, not the pause
			}
			out = append(out, t.ID) // every task of the agent's on the delivery: send_now on one lets the delivery go
		}
	}
	return out, nil
}
