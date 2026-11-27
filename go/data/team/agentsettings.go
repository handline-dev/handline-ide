package team

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/kit/yamlcompat"
)

// What an agent's settings are and how they are written: the one place the chat page's
// Settings tab, the Proposals tab (`hl propose settings`) and agent.set all read and write them. The model names the code prices
// by name, and the idle-close switch (which belongs to the daemon's own settings), are passed in.

var (
	Editable = []string{"model", "effort", "weight", "repo", "browser",
		"context_k", "idle_close", "handoff_note", "sandbox", "sandbox_allow", "mode", "device", "session_mode"}
	// Optional fields are not on every form: the settings tab does not draw them yet, so a form without them leaves them be.
	Optional      = map[string]bool{"sandbox": true, "sandbox_allow": true, "mode": true, "device": true, "session_mode": true}
	// SessionModes are how an agent's sessions run: in a terminal, or headless with the chat as their only view.
	SessionModes = []string{"terminal", "chat"}
	BrowserModes  = []string{"off", "chrome"}
	BrowserCustom = "custom"
	Weights       = []string{"light", "heavy"}
	HandoffNotes  = []string{"default", "on", "off"}
	// Ordinary is the one split between what an agent may set on its own and what raises what it may do: only these apply when
	// an agent asks; every other field, and any added later, waits for the founder as a proposal.
	Ordinary = map[string]bool{"model": true, "effort": true, "context_k": true, "handoff_note": true}
)

// Nothing is said when a change is already so.
const Nothing = "nothing to change"

// Privileged is whether a field raises what the agent may do.
func Privileged(field string) bool { return !Ordinary[field] }

// Settings is one agent's configuration as the settings tab shows it.
type Settings map[string]string

// SettingsOf is what the agent file says and the account the session is on. modelID is the model the account's status
// reports ("" when none); idleMinutes shows a park timeout in seconds as the minutes the form takes.
func (r *Roster) SettingsOf(db *sql.DB, agent, modelID string, idleMinutes func(int64) string) (Settings, error) {
	cfg, err := r.Read(agent)
	if err != nil {
		return nil, err
	}
	account, err := CurrentAccount(db, agent)
	if err != nil {
		return nil, err
	}
	onOff := func(b bool) string {
		if b {
			return "on"
		}
		return "off"
	}
	note := "default"
	if v, ok := cfg["handoff_note"].(bool); ok {
		note = onOff(v)
	}
	idle := ""
	if n, ok := cfg["park_timeout_s"].(int64); ok {
		idle = idleMinutes(n)
	}
	enabled := true
	if v, has := cfg["enabled"]; has {
		enabled = truthy(v)
	}
	vendor, model, effort := Parts(cfg)
	return Settings{"model_id": modelID, "session_account": account,
		"vendor": vendor, "enabled": strconv.FormatBool(enabled), "model": model, "effort": effort,
		"weight": orDefault(cfg, "weight", "light"), "repo": orDefault(cfg, "repo", ""),
		"sandbox": onOff(r.SandboxOn(agent, cfg)), "sandbox_allow": SandboxAllowText(SandboxEntries(cfg)),
		"device": orDefault(cfg, "device", ""), "session_mode": SessionMode(cfg), "mode": r.ModeOf(agent, cfg), "browser": BrowserMode(cfg),
		"context_k": strconv.FormatInt(ContextTokensOf(cfg)/1000, 10), "handoff_note": note, "idle_close": idle}, nil
}

func allDigits(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}

