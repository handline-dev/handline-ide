package adminwrites

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/internal/housekeeping"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/kit/yamlcompat"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/keepawake"
	"github.com/handline/handline/go/scheduler/leases"
	"github.com/handline/handline/go/scheduler/loadgate"
	"github.com/handline/handline/go/scheduler/settings"
	"github.com/handline/handline/go/taskboard/handoff"
)

// settings.set: the founder's timings and switches, config.yaml's own names, written in one step so nothing is written if any key is refused.
// A key set to null is removed (back to the default).

const (
	int1   = "int>=1"
	int0   = "int>=0"
	pct    = "int 0-100"
	tgt    = "int 50-100"
	burst  = "int 0-50"
	flag_  = "bool"
	line   = "line"
	awake  = "off, on or working"
	reaper = "dry, on or off"
	split  = "auto or manual"
	hosts  = "hosts"
	ports  = "lo-hi"
)

// HostsMax is how many hosts network_addresses may name.
const HostsMax = 16

// LineMax is how long a one-line text setting may be, in characters.
const LineMax = 4000

// PromptMin is the shortest handoff prompt kept, in characters; a blank one means the built-in.
const PromptMin = 20

var settingKinds = map[string]string{"active_cap": int1, "park_timeout_s": int0, "handoff_note": flag_,
	"ignore_cache_expiry": flag_, "dev_mode": flag_, "bg_notice_s": int1, "bg_grace_s": int1, "bg_check_s": int1, "notice_digest_s": int1,
	"input_hold_idle_s": int1, handoff.PromptKey: line,
	settings.AttentionWaitKey: int1, settings.AttentionQuietKey: int1, settings.AttentionSentBackKey: int1,
	settings.BlockerAlarmKey: flag_, settings.BlockerAlarmHoldKey: int1, settings.BlockerAlarmOpenerKey: int1,
	"push_prompt_asked": flag_, "push_task_done": flag_, "push_stall": flag_, "push_account_out": flag_,
	keepawake.Key: awake, housekeeping.OrphansKey: reaper,
	loadgate.AgentCapKey: int0, loadgate.LoadPctKey: int0, loadgate.RAMFreePctKey: pct, loadgate.DiskFreeGBKey: int0,
	usage.PaceTargetKey: tgt, usage.PaceBurstKey: burst, usage.SubagentGuardKey: flag_, usage.MoveBeforeKey: pct, usage.SplitKey: split,
	settings.ClearPastBackgroundKey: flag_, settings.TranscriptRowsKey: flag_, settings.RefsNeedGrantKey: flag_,
	leases.HeavyMaxKey: int0, leases.SimulatorMaxKey: int0, leases.BrowserMaxKey: int0, leases.PortRangeKey: ports,
	usage.NetworkAddressesKey: hosts}

var perAgent = map[string]bool{"park_timeout_s": true, "ignore_cache_expiry": true, "bg_notice_s": true, "bg_grace_s": true, "bg_check_s": true,
	"notice_digest_s": true, handoff.PromptKey: true}

// joinLines is a handoff prompt as one line: the line breaks typed in the box become one space, and the ends are trimmed.
func joinLines(s string) string {
	var parts []string
	for _, l := range strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == '\r' }) {
		if l = strings.TrimSpace(l); l != "" {
			parts = append(parts, l)
		}
	}
	return strings.Join(parts, " ")
}

// hostList is v as a list of hosts: at most HostsMax, each named once, none blank.
func hostList(v any) ([]any, bool) {
	l, ok := v.([]any)
	if !ok || len(l) > HostsMax {
		return nil, false
	}
	seen := map[string]bool{}
	out := []any{}
	for _, h := range l {
		s, ok := h.(string)
		if !ok || strings.TrimSpace(s) == "" || textsafe.HasUnsafe(s) || seen[s] {
			return nil, false
		}
		seen[s] = true
		out = append(out, s)
	}
	return out, true
}

