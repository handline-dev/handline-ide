package reads

import (
	"encoding/json"
	"github.com/handline/handline/go/scheduler/settings"
	"github.com/handline/handline/go/scheduler/worktreepaths"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/internal/housekeeping"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/keepawake"
	"github.com/handline/handline/go/scheduler/leases"
	"github.com/handline/handline/go/scheduler/loadgate"
	"github.com/handline/handline/go/taskboard/handoff"
)

// perAgent is rpc_settings.PER_AGENT: the settings an agent.yaml may override.
var perAgent = []string{"park_timeout_s", "ignore_cache_expiry", "bg_notice_s", "bg_grace_s", "bg_check_s", "notice_digest_s", handoff.PromptKey}

// hooksWanted is the events our hook is installed for: claudecode.Wanted, which a test keeps equal.
var hooksWanted = []string{"SessionStart", "SessionEnd", "Stop", "StopFailure", "SubagentStop", "PostCompact", "UserPromptSubmit",
	"PermissionRequest", "TeammateIdle", "Notification", "PostToolUse", "PostToolUseFailure", "PreToolUse"}

// isOurHook is claude_hooks._is_our_hook: `<exe> hook <event>`, the exe being one of ours at any path.
func isOurHook(command any, event string) bool {
	s, ok := command.(string)
	if !ok {
		return false
	}
	parts := strings.Fields(s)
	return len(parts) >= 3 && parts[len(parts)-2] == "hook" && parts[len(parts)-1] == event && slices.Contains(paths.Commands, filepath.Base(parts[0]))
}

// hooksInstalled is rpc_lists._hooks_installed: every event wanted has an entry of ours in the shared settings. A settings file that
// cannot be read is "not installed", not a failed call.
func (e *Env) hooksInstalled() bool {
	b, err := os.ReadFile(filepath.Join(e.Home, "shared", ".claude", "settings.json"))
	var data map[string]any
	if err != nil || json.Unmarshal(b, &data) != nil || data == nil {
		data = map[string]any{}
	}
	hooks, _ := data["hooks"].(map[string]any)
	have := map[string]bool{}
	for event, groups := range hooks {
		list, ok := groups.([]any)
		if !ok {
			continue
		}
	groups:
		for _, g := range list {
			gm, ok := g.(map[string]any)
			if !ok {
				continue
			}
			inner, _ := gm["hooks"].([]any)
			for _, h := range inner {
				if hm, ok := h.(map[string]any); ok && isOurHook(hm["command"], event) {
					have[event] = true
					break groups
				}
			}
		}
	}
	for _, w := range hooksWanted {
		if !have[w] {
			return false
		}
	}
	return true
}

// declaredWorktrees is details.eligible_worktrees' declared half: every registered agent's worktrees inside a root the founder declared.
func (e *Env) declaredWorktrees() ([][2]string, error) {
	trust := team.NewTrust(e.Home)
	var out [][2]string
	for _, name := range paths.ListAgents(e.Home) {
		row, err := db.GetAgentState(e.DB, name)
		if err != nil {
			return nil, err
		}
		for _, wt := range worktreepaths.AllPaths(row) {
			if trust.IsDeclared(wt) {
				out = append(out, [2]string{name, wt})
			}
		}
	}
	return out, nil
}

