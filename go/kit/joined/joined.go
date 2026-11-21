// Package joined is the master a home joined (hl device join), kept in the home's protected store, out of an agent's reach to write.
package joined

import (
	"encoding/json"

	"github.com/handline/handline/go/kit/protected"
)

const ns, name = "master", "credential"

// Master is the master's address, the certificate fingerprint pinned at join ("" for a plain door), the device token that is this
// machine's credential there, its device id, and when it joined; Node and Number are the master's node id and master number as its
// hello last said them (0 before one did).
type Master struct {
	Addr        string `json:"master"`
	Fingerprint string `json:"fp"`
	Token       string `json:"token"`
	Device      string `json:"device"`
	At          int64  `json:"at"`
	Node        string `json:"node,omitempty"`
	Number      int64  `json:"number,omitempty"`
}

// Of is the master home joined; false when it joined none.
func Of(home string) (Master, bool) {
	if home == "" {
		return Master{}, false
	}
	return OfStore(protected.New(home))
}

// OfStore is Of read from s, the home's protected store.
func OfStore(s *protected.Store) (Master, bool) {
	raw, ok := s.Get(ns, name)
	if !ok {
		return Master{}, false
	}
	var m Master
	if err := json.Unmarshal([]byte(raw), &m); err != nil || m.Token == "" {
		return Master{}, false
	}
	return m, true
}

// Keep keeps m as the master home joined.
func Keep(home string, m Master) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return protected.New(home).Put(ns, name, string(b))
}

// Forget drops the master home joined.
func Forget(home string) error { return protected.New(home).Delete(ns, name) }

// ownName is where a master keeps its own device's token: the board holds its hash, so the copy carries it, and the master that takes
// over knows it. It is the token this machine joins with when another master outranks it.
const ownName = "own"

// Own is a master's own device: its token and id.
type Own struct {
	Token  string `json:"token"`
	Device string `json:"device"`
}

// OwnOf is the master's own device home keeps; false when it keeps none.
func OwnOf(home string) (Own, bool) {
	raw, ok := protected.New(home).Get(ns, ownName)
	if !ok {
		return Own{}, false
	}
	var o Own
	if err := json.Unmarshal([]byte(raw), &o); err != nil || o.Token == "" {
		return Own{}, false
	}
	return o, true
}

// KeepOwn keeps o as the master's own device.
func KeepOwn(home string, o Own) error {
	b, err := json.Marshal(o)
	if err != nil {
		return err
	}
	return protected.New(home).Put(ns, ownName, string(b))
}

// Become makes home a runner of m: m kept as the master it joined, its own device forgotten, as m.Token is now that device's token there.
func Become(home string, m Master) error {
	if err := Keep(home, m); err != nil {
		return err
	}
	return protected.New(home).Delete(ns, ownName)
}

// Lead makes home a master: the token it joined with is kept as its own device, and the master it joined forgotten.
func Lead(home string, m Master) error {
	if err := KeepOwn(home, Own{Token: m.Token, Device: m.Device}); err != nil {
		return err
	}
	return Forget(home)
}