// SettingsProblem is why form cannot be saved, or "". Checked here so nothing is written that the agent file would refuse to load again.
func SettingsProblem(form map[string]string) string {
	model, repo := strings.TrimSpace(form["model"]), strings.TrimSpace(form["repo"])
	switch {
	case model == "":
		return "model cannot be empty"
	case textsafe.HasUnsafe(model) || !yamlcompat.IsPlainText(model):
		return fmt.Sprintf("model %s would not be read back as text", quote(model))
	case !contains(ValidEffort, form["effort"]):
		return "effort must be one of " + strings.Join(ValidEffort, ", ")
	case !contains(Weights, form["weight"]):
		return "weight must be one of " + strings.Join(Weights, ", ")
	}
	if v, has := form["sandbox"]; has && v != "on" && v != "off" {
		return "sandbox must be on or off"
	}
	if v, has := form["sandbox_allow"]; has {
		if why := SandboxAllowProblem(v); why != "" {
			return why
		}
	}
	if v, has := form["device"]; has && v != "" && !deviceID.MatchString(v) {
		return "device is a device's node id, or empty for none"
	}
	if v, has := form["session_mode"]; has && v != "" && !contains(SessionModes, v) {
		return "session_mode must be one of " + strings.Join(SessionModes, ", ")
	}
	if v, has := form["mode"]; has && !contains(Modes, v) {
		return "mode must be one of " + strings.Join(Modes, ", ")
	}
	if !contains(BrowserModes, form["browser"]) && form["browser"] != BrowserCustom {
		return "browser must be off or chrome"
	}
	if !contains(HandoffNotes, form["handoff_note"]) {
		return "handoff note must be on, off or default"
	}
	if k := strings.TrimSpace(form["context_k"]); !allDigits(k) || mustAtoi(k) < 1 {
		return "context must be a whole number of thousands of tokens, at least 1"
	}
	if idle := strings.TrimSpace(form["idle_close"]); idle != "" && !allDigits(idle) {
		return "idle close must be a whole number of minutes (0 = never), or blank to follow the global time"
	}
	if repo != "" {
		if st, err := os.Stat(expand(repo)); textsafe.HasUnsafe(repo) || err != nil || !st.IsDir() {
			return fmt.Sprintf("repo %s is not a folder", quote(repo))
		}
	}
	return ""
}

// deviceID is what a node id may look like.
var deviceID = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

func mustAtoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// Change is one editable field a form changes.
type Change struct{ Field, Old, New string }

// SettingsChanges is (field, old, new) for each editable field the form changes. A blank repo means leave it: the agent file
// cannot be told to forget one.
func SettingsChanges(current Settings, form map[string]string) ([]Change, error) {
	var out []Change
	for _, key := range Editable {
		if _, has := form[key]; Optional[key] && !has {
			continue
		}
		next := strings.TrimSpace(form[key])
		if key == "repo" && next == "" {
			continue
		}
		if key == "sandbox_allow" { // each entry once, in order, so the same list is no change
			next = NormalSandboxAllow(next)
		}
		if next != current[key] {
			out = append(out, Change{key, current[key], next})
		}
	}
	return out, nil
}

