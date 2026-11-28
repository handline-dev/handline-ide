package screenpipe

import (
	"encoding/json"
	"slices"
	"time"
)

var timeAfter = time.After

// num is a JSON number as it comes off a connection (json.Number) or as Go wrote it.
func num(v any) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

const maxText = 16384

// ParseInput is a ScreenInput from the wire.
func ParseInput(v any) (Input, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return Input{}, bad("event is a ScreenInput", "event")
	}
	str := func(k string) string { s, _ := m[k].(string); return s }
	ev := Input{Kind: str("kind"), Action: str("action"), Button: str("button"), Key: str("key"), Code: str("code"), Text: str("text")}
	ev.X, _ = num(m["x"])
	ev.Y, _ = num(m["y"])
	ev.DX, _ = num(m["dx"])
	ev.DY, _ = num(m["dy"])
	if n, ok := num(m["clicks"]); ok {
		ev.Clicks = int(n)
	}
	if n, ok := num(m["modifiers"]); ok {
		ev.Modifiers = int(n)
	}
	switch ev.Kind {
	case "mouse":
		if !slices.Contains([]string{"move", "down", "up"}, ev.Action) {
			return Input{}, bad("a mouse event's action is move, down or up", "event")
		}
		if ev.Action != "move" && !slices.Contains([]string{"left", "middle", "right"}, ev.Button) {
			return Input{}, bad("a mouse down or up names its button: left, middle or right", "event")
		}
	case "wheel":
	case "key":
		if ev.Action != "down" && ev.Action != "up" || ev.Key == "" {
			return Input{}, bad("a key event has action down or up and its key", "event")
		}
	case "text":
		if ev.Text == "" || len(ev.Text) > maxText {
			return Input{}, bad("a text event has 1 to 16384 bytes of text", "event")
		}
	default:
		return Input{}, bad("event kind is mouse, wheel, key or text", "event")
	}
	return ev, nil
}

// wire is ev as ParseInput reads it.
func (ev Input) wire() map[string]any {
	return map[string]any{"kind": ev.Kind, "action": ev.Action, "x": ev.X, "y": ev.Y, "dx": ev.DX, "dy": ev.DY, "button": ev.Button,
		"clicks": ev.Clicks, "key": ev.Key, "code": ev.Code, "text": ev.Text, "modifiers": ev.Modifiers}
}
