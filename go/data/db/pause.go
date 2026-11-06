package db

import (
	"encoding/json"
	"time"
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