// settingValue is v if it is what key takes, as the value to write.
func settingValue(key string, v any) (any, error) {
	if s, ok := v.(string); ok && key == handoff.PromptKey {
		s = joinLines(s)
		if n := utf8.RuneCountInString(s); n > 0 && n < PromptMin {
			return nil, badParam("Too short to guide a note. Leave it blank for the built-in.", key)
		}
		v = s
	}
	kind := settingKinds[key]
	switch kind {
	case flag_:
		if b, ok := v.(bool); ok {
			return b, nil
		}
	case awake:
		if m, ok := v.(string); ok && (m == keepawake.Off || m == keepawake.On || m == keepawake.Working) {
			return m, nil
		}
	case split:
		if m, ok := v.(string); ok && (m == usage.SplitAuto || m == usage.SplitManual) {
			return m, nil
		}
	case reaper:
		if m, ok := v.(string); ok && (m == housekeeping.OrphansDry || m == housekeeping.OrphansOn || m == housekeeping.OrphansOff) {
			return m, nil
		}
	case hosts:
		if l, ok := hostList(v); ok {
			return l, nil
		}
	case ports:
		if s, ok := v.(string); ok {
			if lo, hi, ok := leases.ParsePortRange(s); ok {
				return fmt.Sprintf("%d-%d", lo, hi), nil
			}
		}
	case line:
		// config.yaml holds one line: a newline or another control character is refused, not written.
		if s, ok := v.(string); ok && !textsafe.HasUnsafe(s) && utf8.RuneCountInString(s) <= LineMax {
			return s, nil
		}
	default:
		lo, hi := map[string]int64{int1: 1, tgt: 50}[kind], map[string]int64{pct: 100, tgt: 100, burst: 50}[kind]
		if n, ok := rpc.AsInt(v); ok && n >= lo && (hi == 0 || n <= hi) {
			return n, nil
		}
	}
	what := map[string]string{int1: "a whole number, at least 1", int0: "a whole number, at least 0", pct: "a whole number from 0 to 100",
		tgt: "a whole number from 50 to 100", burst: "a whole number from 0 to 50",
		flag_: "true or false", awake: "off, on or working", reaper: "dry, on or off",
		line:  fmt.Sprintf("one line of text, at most %d characters", LineMax),
		hosts: fmt.Sprintf("a list of at most %d different hosts from connection.addresses", HostsMax),
		ports: "a range of ports like 4000-4999, from 1 to 65535"}[kind]
	return nil, badParam(fmt.Sprintf("%s is %s, or null", key, what), key)
}

func (e *Env) settingsSet(c *rpc.Conn, p map[string]any) (any, error) {
	changes, ok := p["changes"].(map[string]any)
	if !ok || len(changes) == 0 {
		return nil, badParam("changes is an object of settings", "changes")
	}
	keys := make([]string, 0, len(changes))
	for k := range changes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	agent, hasAgent := p["agent"].(string)
	values := map[string]any{}
	for _, key := range keys {
		v := changes[key]
		if key == "token_prices" && v != nil {
			return nil, badParam("token_prices is not served yet: config.yaml holds no nested value", key)
		}
		if _, known := settingKinds[key]; !known && key != "token_prices" {
			return nil, badParam(key+" is not a setting", "changes")
		}
		if v != nil {
			w, err := settingValue(key, v)
			if err != nil {
				return nil, err
			}
			values[key] = w
		}
		if p["agent"] != nil && !perAgent[key] {
			return nil, badParam(key+" has no per-agent value", "changes")
		}
	}
	if l, ok := values[usage.NetworkAddressesKey].([]any); ok && e.Network != nil {
		hs := make([]string, len(l))
		for i, h := range l {
			hs[i] = h.(string)
		}
		if err := e.Network.Known(hs); err != nil {
			return nil, err
		}
	}
	if p["agent"] != nil {
		if !hasAgent || agent == "" {
			return nil, badParam("agent is an agent's name", "agent")
		}
		if _, err := e.agentNamed(p, "agent"); err != nil {
			return nil, err
		}
	}
	if hasAgent {
		var set []yamlcompat.Pair
		for _, k := range keys {
			set = append(set, yamlcompat.Pair{Key: k, Val: values[k]})
		}
		if err := team.NewRoster(e.Home).SetKeys(agent, set); err != nil {
			return nil, refused(err.Error(), "state")
		}
	} else {
		cfg, err := e.readConfig()
		if err != nil {
			return nil, refused(err.Error(), "state")
		}
		for _, k := range keys {
			cfg.set(k, values[k])
		}
		if err := cfg.save(); err != nil {
			return nil, refused(err.Error(), "state")
		}
	}
	said := make([]string, len(keys))
	for i, k := range keys {
		word := "set"
		if changes[k] == nil {
			word = "cleared"
		}
		said[i] = k + " " + word
	}
	out := strings.Join(said, ", ")
	if hasAgent {
		out += " for " + agent
	}
	var more map[string]any
	if _, moved := changes[usage.NetworkAddressesKey]; moved && e.Network != nil {
		more = map[string]any{"unreachable_devices": e.Network.Relisten()}
	}
	return wrote(c, out, more)
}
