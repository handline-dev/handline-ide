package db

import (
	"encoding/json"
	"strings"
)

// NoRoomPrefix keys, per account, that no other account can take its agents (founder_noroom:<acct>). The tick keeps it while the
// account's stall, end or pause leaves them nowhere to go and clears it once that is over; the founder reads it on the account's warning.
const NoRoomPrefix = "founder_noroom:"

// NoRoom is since when acct's agents have had nowhere to go, why (login, ended or paused), and why no other account has room.
type NoRoom struct {
	Since int64  `json:"since"`
	Kind  string `json:"kind"`
	Room  string `json:"room"`
}

// ReadNoRoom is the NoRoom a meta value holds; ok is false for a value that is not one.
func ReadNoRoom(value string) (n NoRoom, ok bool) {
	return n, json.Unmarshal([]byte(value), &n) == nil && n.Since > 0
}

// SetNoRoom keeps n for acct, writing only when it changed. A row of the same kind keeps its since: the room changes as windows move.
func SetNoRoom(q Q, acct string, n NoRoom) error {
	v, _, err := Meta(q, NoRoomPrefix+acct)
	if err != nil {
		return err
	}
	if kept, ok := ReadNoRoom(v); ok && kept.Kind == n.Kind {
		n.Since = kept.Since
	}
	raw, err := json.Marshal(n)
	if err != nil || v == string(raw) {
		return err
	}
	return SetMeta(q, NoRoomPrefix+acct, string(raw))
}

// NoRooms is each account's NoRoom, by account.
func NoRooms(q Q) (map[string]NoRoom, error) {
	names, err := MetaNames(q, NoRoomPrefix)
	if err != nil {
		return nil, err
	}
	out := map[string]NoRoom{}
	for _, name := range names {
		v, _, err := Meta(q, name)
		if err != nil {
			return nil, err
		}
		if n, ok := ReadNoRoom(v); ok {
			out[strings.TrimPrefix(name, NoRoomPrefix)] = n
		}
	}
	return out, nil
}

// ClearNoRoom drops acct's NoRoom.
func ClearNoRoom(q Q, acct string) error { return ClearMeta(q, NoRoomPrefix+acct) }