// settingsValues is rpc_lists._values: every setting the screen shows, with what it stands at now.
func (e *Env) settingsValues() map[string]any {
	cfg := usage.LoadConfig(e.Home)
	s := &settings.Settings{Home: e.Home}
	park := s.DefaultCloseTimeoutS()
	limits := loadgate.LimitsOf(cfg)
	pace := usage.PaceSettingsOf(cfg)
	bgNotice := park
	if bgNotice == 0 {
		bgNotice = 1
	}
	prices := map[string]any{}
	if m, ok := cfg["token_prices"].(map[string]any); ok {
		for k, v := range m {
			prices[k] = v
		}
	}
	return map[string]any{
		"active_cap": cfg.DefaultCap(), "park_timeout_s": park, "handoff_note": s.Flag("handoff_note", true), "ignore_cache_expiry": cfg["ignore_cache_expiry"] == true,
		"dev_mode":    DevMode(cfg),
		"bg_notice_s": s.ConfigSeconds("bg_notice_s", bgNotice), "bg_grace_s": s.ConfigSeconds("bg_grace_s", settings.BgGraceS),
		"bg_check_s": s.ConfigSeconds("bg_check_s", settings.BgCheckS), "notice_digest_s": s.ConfigSeconds("notice_digest_s", settings.NoticeDigestS),
		"token_prices":                prices,
		"input_hold_idle_s":           s.ConfigSeconds("input_hold_idle_s", defaultHoldIdleS),
		settings.AttentionWaitKey:     s.ConfigSeconds(settings.AttentionWaitKey, settings.AttentionWaitS),
		settings.AttentionQuietKey:    s.ConfigSeconds(settings.AttentionQuietKey, settings.AttentionQuietS),
		settings.AttentionSentBackKey: s.ConfigSeconds(settings.AttentionSentBackKey, settings.AttentionSentBack),
		settings.BlockerAlarmKey:      s.Flag(settings.BlockerAlarmKey, false),
		settings.BlockerAlarmHoldKey:  s.ConfigSeconds(settings.BlockerAlarmHoldKey, settings.BlockerAlarmHoldS),
		settings.BlockerAlarmLeadKey:  s.ConfigSeconds(settings.BlockerAlarmLeadKey, settings.BlockerAlarmLeadS),
		handoff.PromptKey:             handoffPrompt(s.TextSetting("", handoff.PromptKey)),
		// Whether each notifications event raises a notice on the founder's devices; the event is on the topic either way.
		"push_prompt_asked": s.Flag("push_prompt_asked", true), "push_task_done": s.Flag("push_task_done", true),
		"push_stall": s.Flag("push_stall", true), "push_account_out": s.Flag("push_account_out", true),
		loadgate.AgentCapKey: limits.Agents, loadgate.LoadPctKey: limits.LoadPct, loadgate.RAMFreePctKey: limits.RAMFreePct, loadgate.DiskFreeGBKey: limits.DiskFreeGB,
		keepawake.Key: keepawake.ModeOf(cfg), housekeeping.OrphansKey: housekeeping.OrphansModeOf(cfg),
		usage.LaunchHoldKey: pace.On, usage.PaceTargetKey: pace.Target, usage.PaceBurstKey: pace.Burst,
		usage.SubagentGuardKey:          s.Flag(usage.SubagentGuardKey, false),
		usage.MoveBeforeKey:             s.Percent(usage.MoveBeforeKey),
		settings.ClearPastBackgroundKey: s.Flag(settings.ClearPastBackgroundKey, false),
		settings.TranscriptRowsKey:      s.Flag(settings.TranscriptRowsKey, false),
		settings.RefsNeedGrantKey:       s.Flag(settings.RefsNeedGrantKey, false),
		leases.SimulatorMaxKey:          leases.Max(cfg, leases.Simulator), leases.BrowserMaxKey: leases.Max(cfg, leases.Browser),
		usage.NetworkAddressesKey: hostsSetting(cfg[usage.NetworkAddressesKey]),
	}
}

// hostsSetting is network_addresses as written: the list of hosts, or nil when unset (the network listener serves -listen).
func hostsSetting(v any) any {
	l, ok := v.([]any)
	if !ok {
		return nil
	}
	out := []any{}
	for _, h := range l {
		if s, ok := h.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// DevMode is whether config turns dev mode on (dev_mode in config.yaml): the daemon restarts itself when its own code changed.
func DevMode(cfg usage.Config) bool { return truthy(cfg["dev_mode"]) }

// handoffPrompt is the handoff prompt as set, or the built-in one it falls back to.
func handoffPrompt(set string) string {
	if set == "" {
		return handoff.AskTemplate
	}
	return set
}

// ownHandoffPrompts is the agents whose agent.yaml sets a handoff prompt of their own, which the global one does not reach.
func (e *Env) ownHandoffPrompts() []string {
	roster := team.NewRoster(e.Home)
	own := []string{}
	for _, a := range paths.ListAgents(e.Home) {
		cfg, _ := roster.Read(a)
		if s, _ := cfg[handoff.PromptKey].(string); strings.TrimSpace(s) != "" {
			own = append(own, a)
		}
	}
	return own
}

// settingsGet is settings.get: timings, terminal host, listener, trust folders, hooks and every value.
func (e *Env) settingsGet(_ *rpc.Conn, params map[string]any) (any, error) {
	agent, err := e.chatAgent(params, false)
	if err != nil {
		return nil, err
	}
	values := e.settingsValues()
	declared, err := e.declaredWorktrees()
	if err != nil {
		return nil, err
	}
	installed := e.hooksInstalled()
	names := paths.ListAccounts(e.Home)
	timings := map[string]any{}
	for k, v := range values {
		if strings.HasSuffix(k, "_s") {
			timings[k] = v
		}
	}
	folders := []any{}
	accounts := []any{}
	for _, a := range names {
		for _, d := range declared {
			folders = append(folders, map[string]any{"account": a, "agent": d[0], "path": d[1]})
		}
		accounts = append(accounts, map[string]any{"account": a, "installed": installed})
	}
	out := map[string]any{
		"timings": timings,
		// This daemon drives no terminal host yet, as daemon.status says.
		"terminal_host": map[string]any{"choice": "none", "available": []any{}, "health": "down"},
		"listener":      map[string]any{"enabled": false, "bind": "loopback", "encryption": "none", "fingerprint": nil},
		"trust_folders": folders,
		"hooks":         map[string]any{"installed": installed, "accounts": accounts},
		"values":        values,
	}
	if agent != "" {
		cfg, _ := team.NewRoster(e.Home).Read(agent)
		over := map[string]any{}
		for _, k := range perAgent {
			if v, ok := cfg[k]; ok {
				over[k] = v
			}
		}
		out["overrides"] = over
	} else {
		out["handoff_prompt_default"] = handoff.AskTemplate
		out["handoff_prompt_own"] = e.ownHandoffPrompts()
	}
	return out, nil
}
