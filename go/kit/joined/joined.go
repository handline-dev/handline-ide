// Package joined is the master a home joined (hl device join), kept in the home's protected store, out of an agent's reach to write.
package joined

import (
	"encoding/json"

	"github.com/handline/handline/go/kit/protected"
)

const ns, name = "master", "credential"

// Master is the master's address, the certificate fingerprint pinned at join ("" for a plain door), the device token that is this
// machine's credential there, its device id, and when it joined.
type Master struct {
	Addr        string `json:"master"`
	Fingerprint string `json:"fp"`
	Token       string `json:"token"`
	Device      string `json:"device"`
	At          int64  `json:"at"`
}

// Of is the master home joined; false when it joined none.
func Of(home string) (Master, bool) {
	if home == "" {
		return Master{}, false
	}
	raw, ok := protected.New(home).Get(ns, name)
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