// FormWith is the current settings and the form once values are put over them; an error for a field that is not a setting.
func (r *Roster) FormWith(db *sql.DB, agent string, values map[string]string, modelID string, idleMinutes func(int64) string) (Settings, map[string]string, error) {
	var unknown []string
	for k := range values {
		if !contains(Editable, k) {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sortStrings(unknown)
		return nil, nil, fmt.Errorf("not a setting: %s (settings: %s)", strings.Join(unknown, ", "), strings.Join(Editable, ", "))
	}
	current, err := r.SettingsOf(db, agent, modelID, idleMinutes)
	if err != nil {
		return nil, nil, err
	}
	form := map[string]string{}
	for _, k := range Editable {
		form[k] = current[k]
	}
	for k, v := range values {
		form[k] = v
	}
	return current, form, nil
}

// IdleClose is the daemon's own idle-close setting, which a form may change.
type IdleClose interface {
	Parse(text string) error
	Set(text, agent string) (bool, string)
}

// SaveSettings writes changes to the agent's row through Register, which keeps every other field as it is. A running session is
// not restarted.
func (r *Roster) SaveSettings(agent string, changes []Change, idle IdleClose) (bool, string) {
	next := map[string]string{}
	for _, c := range changes {
		next[c.Field] = c.New
	}
	idleVal, hasIdle := next["idle_close"]
	delete(next, "idle_close")
	note, hasNote := next["handoff_note"]
	delete(next, "handoff_note")
	if hasIdle {
		if err := idle.Parse(idleVal); err != nil { // a bad one is refused before the file is written
			return false, "not saved: " + err.Error()
		}
	}
	if len(next) > 0 {
		o := RegisterOpts{Weight: next["weight"], Repo: next["repo"]}
		if v, ok := next["model"]; ok {
			o.Model = strp(v)
		}
		if v, ok := next["effort"]; ok {
			o.Effort = strp(v)
		}
		if v, ok := next["mode"]; ok {
			o.Mode = strp(v)
		}
		if v, ok := next["sandbox"]; ok {
			b := v == "on"
			o.Sandbox = &b
		}
		if v, ok := next["sandbox_allow"]; ok {
			o.SandboxAllow = strp(v)
		}
		if v, ok := next["device"]; ok {
			o.Device = strp(v)
		}
		if v, ok := next["session_mode"]; ok {
			o.SessionMode = strp(v)
		}
		if v, ok := next["context_k"]; ok {
			n := int64(mustAtoi(v)) * 1000
			o.ContextTokens = &n
		}
		if mode, ok := next["browser"]; ok {
			cfg, err := r.Read(agent)
			if err != nil {
				return false, "not saved: " + err.Error()
			}
			chrome := mode == "chrome"
			mcp, err := BrowserMCPConfig(cfg, mode)
			if err != nil {
				return false, "not saved: " + err.Error()
			}
			o.Chrome, o.MCPConfig = &chrome, &mcp
		}
		if _, err := r.Register(agent, o); err != nil {
			return false, "not saved: " + err.Error()
		}
	}
	if hasNote {
		var v any // default: no setting of its own
		if note != "default" {
			v = note == "on"
		}
		if err := r.SetKeys(agent, []yamlcompat.Pair{{Key: "handoff_note", Val: v}}); err != nil {
			return false, "handoff note not saved: " + err.Error()
		}
	}
	if hasIdle {
		if ok, said := idle.Set(idleVal, agent); !ok {
			var done []string
			for _, c := range changes {
				if c.Field != "idle_close" && c.Field != "handoff_note" {
					done = append(done, c.Field+" "+c.New)
				}
			}
			msg := "idle close not saved: " + said
			if len(done) > 0 {
				msg += " (" + strings.Join(done, ", ") + " was saved)"
			}
			return false, msg
		}
	}
	parts := make([]string, len(changes))
	for i, c := range changes {
		parts[i] = c.Field + " " + c.New
	}
	return true, "saved: " + strings.Join(parts, ", ")
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// ChangesFor is what values would change, and why it cannot be saved ("" when it can): checked the way the Settings tab checks its
// form.
func (r *Roster) ChangesFor(db *sql.DB, agent string, values map[string]string, modelID string, idleMinutes func(int64) string) ([]Change, string) {
	current, form, err := r.FormWith(db, agent, values, modelID, idleMinutes)
	if err != nil {
		return nil, err.Error()
	}
	if problem := SettingsProblem(form); problem != "" {
		return nil, problem
	}
	all, err := SettingsChanges(current, form)
	if err != nil {
		return nil, err.Error()
	}
	var changes []Change
	for _, c := range all {
		if _, asked := values[c.Field]; asked {
			changes = append(changes, c)
		}
	}
	if len(changes) == 0 {
		return nil, Nothing + ": it is already so"
	}
	return changes, ""
}

// SetValues writes values through the register path, which only the founder's own session may take.
func (r *Roster) SetValues(db *sql.DB, agent string, values map[string]string, modelID string, idleMinutes func(int64) string, idle IdleClose) (bool, string) {
	changes, problem := r.ChangesFor(db, agent, values, modelID, idleMinutes)
	if problem != "" {
		return false, "not saved: " + problem
	}
	return r.SaveSettings(agent, changes, idle)
}

// SetByAgent is an agent's own change, applied for it by the daemon: only its own, and only what Ordinary names. speaker is the
// caller's own word, as for ask and say: this guards against mistakes, not a hostile local process.
func (r *Roster) SetByAgent(db *sql.DB, agent string, values map[string]string, speaker string, modelID string, idleMinutes func(int64) string, idle IdleClose) (bool, string) {
	if speaker != agent {
		who := speaker
		if who == "" {
			who = "someone"
		}
		return false, fmt.Sprintf("%s may not change %s's settings; an agent changes its own. For another's, ask the founder: `hl propose settings %s <setting>=<value> --why \"...\"`", addr.Ref(who), addr.Ref(agent), addr.Address(db, speaker, agent))
	}
	var held []string
	for k := range values {
		if Privileged(k) {
			held = append(held, k)
		}
	}
	if len(held) > 0 {
		sortStrings(held)
		asks := make([]string, len(held))
		for i, k := range held {
			asks[i] = k + "=<value>"
		}
		return false, fmt.Sprintf("%s needs the founder's approval: ask with `hl propose settings %s %s --why \"...\"`", strings.Join(held, ", "), addr.Address(db, speaker, agent), strings.Join(asks, " "))
	}
	return r.SetValues(db, agent, values, modelID, idleMinutes, idle)
}

// SessionMode is how the agent's sessions run, terminal unless it says chat.
func SessionMode(cfg Config) string {
	if orDefault(cfg, "session_mode", "") == "chat" {
		return "chat"
	}
	return "terminal"
}
