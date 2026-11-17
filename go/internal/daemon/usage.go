package daemon

import (
	"encoding/json"
	"maps"
	"time"
)

// UsageEvery is how often the accounts' usage is looked at, to tell the accounts topic of a change.
const UsageEvery = 5 * time.Second

// usageLoop tells the accounts topic each account whose row moved: it looks once at start, then every UsageEvery while someone is
// subscribed to the topic.
func (d *Daemon) usageLoop() {
	defer d.wg.Done()
	t := time.NewTicker(UsageEvery)
	defer t.Stop()
	for {
		if d.usageSeen == nil || d.hub.Subscribed("accounts") {
			d.sendUsageChanges()
		}
		select {
		case <-d.stop:
			return
		case <-t.C:
		}
	}
}

// sendUsageChanges sends account.changed, with the usage and roster fields, for each account whose row differs from the one last told.
// The first look sends nothing: a client's snapshot shows what is there. An account whose login moved while its row was read is left to
// the next look, so the topic never says an earlier login after a later one.
func (d *Daemon) sendUsageChanges() {
	d.acctMu.Lock()
	seq := maps.Clone(d.acctSeq)
	d.acctMu.Unlock()
	env := d.readsEnv()
	rows, err := env.AccountsWithUsage()
	if err != nil {
		d.cfg.Log("  stream: account usage: %v", err)
		return
	}
	d.acctMu.Lock()
	defer d.acctMu.Unlock()
	seen := make(map[string]string, len(rows))
	for _, a := range rows {
		name, _ := a["name"].(string)
		seen[name] = usageKey(a)
		if d.usageSeen == nil || d.usageSeen[name] == seen[name] {
			continue
		}
		if d.acctSeq[name] != seq[name] {
			seen[name] = d.usageSeen[name]
			continue
		}
		if err := d.hub.AccountChanged(a); err != nil {
			d.cfg.Log("  stream: account.changed %s: %v", name, err)
			seen[name] = d.usageSeen[name] // told at the next look
		}
	}
	d.usageSeen = seen
}

// usageKey is an account row without the age of its read and the pace's line and rate, which move with the clock, and the factors, which
// move with each cost sample and ride along on the next event.
func usageKey(a map[string]any) string {
	k := maps.Clone(a)
	delete(k, "age_s")
	delete(k, "factors")
	if p, ok := k["pace"].(map[string]any); ok {
		k["pace"] = p["state"]
	}
	b, _ := json.Marshal(k)
	return string(b)
}
