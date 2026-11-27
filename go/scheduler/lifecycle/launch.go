package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	cli "github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/verdict"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/version"
	"github.com/handline/handline/go/protocol/sessiontoken"
	"github.com/handline/handline/go/scheduler/access"
	"github.com/handline/handline/go/scheduler/worktreepaths"
	"github.com/handline/handline/go/taskboard/handoff"
)

// DisallowedTools is the tools a session may not use. Work for another agent goes through `hl task new`, where the bridge can see it:
// SendMessage reaches another agent's own session directly, bypassing the bridge. A subagent (Agent) is allowed for research or review
// inside the session's own task; the PreToolUse guard records it and refuses it when the account's pace says it would reach a limit. ScheduleWakeup and CronCreate: a self-scheduled wakeup survives the park's /clear and fires into a session at the usage
// wall, and Handline types the answer into the session when it comes, so nothing an agent schedules is needed. Build and the launch record name
// the same tools, or a change to one reads as a launch that never drifted to the other.
const DisallowedTools = "SendMessage,ScheduleWakeup,CronCreate"

// AccountStatus is the usage status of one account, as launch uses it. A nil trusted figure is one nothing can name.
type AccountStatus struct {
	Exists, Stale             bool
	AgeS                      *float64
	ShortTrusted, LongTrusted *int
	ShortLabel, LongLabel     string
	ShortReset, LongReset     int64 // 0: none
	// Unmetered: its vendor reports no usage, so no room can be read; it takes only the agents pinned to it.
	Unmetered bool
	// Paused: the founder paused it; it takes no agent, pinned or not.
	Paused bool
}

// UsagePort is the accounts' usage status and load as launch reads them.
type UsagePort interface {
	Read(acct string) AccountStatus
	ShortBlock() int
	LongBlock() int
	// PickAccount is the best of the candidates for agent (bootstrap: a never-run account is a real place to go; live refresh: this is
	// the one caller about to place an agent, so a carried or idle-floored candidate is asked fresh); "" when none.
	PickAccount(candidates []string, agent string) string
	// PickBeside is PickAccount for one more session like agent's beside the one it has: agent's own load counts.
	PickBeside(candidates []string, agent string) string
	// PickReasons is each candidate's line in the same pick, asked without a live refresh: why it was refused, or where it ranks.
	PickReasons(candidates []string, agent string) []usage.Reason
	// NotFor is whether the account is another vendor's than vendor.
	NotFor(acct, vendor string) bool
	// Admit is verdict.Admit's Fits tier for the account: nothing known says it is a bad place to start. A stale or missing reading
	// is the account nobody has run on lately, and the session about to start is the only thing that writes a fresh one.
	Admit(acct, agent, vendor string) verdict.Admission
	// Urgency is the account's unused weekly use per hour left (usage.Urgency); ok false where its week cannot be read.
	Urgency(acct string) (float64, bool)
	// ManualSplit is account_split manual: the daemon places and moves no agent; each stays on its account and waits at its wall.
	ManualSplit() bool
	// Headroom is what the account's weekly bucket still holds as a share of the week's quota, ok false when the pace cannot be read. The
	// larger, the slower-paced.
	Headroom(acct string) (float64, bool)
	// ActiveOn is how many agents are mid-turn on the account, agent itself left out: one counted
	// from its wake_reserved event until its Stop counts too. The pick spreads similar accounts by it.
	ActiveOn(q db.Q, acct, agent string) (int, error)
	// StartFit is the pick's test of the account for agent's start: why it is not a first choice ("" when it is), and the points left
	// under its nearer wall. A move ranks by it as a new start does.
	StartFit(acct, agent string) (unfit string, room int)
	// MidTurn is every agent mid-turn on any account, agent left out, as ActiveOn counts them; nil when it cannot be read.
	MidTurn(q db.Q, agent string) []string
}

// AccountsPort is the team's accounts as launch uses them.
type AccountsPort interface {
	MayStart(acct string) bool
	LoginCommand(acct string) (string, error)
	// EnvFrom is the environment of a session of agent on acct, starting from base: the caller's.
	EnvFrom(base map[string]string, acct, agent string) map[string]string
}

// VendorsPort is vendors/ as launch uses it.
type VendorsPort interface {
	ForAgent(agent string) (cli.Agent, error)
	AgentVendor(agent string) string
	AccountVendor(acct string) string
	ConfigDir(acct string) string
}

// LaunchPorts are the other groups' calls launch makes.
type LaunchPorts struct {
	Usage    UsagePort
	Accounts AccountsPort
	Vendors  VendorsPort
	// AccountReading is which account an agent's usage is landing on, and whether that is recorded, reported or assumed.
	AccountReading func(q db.Q, agent string) (account, source string, ok bool)
	// CurrentAccount is the account recorded for the agent.
	CurrentAccount func(q db.Q, agent string) string
	// WaitingFor is whether anything is waiting to be told to the agent.
	WaitingFor func(q db.Q, agent string) bool
	// TeamRules is the rules of each team the agent is in (team.Rules): its base.md text and its kept rules.
	TeamRules func(agent string) []string
	// Held is the agent's workspace's tools_deny and mode_max; nil: none.
	Held func(agent string) team.Held
	// Routing is the roster; nil when there is none.
	Routing func() *team.Team
	// McpApproved is the MCP servers of the launch's working directory that the founder approved, so a launch may enable them.
	McpApproved func(cwd string) []string
	// PaneAgent is the agent the daemon stamped the pane with ("" for a pane it did not open for an agent); nil: no pane is stamped.
	PaneAgent func(handle string) string
	// RegisterWorker joins the agent's terminal to the team's Run.
	RegisterWorker func(agent, handle string) error
	Warn           func(string)
	// Devices is the runners agents run on; nil is this machine alone.
	Devices DevicesPort
}

// DevicesPort is the runners as launch sees them (go/internal/nodes): an agent on one starts on the board's accounts logged in there.
type DevicesPort interface {
	// DeviceOf is the runner agent's session runs on, or opens on next, "" for this machine.
	DeviceOf(agent string) string
	// Accounts is, by board account, the config dir of each logged in on node, in this machine's home's terms.
	Accounts(node string) map[string]string
}

// Refusal is a launch that must not go ahead, in words for the person at the terminal.
type Refusal struct{ Msg string }

func (r *Refusal) Error() string { return r.Msg }

func refuse(format string, a ...any) *Refusal { return &Refusal{fmt.Sprintf(format, a...)} }

// Launcher is hl launch, the execution layer. The launcher wrapper sets the account's config directory in the
// environment before the vendor's binary starts, which the host preserves across hibernation and restore.
type Launcher struct {
	Q      db.Q
	Home   string
	Roster *team.Roster
	Ports  LaunchPorts
	Now    func() time.Time
	// Tokens is the session proof and key store; Build refuses without it.
	Tokens *sessiontoken.Tokens
	// FounderTyped says the founder's words are typed at agent (supervisor.Env.FounderTyped); nil is never.
	FounderTyped func(agent string) bool

	// placeMu makes choosing an account and holding its slot (wake_reserved) one step: a launch's choice (Build), a wake's fresh or
	// cold choice, and the slot of a move or a warm resume, asked again under it (wake.go). Taken after the agent's lock and the terminal lock, never before them; nothing under it makes a
	// network call or waits (usage.Env.Refresh, the one live read a pick may make, is not set in the daemon), and the only locks it takes
	// are the runners' (DevicesPort: placement's, then the registry's), which take none of the lifecycle's.
	placeMu sync.Mutex
}

// machine is the machine an account is chosen on: a runner (node) with the config dir of each board account logged in there, or this
// machine (node "").
type machine struct {
	node string
	dirs map[string]string
}

// machineOf is the machine agent's session runs on or opens on next.
func (l *Launcher) machineOf(agent string) machine {
	if l.Ports.Devices == nil {
		return machine{}
	}
	return l.machineAt(l.Ports.Devices.DeviceOf(agent))
}

// machineAt is node, "" for this machine.
func (l *Launcher) machineAt(node string) machine {
	if node == "" || l.Ports.Devices == nil {
		return machine{node: node}
	}
	return machine{node, l.Ports.Devices.Accounts(node)}
}

// Placing takes placeMu; the returned func lets it go.
func (l *Launcher) Placing() func() {
	l.placeMu.Lock()
	return l.placeMu.Unlock
}

func (l *Launcher) warn(format string, a ...any) {
	if l.Ports.Warn != nil {
		l.Ports.Warn(fmt.Sprintf(format, a...))
	}
}

func (l *Launcher) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

// mayStartAccounts is the accounts agent may start on: logged in on its machine (`assignable` says nothing about credentials), and
// named by its workspace's accounts rule when one is on.
func (l *Launcher) mayStartAccounts(agent string) []string {
	return l.mayStartOn(l.machineOf(agent), agent)
}

// StartsOn is whether the launch could choose an account for agent on node ("" is this machine), room aside: its pin or held account
// when it has one, else any it may start on there (its vendor's, allowed, logged in there, not paused). Placement and a wake read it.
func (l *Launcher) StartsOn(agent, node string) bool {
	if node != "" && l.Ports.Devices == nil {
		return false
	}
	m := l.machineAt(node)
	if pin := l.heldTo(m, agent); pin != "" {
		return !l.unusable(m, agent, pin)
	}
	for _, a := range l.mayStartOn(m, agent) {
		if !l.unusable(m, agent, a) {
			return true
		}
	}
	return false
}

func (l *Launcher) mayStartOn(m machine, agent string) []string {
	allowed, _ := l.accountsRule(agent)
	var out []string
	for _, a := range paths.ListAccounts(l.Home) {
		loggedIn := m.dirs[a] != ""
		if m.node == "" {
			loggedIn = l.Ports.Accounts.MayStart(a)
		}
		if loggedIn && (allowed == nil || slices.Contains(allowed, a)) {
			out = append(out, a)
		}
	}
	return out
}

// accountsRule is the accounts agent's workspace lets it run on and the rule that says so, nil when no accounts rule is on.
func (l *Launcher) accountsRule(agent string) ([]string, *team.KeptRule) {
	r, _ := l.rule(l.Q, agent, team.RuleAccounts)
	if r == nil {
		return nil, nil
	}
	return r.Value.([]string), r
}

// leftOut is how agent's workspace's accounts rule leaves acct out, "" when it does not or acct is "".
func (l *Launcher) leftOut(agent, acct string) string {
	if acct == "" {
		return ""
	}
	allowed, rule := l.accountsRule(agent)
	if allowed == nil || slices.Contains(allowed, acct) {
		return ""
	}
	return "is left out by its workspace's rule " + rule.Words()
}

// rule is the kept rule key in force for agent's workspace and that workspace's name, nil when none is on. One that cannot be read is
// none: a rule never stalls work.
func (l *Launcher) rule(q db.Q, agent, key string) (*team.KeptRule, string) {
	rules, ws, err := team.AgentRules(q, agent)
	if err != nil {
		l.warn("kept rules: not read for %s: %v", agent, err)
		return nil, ""
	}
	for _, r := range rules {
		if r.Key == key && team.RuleOn(r.Value) {
			return &r, ws
		}
	}
	return nil, ws
}

// PinOf is the account its row pins the agent to, "" for none. A pinned agent runs only there: nothing that picks or moves
// accounts puts it elsewhere, and while the account cannot take it, it waits with that account's reason.
func (l *Launcher) PinOf(agent string) string {
	cfg, err := l.Roster.Read(agent)
	if err != nil {
		return ""
	}
	pin, _ := cfg["account"].(string)
	return strings.TrimSpace(pin)
}

// ManualSplit is whether account_split is manual for agent: its own value in its row, else the config's.
func (l *Launcher) ManualSplit(agent string) bool {
	if cfg, err := l.Roster.Read(agent); err == nil {
		if v, ok := cfg[usage.SplitKey].(string); ok && (v == usage.SplitManual || v == usage.SplitAuto) {
			return v == usage.SplitManual
		}
	}
	return l.Ports.Usage.ManualSplit()
}

// heldTo is the one account the agent may start on, "" for any: its pin, else with account_split manual the account it is on. An agent
// on no account yet, or on one it cannot use at all (another vendor's, left out, logged out or paused), is placed as in auto.
func (l *Launcher) heldTo(m machine, agent string) string {
	if pin := l.PinOf(agent); pin != "" || !l.ManualSplit(agent) {
		return pin
	}
	own := l.Ports.CurrentAccount(l.Q, agent)
	if own == "" || l.unusable(m, agent, own) {
		return ""
	}
	return own
}

// unusable is whether agent cannot run on acct whatever its room: another vendor's, left out by its workspace, logged out or paused.
func (l *Launcher) unusable(m machine, agent, acct string) bool {
	return l.Ports.Vendors.AccountVendor(acct) != l.Ports.Vendors.AgentVendor(agent) || l.leftOut(agent, acct) != "" ||
		!slices.Contains(l.mayStartOn(m, agent), acct) || l.Ports.Usage.Read(acct).Paused
}

// pinnedWhy is why the account the agent is pinned to cannot take it now, "" when it can. The account must be the agent's vendor's and
// logged in. One whose vendor reports no usage takes it with no room test; one that reports usage is held to its room as any account is.
func (l *Launcher) pinnedWhy(m machine, agent, pin string) string {
	vendor, theirs := l.Ports.Vendors.AgentVendor(agent), l.Ports.Vendors.AccountVendor(pin)
	switch out := l.leftOut(agent, pin); {
	case theirs != vendor:
		return fmt.Sprintf("it is pinned to %s, a %s account, but it runs %s", pin, theirs, vendor)
	case out != "":
		return fmt.Sprintf("it is pinned to %s, which %s", pin, out)
	case !slices.Contains(l.mayStartOn(m, agent), pin):
		return fmt.Sprintf("it is pinned to %s, which is not logged in or may not start", pin)
	case l.Ports.Usage.Read(pin).Paused:
		return fmt.Sprintf("it is pinned to %s, which is paused", pin)
	case l.Ports.Usage.Read(pin).Unmetered:
		return ""
	}
	if adm := l.Ports.Usage.Admit(pin, agent, vendor); !adm.Fits {
		return fmt.Sprintf("it is pinned to %s, which cannot take it: %s", pin, strings.Join(adm.Reasons, ", "))
	}
	return ""
}

// FindRoom is the account that can take this agent's next session, "" for none: the account it is pinned to, else the best of the
// logged-in ones, and failing that one nothing says is a bad place to start. An account whose vendor reports no usage is never picked here.
func (l *Launcher) FindRoom(agent string) string { return l.findRoom(l.machineOf(agent), agent) }

func (l *Launcher) findRoom(m machine, agent string) string {
	if pin := l.heldTo(m, agent); pin != "" {
		if l.pinnedWhy(m, agent, pin) != "" {
			return ""
		}
		return pin
	}
	candidates := l.mayStartOn(m, agent)
	if picked := l.Ports.Usage.PickAccount(candidates, agent); picked != "" {
		return picked
	}
	vendor := l.Ports.Vendors.AgentVendor(agent)
	for _, a := range candidates {
		if l.Ports.Usage.Admit(a, agent, vendor).Fits && l.paused(a, false) == nil {
			return a
		}
	}
	return ""
}

// ChooseAccount is which account this launch runs on, "" when there is none. The moment a session is made is the only moment its
// account can be chosen, since after execve the config directory is fixed. A resume with no destination goes back to the account it
// was recorded on. account is a destination someone already chose (a move), a resume too: Build carries the session there
// (carrySession) and still holds the account to the vendor, login and wall checks.
func (l *Launcher) ChooseAccount(agent string, resuming bool, account string) string {
	return l.chooseOn(l.machineOf(agent), agent, resuming, account)
}

func (l *Launcher) chooseOn(m machine, agent string, resuming bool, account string) string {
	pin := l.PinOf(agent)
	outside := func(a string) bool { return l.leftOut(agent, a) != "" }
	switch {
	case pin != "" && account != "" && account != pin:
		return "" // a destination other than the pin: a pinned agent goes nowhere else
	case pin != "" && outside(pin):
		return "" // a pin its workspace's accounts rule leaves out takes it nowhere
	case account != "" && outside(account):
		return "" // a destination its workspace's accounts rule leaves out
	case account != "":
		return account
	case resuming && pin != "":
		return pin // a session recorded elsewhere, from before the pin, is not followed
	case resuming && !outside(l.Ports.CurrentAccount(l.Q, agent)):
		return l.Ports.CurrentAccount(l.Q, agent)
	}
	return l.findRoom(m, agent) // a session on an account the rule leaves out is carried to one it names
}

// WakeWait is a closed agent's wake walled behind an account's 5h or weekly wall, its slots, or its workspace's agent_cap.
type WakeWait struct {
	Account string
	Pct     int
	Reset   int64 // when the walled window rolls over; 0 unknown
	// Assumed is true when Account is a guess (one written before a launch confirmed it) rather than the account this resume is tied to.
	Assumed bool
	Window  string // the walled window's length as a person says it
	IsLong  bool   // the longest window, not the shortest
	// Slot: no wall, but nothing may start - the account is paused (Paused) or the workspace is at its agent_cap (Workspace).
	Slot bool
	// Paused: the account is paused, so nothing starts on it until it is resumed.
	Paused bool
	// Workspace: the agent's workspace has Busy of its agent_cap (Full) mid-turn, Rule saying so; no account is in question.
	Full, Busy int
	Workspace  string
	Rule       *team.KeptRule
}

// wall is the window this account is walled behind, the shortest first: an account walled on both reports the shortest one's rollover,
// and once that passes it is refused again on the longest still standing.
func (l *Launcher) wall(acct string, assumed bool) *WakeWait {
	st := l.Ports.Usage.Read(acct)
	if st.ShortTrusted != nil && *st.ShortTrusted >= l.Ports.Usage.ShortBlock() {
		return &WakeWait{Account: acct, Pct: *st.ShortTrusted, Reset: st.ShortReset, Assumed: assumed, Window: st.ShortLabel}
	}
	if st.LongTrusted != nil && *st.LongTrusted >= l.Ports.Usage.LongBlock() {
		return &WakeWait{Account: acct, Pct: *st.LongTrusted, Reset: st.LongReset, Assumed: assumed, Window: st.LongLabel, IsLong: true}
	}
	return nil
}

// paused is the wait of a start on acct while it is paused, nil when it is not.
func (l *Launcher) paused(acct string, assumed bool) *WakeWait {
	if l.Ports.Usage.Read(acct).Paused {
		return &WakeWait{Account: acct, Assumed: assumed, Window: "slot", Slot: true, Paused: true}
	}
	return nil
}

// SlowestPaced is the account a closed agent moves to when its own (skip) cannot take it: of the other accounts that may start, take this
// vendor, fit and are not paused, the one whose week runs out soonest. "" when none can.
func (l *Launcher) SlowestPaced(q db.Q, agent string, skip string) string {
	if l.ManualSplit(agent) && !l.Ports.Usage.Read(skip).Paused {
		return ""
	}
	best, _ := l.RoomElsewhere(q, agent, map[string]string{skip: ""}, false)
	return best
}

// RoomElsewhere is SlowestPaced with a reason for each account passed over: exclude names the accounts left out and why. The second result
// is why no account has room, "" when one does. A move to an account is held to this same test, so a wake and a handoff cannot disagree.
func (l *Launcher) RoomElsewhere(q db.Q, agent string, exclude map[string]string, fitOnly bool) (string, string) {
	return l.roomElsewhere(q, agent, exclude, fitOnly)
}

// ClosestRoom is the account with the most headroom of those a person may choose: what a person is offered when no account has room. ""
// when a wall, a paused account or a pin rules them all out.
func (l *Launcher) ClosestRoom(q db.Q, agent string, row *db.AgentState, exclude map[string]string) string {
	if l.PinOf(agent) != "" {
		return ""
	}
	best, most := "", math.Inf(-1)
	for _, a := range l.mayStartAccounts(agent) {
		if _, skip := exclude[a]; skip || !l.pastFit(agent, a) {
			continue
		}
		h, ok := l.Ports.Usage.Headroom(a)
		if !ok {
			h = 0
		}
		if h > most {
			best, most = a, h
		}
	}
	return best
}

// pastFit is whether a person may put agent on acct: no wall, nothing outright against it, and not paused.
func (l *Launcher) pastFit(agent, acct string) bool {
	return l.Ports.Usage.Admit(acct, agent, l.Ports.Vendors.AgentVendor(agent)).Fits && l.paused(acct, false) == nil
}

// RoomPastFit is why agent cannot go on acct by a person's choice past the room test, "" when it can.
func (l *Launcher) RoomPastFit(q db.Q, agent string, row *db.AgentState, acct string) string {
	if pin := l.PinOf(agent); pin != "" && acct != pin {
		return "is not " + pin + ", the account it is pinned to"
	}
	if why := l.leftOut(agent, acct); why != "" {
		return why
	}
	if !slices.Contains(l.mayStartAccounts(agent), acct) {
		return "is not logged in or may not start"
	}
	if !l.pastFit(agent, acct) {
		return "is at a limit or paused"
	}
	return ""
}

// WorthRepick is whether a cold wake on own is better put on to: the week that runs out faster, by usage.RepickGain points per hour; with
// either week unreadable the pick stands, as it did before the weekly ranking.
func (l *Launcher) WorthRepick(own, to string) bool {
	a, okA := l.Ports.Usage.Urgency(own)
	b, okB := l.Ports.Usage.Urgency(to)
	return !okA || !okB || b-a >= usage.RepickGain
}

// RoomNow is a person's start off skip: RoomElsewhere.
func (l *Launcher) RoomNow(q db.Q, agent string, skip string) (string, string) {
	return l.roomElsewhere(q, agent, map[string]string{skip: ""}, false)
}

func (l *Launcher) roomElsewhere(q db.Q, agent string, exclude map[string]string, fitOnly bool) (string, string) {
	if pin := l.PinOf(agent); pin != "" {
		return "", "it is pinned to " + pin
	}
	vendor := l.Ports.Vendors.AgentVendor(agent)
	best, most := "", math.Inf(-1)
	roomiest, roomMost := "", -1
	var passed []string
	var ranked []usage.Ranked
	for _, a := range l.mayStartAccounts(agent) {
		if _, skip := exclude[a]; skip {
			continue
		}
		if adm := l.Ports.Usage.Admit(a, agent, vendor); !adm.Fits {
			passed = append(passed, a+": "+strings.Join(adm.Reasons, ", "))
			continue
		}
		if l.paused(a, false) != nil {
			passed = append(passed, a+": paused")
			continue
		}
		// As the pick: an account that fits first; where none does, the one with the most room under its wall (not for fitOnly).
		if unfit, room := l.Ports.Usage.StartFit(a, agent); unfit != "" {
			passed = append(passed, a+": "+unfit)
			if !fitOnly && room > roomMost {
				roomiest, roomMost = a, room
			}
			continue
		}
		h, ok := l.Ports.Usage.Headroom(a)
		if !ok {
			h = 0 // a pace nobody can read is not evidence of room beyond the burst
		}
		if h > most {
			best, most = a, h
		}
		active, err := l.Ports.Usage.ActiveOn(q, a, agent)
		if err != nil {
			return "", "the agents mid-turn cannot be read: " + err.Error()
		}
		u, known := l.Ports.Usage.Urgency(a)
		ranked = append(ranked, usage.Ranked{Name: a, Urgency: u, Known: known, Active: active})
	}
	// The week that runs out soonest first, similar ones spread by agents mid-turn; the most headroom only where no week can be read.
	if b, ok := usage.BestByUrgency(ranked); ok {
		best = b.Name
	}
	if best == "" {
		best = roomiest
	}
	if best == "" {
		return "", "no account can take one more agent (" + strings.Join(passed, "; ") + ")"
	}
	return best, ""
}

// RoomOn is the same test for one account a person chose: why it cannot take the agent, "" when it can.
func (l *Launcher) RoomOn(q db.Q, agent string, row *db.AgentState, acct string) string {
	if pin := l.PinOf(agent); pin != "" && acct != pin {
		return "is not " + pin + ", the account it is pinned to"
	}
	if why := l.leftOut(agent, acct); why != "" {
		return why
	}
	for _, a := range l.mayStartAccounts(agent) {
		if a != acct {
			continue
		}
		if adm := l.Ports.Usage.Admit(a, agent, l.Ports.Vendors.AgentVendor(agent)); !adm.Fits {
			return "is not a place for it: " + strings.Join(adm.Reasons, ", ")
		}
		if l.paused(a, false) != nil {
			return "is paused"
		}
		return ""
	}
	return "is not logged in or may not start"
}

// FreesFirst is, for an agent no account has room for, the walled account whose wall lifts first, else a paused one (when it is resumed
// cannot be told); nil when none is walled or paused.
func (l *Launcher) FreesFirst(agent string) *WakeWait { return l.freesFirst(l.machineOf(agent), agent) }

func (l *Launcher) freesFirst(m machine, agent string) *WakeWait {
	vendor := l.Ports.Vendors.AgentVendor(agent)
	var best *WakeWait
	rank := func(w *WakeWait) float64 {
		if w.Reset == 0 {
			return math.Inf(1)
		}
		return float64(w.Reset)
	}
	for _, a := range l.mayStartOn(m, agent) {
		if l.Ports.Usage.NotFor(a, vendor) {
			continue
		}
		w := l.wall(a, false)
		if w == nil {
			w = l.paused(a, false)
		}
		if w != nil && (best == nil || rank(w) < rank(best)) {
			best = w
		}
	}
	return best
}

// NoRoom is why no account can take this agent, naming the one that frees up first; for a pinned agent, why its account cannot.
func (l *Launcher) NoRoom(agent string) string { return l.noRoom(l.machineOf(agent), agent) }

func (l *Launcher) noRoom(m machine, agent string) string {
	if pin := l.PinOf(agent); pin != "" {
		if why := l.pinnedWhy(m, agent, pin); why != "" {
			return why
		}
	}
	named := paths.ListAccounts(l.Home)
	if len(l.mayStartOn(m, agent)) == 0 {
		if len(named) == 0 {
			return "no account is registered - add one in the app's Accounts page first"
		}
		if allowed, rule := l.accountsRule(agent); allowed != nil {
			return "no account its workspace's rule " + rule.Words() + " names is logged in" + onRunner(m.node)
		}
		if m.node != "" {
			return "no account is logged in" + onRunner(m.node) + ". A human logs in there once (Handline never touches credentials)"
		}
		cmd, _ := l.Ports.Accounts.LoginCommand(named[0])
		return fmt.Sprintf("no account is logged in. A human has to do this once (Handline never touches credentials):\n\n  %s\n", cmd)
	}
	why := "no account can take one more agent"
	var each []string
	for _, r := range l.Ports.Usage.PickReasons(l.mayStartOn(m, agent), agent) {
		each = append(each, r.Account+": "+r.Why)
	}
	if len(each) > 0 {
		why += " (" + strings.Join(each, "; ") + ")"
	}
	if w := l.freesFirst(m, agent); w != nil {
		return why + " - " + DescribeWakeWait(w)
	}
	return why
}

// onRunner is " on node <node>" for a runner, "" for this machine.
func onRunner(node string) string {
	if node == "" {
		return ""
	}
	return " on node " + node
}

// WakeWait is whether a closed agent's wake is held by its workspace's agent_cap or walled behind its own account's 5h or weekly wall:
// the check made before anything is typed into its terminal. A resume cannot follow an account change, but the pinning rescue can start
// fresh elsewhere, so a wait is not final. It answers only "is it full", not "is anyone trying to wake this agent". A start with no
// session to resume goes wherever ChooseAccount finds room; with none it waits on the account that frees up first. The resume branch is
// pinned to the account its session is tied to. A computed condition: called again after a window rolls over, it returns nil.
func (l *Launcher) WakeWait(q db.Q, agent string, row *db.AgentState) *WakeWait {
	if !isClosed(row) {
		return nil
	}
	if w := l.WorkspaceWait(q, agent); w != nil {
		return w
	}
	if row.ResumeSession.String != "" && l.leftOut(agent, l.Ports.CurrentAccount(q, agent)) == "" {
		acct, source, ok := l.Ports.AccountReading(q, agent)
		if !ok {
			return nil
		}
		if w := l.wall(acct, source == "assumed"); w != nil {
			return w
		}
		return l.paused(acct, source == "assumed")
	}
	if l.ChooseAccount(agent, false, "") != "" {
		return nil
	}
	return l.FreesFirst(agent)
}

// WorkspaceWait is the wait of agent's workspace at its agent_cap: as many of its other agents mid-turn as the rule allows, nil when
// fewer are. A count that cannot be read holds nothing.
func (l *Launcher) WorkspaceWait(q db.Q, agent string) *WakeWait {
	return l.workspaceWait(q, agent, agent)
}

// RoomBeside is why one more session like agent's, beside the one it runs (a temp of it, go/internal/temps), could not start on this
// machine now: its workspace at its agent_cap with agent counted, or no account with room for it with agent's own load counted. ""
// when it could. The machine's load is the caller's to ask (loadgate.Gate.Held).
func (l *Launcher) RoomBeside(q db.Q, agent string) string {
	if w := l.workspaceWait(q, agent, ""); w != nil {
		return DescribeWakeWait(w)
	}
	m := l.machineAt("")
	candidates := l.mayStartOn(m, agent)
	if pin := l.heldTo(m, agent); pin != "" {
		candidates = []string{pin}
	}
	if l.Ports.Usage.PickBeside(candidates, agent) == "" {
		return l.noRoom(m, agent)
	}
	return ""
}

// workspaceWait is WorkspaceWait counting every agent of the workspace mid-turn but left ("" counts all).
func (l *Launcher) workspaceWait(q db.Q, agent, left string) *WakeWait {
	r, ws := l.rule(q, agent, team.RuleAgentCap)
	if r == nil {
		return nil
	}
	members, err := team.WorkspaceAgents(q, agent)
	if err != nil {
		l.warn("agent_cap: members of %s not read: %v", ws, err)
		return nil
	}
	busy := 0
	for _, a := range l.Ports.Usage.MidTurn(q, left) {
		if slices.Contains(members, a) {
			busy++
		}
	}
	if capN := int(r.Value.(int64)); busy >= capN {
		return &WakeWait{Slot: true, Workspace: ws, Full: capN, Busy: busy, Rule: r}
	}
	return nil
}

// WakeWaitReason is the sentence ps, triggers and the daemon's own log line print for a walled wake: one wording, so a screen can
// never say something the daemon does not act on. "" when it is not walled.
func (l *Launcher) WakeWaitReason(q db.Q, agent string, row *db.AgentState) string {
	if w := l.WakeWait(q, agent, row); w != nil {
		return DescribeWakeWait(w)
	}
	return ""
}

// WaitClock is when the walled window rolls over: a clock for the shortest one, a date and clock for the longest, which is days away.
func WaitClock(w *WakeWait, unknown string) string {
	if w.Reset == 0 {
		return unknown
	}
	layout := "15:04"
	if w.IsLong {
		layout = "01-02 15:04"
	}
	return time.Unix(w.Reset, 0).Local().Format(layout)
}

// DescribeWakeWait is the wording for a wait already computed, so a caller that holds one does not recompute it.
func DescribeWakeWait(w *WakeWait) string {
	if w.Workspace != "" {
		return fmt.Sprintf("waiting for a slot in workspace %s (%d of %d of its agents mid-turn; its rule %s)", w.Workspace, w.Busy, w.Full, w.Rule.Words())
	}
	if w.Paused {
		return fmt.Sprintf("waiting for %s: it is paused; resume it in the app's settings", w.Account)
	}
	who := w.Account
	if w.Assumed {
		who = "assumed account " + w.Account
	}
	return fmt.Sprintf("waiting for %s's %s window (%d%%, rolls over %s)", who, w.Window, w.Pct, WaitClock(w, "unknown"))
}

// mcpConfigArg is `--mcp-config <file>` for the shared mcp.json, if it exists and is a valid {"mcpServers": {...}} object. Not strict:
// passing this leaves an account's own servers loading too. A missing file changes nothing. An invalid one must not stop the launch (a
// typo in a shared file would otherwise wall off every agent), so the caller only warns.
func (l *Launcher) mcpConfigArg() (argv []string, problem string) {
	p := paths.MCPConfig(l.Home)
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil, ""
	}
	var data map[string]any
	if err == nil {
		err = json.Unmarshal(b, &data)
	}
	if err != nil {
		return nil, fmt.Sprintf("%s is invalid (%v)", p, err)
	}
	// Well-formed JSON that is not claude's shape still makes `claude` exit before the session starts: the exact wall-off this skip
	// exists to avoid. Server entries themselves need no checking.
	if _, ok := data["mcpServers"].(map[string]any); !ok {
		return nil, fmt.Sprintf("%s is invalid (not a {\"mcpServers\": {...}} object)", p)
	}
	return []string{"--mcp-config", p}, ""
}

// mcpConfigMaterial is what the mcp config part hashes for the shared mcp.json: whether a launch would pass --mcp-config at all, and if so the
// file's content, so editing it in place reads as drift even though the path never changes. Missing and invalid hash the same.
func (l *Launcher) mcpConfigMaterial() string {
	if argv, _ := l.mcpConfigArg(); len(argv) == 0 {
		return "absent"
	}
	b, err := os.ReadFile(paths.MCPConfig(l.Home))
	if err != nil {
		return "absent"
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (l *Launcher) git(worktree string, args ...string) string {
	r := gitrun.Run(worktree, args...)
	if r.RC != 0 {
		return ""
	}
	return strings.TrimSpace(r.Stdout)
}

// MainRefFiles are the files git rewrites to move the repository's default branch (gitrun.DefaultBranch), which a session may not
// write though the sandbox lets it write .git for its own commits: the branch's ref, the packed refs, origin/HEAD, and their locks.
// origin/HEAD is among them because it says which branch the default is: a session that repointed it would leave the real branch
// unguarded here and unseen by the land guard, a done and a prune. A launch names it first (gitrun.NameOriginHead), and puts
// followRemoteHEAD=never in the session's environment (leaveOriginHead) so the session's fetch does not try to write it and complain.
func (l *Launcher) MainRefFiles(worktree string) []string {
	common := l.git(worktree, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if common == "" {
		return nil
	}
	ref := filepath.Join(common, "refs", "heads", filepath.FromSlash(gitrun.DefaultBranch(worktree)))
	packed := filepath.Join(common, "packed-refs")
	head := filepath.Join(common, "refs", "remotes", "origin", "HEAD")
	return []string{ref, ref + ".lock", packed, packed + ".lock", head, head + ".lock"}
}

// knowsAs is the agent a temp was hired from, whose lessons and memory folder it works with; agent itself for an agent of its own.
func (l *Launcher) knowsAs(agent string) (string, error) {
	from, err := paths.HiredFrom(l.Q, paths.AgentKey(l.Home, agent))
	if from == "" || err != nil {
		return agent, err
	}
	return from, nil
}

// MemoryDir is the folder agent's sessions keep their memory in: its own, or for a temp the agent's it was hired from, to read and write.
func (l *Launcher) MemoryDir(agent string) (string, error) {
	knows, err := l.knowsAs(agent)
	return filepath.Join(paths.AgentDir(l.Home, knows), "memory"), err
}

// HiddenMemories are the memory folders agent may not read: every other agent's.
func (l *Launcher) HiddenMemories(agent string) ([]string, error) {
	keys, err := paths.ReadAgents(l.Home)
	if err != nil {
		return nil, err
	}
	own := paths.AgentKey(l.Home, agent)
	knows, err := paths.KnowsAs(l.Q, own)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, k := range keys {
		if k != own && k != knows { // a temp's memory folder is the agent's it was hired from
			out = append(out, filepath.Join(paths.AgentsDir(l.Home), k, "memory"))
		}
	}
	sort.Strings(out)
	return out, nil
}

var githubRemote = regexp.MustCompile(`github\.com[:/]([\w.-]+/[\w.-]+?)(?:\.git)?\s`)

// TrustedEnvironment is what the permission classifier should count as the team's own, as the prose it reads: the state folder, this
// worktree and its repository, the GitHub repos its remotes name, and the other agents' folders the founder granted it (readGranted).
// Anything else stays outside the boundary.
func (l *Launcher) TrustedEnvironment(worktree string, readGranted []string) []string {
	lines := []string{fmt.Sprintf("The team's state folder %s and everything under it is the team's own: reading and writing it, and running `hl`, is routine.", l.Home)}
	if len(readGranted) > 0 {
		lines = append(lines, "The founder granted this agent read access to "+strings.Join(readGranted, ", ")+
			", other agents' memories and transcripts included: reading them, with the Read tool or a shell command, is sanctioned.")
	}
	if worktree == "" {
		return lines
	}
	lines = append(lines, fmt.Sprintf("This agent's own git worktree is %s: `cd`, editing files, `sed -i`, running tests and committing inside it are routine.", worktree))
	if common := l.git(worktree, "rev-parse", "--path-format=absolute", "--git-common-dir"); common != "" {
		lines = append(lines, fmt.Sprintf("Its repository is %s, and the sibling worktrees under %s are other agents of the same team.", filepath.Dir(common), filepath.Dir(worktree)))
	}
	seen := map[string]bool{}
	var repos []string
	for _, line := range strings.Split(l.git(worktree, "remote", "-v"), "\n") {
		if m := githubRemote.FindStringSubmatch(line + " "); m != nil && !seen[m[1]] {
			seen[m[1]] = true
			repos = append(repos, m[1])
		}
	}
	sort.Strings(repos)
	if len(repos) > 0 {
		var sites []string
		for _, r := range repos {
			sites = append(sites, "github.com/"+r)
		}
		lines = append(lines, "Source control: the GitHub repos "+strings.Join(sites, ", ")+" and their branches.")
	}
	return lines
}

// Transcripts is where a vendor keeps the transcripts of the sessions run in a worktree.
type Transcripts interface {
	Name() string
	TranscriptDir(configDir, worktree string) (string, bool)
}

// Confined is what a session may not read or write beyond the sandbox's own limits.
type Confined struct {
	// Hide is what it may not read: other agents' transcripts and memories, and the key folders. AskRead is the part of Hide, the
	// transcripts and memories, whose Read tool asks the founder instead of refusing.
	Hide, AskRead []string
	// NoWrite is the files that move the main branch.
	NoWrite []string
	// Lifted is the founder's grants that took a transcript or memory folder out of Hide.
	Lifted []string
}

// Confinement is what a session of agent, running in worktree, may not read or write. A transcript or memory folder one of granted
// holds is not hidden; the key folders are hidden whatever is granted.
func (l *Launcher) Confinement(agent, worktree string, tr Transcripts, granted []string) (Confined, error) {
	var c Confined
	hide, err := l.HiddenTranscripts(agent, tr)
	if err != nil {
		return c, err
	}
	memories, err := l.HiddenMemories(agent)
	if err != nil {
		return c, err
	}
	hide = append(hide, memories...)
	for _, g := range granted {
		if len(access.Outside([]string{g}, hide)) < len(hide) {
			c.Lifted = append(c.Lifted, g)
		}
	}
	c.AskRead = access.Outside(c.Lifted, hide)
	c.Hide = append(append([]string{}, c.AskRead...), l.Tokens.HiddenFolders()...)
	c.NoWrite = l.MainRefFiles(worktree)
	return c, nil
}

// HiddenTranscripts are the transcript folders agent may not read: every other agent's, in every account of this vendor, since an agent
// moves between accounts. Only agents with a worktree on record can be named.
func (l *Launcher) HiddenTranscripts(agent string, drv Transcripts) ([]string, error) {
	rows, err := db.AllAgentStates(l.Q)
	if err != nil {
		return nil, err
	}
	var configs []string
	for _, a := range paths.ListAccounts(l.Home) {
		if l.Ports.Vendors.AccountVendor(a) == drv.Name() {
			configs = append(configs, l.Ports.Vendors.ConfigDir(a))
		}
	}
	mine := ""
	for _, r := range rows {
		if r.Agent == agent {
			mine = worktreepaths.RecordedPath(r.Worktree.String)
		}
	}
	var out []string
	for _, r := range rows {
		if r.Agent == agent || r.Worktree.String == "" {
			continue
		}
		if mine != "" && realpath(worktreepaths.RecordedPath(r.Worktree.String)) == realpath(mine) {
			continue // a desk it now holds that another agent held before: its own transcripts are kept there too
		}
		first := r.Worktree.String
		if i := strings.Index(first, "::"); i >= 0 {
			first = first[i+2:]
		}
		for _, cfg := range configs {
			if d, ok := drv.TranscriptDir(cfg, first); ok {
				out = append(out, d)
			}
		}
	}
	return out, nil
}

// youAre is the prompt's first line: the agent's name, and its name as $HANDLINE_AGENT holds it when that is qualified by its workspace.
func youAre(name, handle string) string {
	if handle == name {
		return fmt.Sprintf("You are `%s`, one agent in a Handline team. Your name is also in $HANDLINE_AGENT.", name)
	}
	return fmt.Sprintf("You are `%s`, one agent in a Handline team; outside your workspace you are `%s`, as $HANDLINE_AGENT holds it.", name, handle)
}

// staticPreamble is the part of ContextFor that a rebuild reproduces byte for byte, but for the team rules: everything except the
// handoff, which is supposed to differ every time this agent is parked. The rules follow it in ContextFor; FingerprintParts hashes the
// inputs a person changes.
func (l *Launcher) staticPreamble(agent string) (string, error) {
	parts := []string{
		youAre(addr.For(l.Q, agent, agent), paths.AgentHandle(l.Home, agent)),
		"Stay inside the folder you were started in. To read another branch's version of a file, use `git show <ref>:<path>` - it needs nothing outside the folder. Copying a file out to /tmp to read it stops you on a permission prompt only a human can answer, and you will sit there until one does.",
		"On a long request, keep `hl progress \"done: ... left: ...\"` current: your handoff carries it to the session that takes over.",
	}
	if line := l.yourTeam(agent); line != "" {
		parts = append(parts, line)
	}
	// The headings name the files they came from, so that moving them into the database changed no session's fingerprint.
	for _, f := range [][2]string{{db.PartRole, "# Your role (ROLE.md)"}, {db.PartBase, "# Your standing context (base.md)"}} {
		if text := l.instruction(agent, f[0]); text != "" {
			parts = append(parts, f[1]+"\n"+text)
		}
	}
	return strings.Join(parts, "\n\n"), nil
}

// ContextFor is ROLE.md + base.md + the team rules + its founder's preferences + the agent's lessons + the handoff waiting for it + the
// daemon's own record of where the agent was, as one system-prompt appendix.
func (l *Launcher) ContextFor(agent string) (string, error) {
	static, err := l.staticPreamble(agent)
	if err != nil {
		return "", err
	}
	parts := append([]string{static}, l.Ports.TeamRules(agent)...)
	// Lessons are not in the static preamble: an add mid-session must not read as a changed start; the next session carries it.
	owner, err := addr.OwnerOf(l.Q, agent)
	if err != nil && !errors.Is(err, actor.ErrNoOwner) {
		return "", err
	}
	knows, err := l.knowsAs(agent)
	if err != nil {
		return "", err
	}
	for _, whose := range []string{owner, knows} {
		if whose == "" { // no owner yet
			continue
		}
		lessons, err := db.LiveLessons(l.Q, whose)
		if err != nil {
			return "", err
		}
		parts = append(parts, db.PromptSection(whose, lessons))
	}
	text, err := db.GetHandoff(l.Q, agent)
	if err != nil {
		return "", err
	}
	if text != "" {
		parts = append(parts, "# Where you left off (your handoff)\nYou were interrupted and resumed. This is your own record.\n"+strings.TrimSpace(text))
	}
	where, err := l.where(agent, text)
	if err != nil {
		return "", err
	}
	parts = append(parts, where)
	if self, k := paths.AgentKey(l.Home, agent), paths.AgentKey(l.Home, knows); k != self {
		record, err := l.recordOf(self, k)
		if err != nil {
			return "", err
		}
		parts = append(parts, record)
	}
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n\n"), nil
}

// recordOf is what the agent a temp was hired from gets at a fresh start (its handoff and the daemon's record under it), for the temp to
// start knowing what its agent knows.
func (l *Launcher) recordOf(temp, origin string) (string, error) {
	text, err := db.GetHandoff(l.Q, origin)
	if err != nil {
		return "", err
	}
	where, err := l.where(origin, text)
	if err != nil {
		return "", err
	}
	head := fmt.Sprintf("# Your agent's record\nYou are a temp of %s. This is its own record as it gets it at a fresh start: \"you\" in it is %s.",
		addr.Ref(origin), addr.Ref(origin))
	parts := []string{addr.RenderFor(l.Q, temp, head)}
	if text = strings.TrimSpace(text); text != "" {
		parts = append(parts, text)
	}
	return strings.Join(append(parts, where), "\n\n"), nil
}

// where is the daemon's own record of where agent was (handoff.WhereText), for a session that starts with the handoff text.
func (l *Launcher) where(agent, text string) (string, error) {
	// the agent the handoff lists messages with: the opener of the oldest task it holds, else its workspace's contact
	lead, _, err := db.AgentNoticeTo(l.Q, agent)
	if err != nil {
		return "", err
	}
	if actor.IsPerson(lead) && l.Ports.Routing != nil {
		if t := l.Ports.Routing(); t != nil {
			if c := t.ContactOf(t.TeamOf(agent)); c != agent {
				lead = c
			}
		}
	}
	return handoff.WhereText(l.Q, agent, lead, text, l.now())
}

// yourTeam is the agent's workspace peers and its contact, read from the roster, and the hl commands that reach them. Every agent of
// a workspace is told the same: no one reports to anyone. Generated rather than written into each ROLE.md: a role that named its
// colleagues by hand kept naming them after they were retired and renamed. How a team works together is its rules file (team.Rules).
func (l *Launcher) yourTeam(agent string) string {
	f, ok := l.teamFacts(agent)
	if !ok {
		return ""
	}
	lines := []string{"# Your team"}
	if len(f.peers) > 0 {
		lines = append(lines, "Your peers in workspace `"+f.ws+"`: "+strings.Join(f.peers, ", ")+".")
	}
	switch f.contact {
	case "":
	case agent:
		lines = append(lines, "You are the workspace's contact: work from another workspace comes to you. Pass it to the peer it fits, and do your own work too.",
			team.Protocol("story"))
	default:
		lines = append(lines, "The workspace's contact is "+f.label(f.contact)+": work from another workspace comes to it, and it passes that work to the peer it fits.")
	}
	lines = append(lines, team.Protocol("ends"),
		"Give anyone work with `hl task new --assign <name> -t <time> \"<title>\" \"<what to do, and what done looks like>\"`; its done goes back to "+
			"whoever opened the task. Write about a task with `hl task comment T<n> add \"@<name> ...\"`; correct a task the same way, throw a "+
			"stale one away with `hl task discard T<n> \"<why>\"`, and record a wait with `--blocked-by T<n>`.")
	if f.contact != "" && f.contact != agent {
		lines = append(lines, "Being blocked, or a decision someone else must make, is a task for the owner of the task you hold, or for the "+
			"contact when you do not know who: `hl task new --assign <name> -t <time> \"<title>\" \"<body>\"`.")
	} else {
		lines = append(lines, "Being blocked, or a decision someone else must make, is a task for the owner of the task you hold: "+
			"`hl task new --assign <name> -t <time> \"<title>\" \"<body>\"`.")
	}
	talk := team.Protocol("terminal")
	if f.founderTyped {
		talk = "The founder may type straight into your terminal. When the person types to you directly, answer with `hl say` too; your terminal " +
			"text is not shown to them. A `... waiting for you:` delivery of a task comment is not typing: answer it on that task. " + talk
	}
	lines = append(lines, talk, team.Protocol("land"), team.Protocol("legacy"), team.Protocol("memory"), team.Protocol("progress"),
		"`hl team` shows who you may reach and what each is doing now. "+team.Protocol("founder"), team.Protocol("command"))
	return strings.Join(lines, "\n")
}

// teamFacts are what the team section says of agent's team, apart from Handline's own wording: its workspace, contact, peers by name
// and whether the founder types to it. ok is false with no team to read.
type teamFacts struct {
	ws, contact  string
	peers        []string
	founderTyped bool
	label        func(key string) string
}

func (l *Launcher) teamFacts(agent string) (f teamFacts, ok bool) {
	var t *team.Team
	if l.Ports.Routing != nil {
		t = l.Ports.Routing()
	}
	if t == nil {
		return f, false
	}
	f.ws = t.TeamOf(agent)
	f.contact = t.ContactOf(f.ws)
	f.label = func(key string) string { return "`" + addr.For(l.Q, agent, key) + "`" } // by name, as hl team shows it
	for _, n := range t.Members(f.ws) {
		if n != agent {
			f.peers = append(f.peers, f.label(n))
		}
	}
	f.founderTyped = l.FounderTyped != nil && l.FounderTyped(agent)
	return f, true
}

// teamMaterial is teamFacts as one string, "" with no team: what a launch's team section hashes, so a Handline update that only
// rewords it is no change. The peers are left out: an agent joining or leaving the team is no reason to restart every running member.
// A launch still lists the peers as they are then, and `hl team` shows who is there now.
func (l *Launcher) teamMaterial(agent string) string {
	f, ok := l.teamFacts(agent)
	if !ok {
		return ""
	}
	contact := ""
	if f.contact != "" {
		contact = f.label(f.contact)
	}
	return strings.Join([]string{f.ws, contact, strconv.FormatBool(f.founderTyped)}, "\x1f")
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func (l *Launcher) effortOf(cfg team.Config) string {
	if e, ok := cfg["effort"].(string); ok && e != "" {
		return e
	}
	return team.DefaultEffort
}

func modelOf(cfg team.Config) string {
	if m, ok := cfg["model"].(string); ok {
		return m
	}
	return ""
}

// held is the kept rules the agent's sessions start under.
func (l *Launcher) held(agent string) team.Held {
	if l.Ports.Held == nil {
		return team.Held{}
	}
	return l.Ports.Held(agent)
}

// disallowed is the global disallowed-tools list with the workspace's tools_deny added.
func (l *Launcher) disallowed(agent string) string {
	return strings.Join(append([]string{DisallowedTools}, l.held(agent).Deny...), ",")
}

// modeOf is the agent's mode, held down to its workspace's mode_max.
func (l *Launcher) modeOf(agent string, cfg team.Config) string {
	return l.held(agent).CapMode(l.Roster.ModeOf(agent, cfg))
}

func (l *Launcher) plainMode(agent string, cfg team.Config) bool {
	m := l.modeOf(agent, cfg)
	return m == team.DefaultMode
}

// SandboxOffPart is what FingerprintParts records under "sandbox" for a session launched with the sandbox off.
var SandboxOffPart = sha256Hex("off")[:8]

// LaunchedAt is beside the setting hashes in a session's recorded parts: when it was launched, which tells one session from the next
// even when both resume the same transcript.
const LaunchedAt = "launched_at"

// LaunchedBuild is beside the setting hashes too: the Handline build that launched it.
const LaunchedBuild = "build"

// LaunchedInstructions is beside the setting hashes too: the newest instruction version it launched with.
const LaunchedInstructions = "instructions"

// LaunchedValues is beside the setting hashes too: the settings whose value a person can read, for saying what a later launch changed.
const LaunchedValues = "values"

// LaunchedTexts is beside the setting hashes too: the text of ROLE.md, base.md and the team instructions, for saying which lines a later
// launch adds and removes. The team rules text is under LaunchedRules, which RulesNote also rewrites once a session is told.
const LaunchedTexts = "texts"

// changedBytesMax is how many bytes of changed lines a start keeps, in order over all its parts; More counts the lines past it.
const changedBytesMax = 32 << 10

// textBytesMax is how many bytes of whole part texts a start keeps over all its changed parts, in order; a part that does not fit has no Text.
const textBytesMax = 256 << 10

// partOrder is the order the parts are written in.
// Renaming a part restarts no one: a name the earlier record lacks is skipped.
var partOrder = []string{"model", "effort", "mcp config", "browser", "permissions", "sandbox", "ROLE.md", "base.md", "team instructions", rulesPart}

// FingerprintParts is what a launch reads from the settings a person edits, one short hash each, named for the person. Recorded at
// launch (LaunchRecord), and compared by the daemon to name what a running session has not picked up. Not here: what Build reads
// from the machine (the account, the handoff, hidden transcripts, the main-ref files) and the fixed text of the preamble.
func (l *Launcher) FingerprintParts(agent string) (map[string]string, error) {
	texts, rules := l.texts(agent)
	return l.partsOf(agent, texts, rules)
}

// PartsSince is FingerprintParts as the agent's running session is held to: a part whose text the launch recorded and the next launch
// gives unchanged keeps the recorded hash, since only how Handline hashes it moved (T9237 did, for team instructions), which a restart
// would not change for the session.
func (l *Launcher) PartsSince(agent string) (map[string]string, error) {
	row, err := db.GetAgentState(l.Q, agent)
	if err != nil {
		return nil, err
	}
	var old map[string]json.RawMessage
	if row != nil {
		_ = json.Unmarshal([]byte(row.LaunchParts.String), &old)
	}
	texts, rules := l.texts(agent)
	return l.partsSince(agent, old, texts, rules)
}

func (l *Launcher) partsSince(agent string, old map[string]json.RawMessage, texts map[string]string, rules []string) (map[string]string, error) {
	parts, err := l.partsOf(agent, texts, rules)
	if err != nil {
		return nil, err
	}
	for k, before := range launchedTexts(old) {
		var h string
		if now, ok := texts[k]; ok && now == before && json.Unmarshal(old[k], &h) == nil {
			parts[k] = h
		}
	}
	return parts, nil
}

// launchedTexts is the part texts a launch record keeps (LaunchedTexts, and the team rules under LaunchedRules), by part.
func launchedTexts(old map[string]json.RawMessage) map[string]string {
	var texts map[string]string
	if json.Unmarshal(old[LaunchedTexts], &texts) != nil || texts == nil { // "texts": null reads as a nil map
		texts = map[string]string{}
	}
	var rules string
	if json.Unmarshal(old[LaunchedRules], &rules) == nil {
		texts[rulesPart] = rules
	}
	return texts
}

// partsOf is FingerprintParts of texts and rules read once, so a record's texts are of the version its hashes are.
func (l *Launcher) partsOf(agent string, texts map[string]string, rules []string) (map[string]string, error) {
	cfg, err := l.Roster.Read(agent)
	if err != nil {
		return nil, err
	}
	h := func(s string) string { return sha256Hex(s)[:8] }
	mcp := team.MCPConfigArgs(cfg)
	mcpTail := ""
	if len(mcp) > 0 {
		mcpTail = mcp[1]
	}
	browser := ""
	if team.Chrome(cfg) {
		browser = "on"
	}
	perm := l.disallowed(agent)
	if !l.plainMode(agent, cfg) {
		perm += "\x1emode:" + l.modeOf(agent, cfg)
	}
	// A kept grant lifts a hide only at launch, so a new one is applied as a person's settings change is (tick.ApplySettings).
	kept, err := l.keptGrants(agent)
	if err != nil {
		return nil, err
	}
	if kept != "" {
		perm += "\x1egrants:" + kept
	}
	sandbox := h("")
	if !l.Roster.SandboxOn(agent, cfg) {
		sandbox = SandboxOffPart
	}
	return map[string]string{
		"model":             h(modelOf(cfg)),
		"effort":            h(l.effortOf(cfg)),
		"mcp config":        h(l.mcpConfigMaterial() + "\x1e" + mcpTail),
		"browser":           h(browser),
		"permissions":       h(perm),
		"sandbox":           sandbox,
		"ROLE.md":           h(texts["ROLE.md"]),
		"base.md":           h(texts["base.md"]),
		"team instructions": h(l.teamMaterial(agent)),
		rulesPart:           rulesHash(rules),
	}, nil
}

// keptGrants is the agent's always grants, in the order kept; "" for none.
func (l *Launcher) keptGrants(agent string) (string, error) {
	gs, err := access.Grants(l.Q, agent)
	if err != nil {
		return "", err
	}
	var kept []string
	for _, g := range gs {
		if g.Scope == "always" {
			kept = append(kept, g.Path)
		}
	}
	return strings.Join(kept, "\x1f"), nil
}

// texts are ROLE.md, base.md, the team instructions and the team rules as the next launch gives them, and the rules as read.
func (l *Launcher) texts(agent string) (map[string]string, []string) {
	rules := l.Ports.TeamRules(agent)
	return map[string]string{"ROLE.md": l.instruction(agent, db.PartRole), "base.md": l.instruction(agent, db.PartBase), "team instructions": l.yourTeam(agent),
		rulesPart: strings.Join(rules, "\n\n")}, rules
}

// instruction is the agent's part in force (db.PartRole, db.PartBase), "" for none or when it cannot be read.
func (l *Launcher) instruction(agent, part string) string {
	text, _ := team.AgentInstruction(l.Q, agent, part)
	return text
}

// PromptParts are the parts a launch gives in the appended system prompt. claude --resume keeps the system prompt its conversation began
// with, so a resume does not pick them up: they reach a session only at a fresh start.
var PromptParts = []string{"ROLE.md", "base.md", "team instructions"}

// LaunchRecord is the parts as JSON, in order, with the launch's own stamp, as `launch_parts` keeps them.
func (l *Launcher) LaunchRecord(agent string) (string, error) { return l.launchRecord(agent, "") }

// resumeRecord is LaunchRecord for a resume of the session recorded as was: the prompt parts and the team rules stay as was has them
// (hash, text and the instruction version seen), since the resumed session still has those: a prompt part stays owed until a fresh
// start, and changed rules are still told.
func (l *Launcher) resumeRecord(agent, was string) (string, error) { return l.launchRecord(agent, was) }

func (l *Launcher) launchRecord(agent, was string) (string, error) {
	seen, err := db.LastInstructionID(l.Q, 0) // before the texts: a version written while they are read is one this launch may not have
	if err != nil {
		return "", err
	}
	texts, rules := l.texts(agent)
	parts, err := l.partsOf(agent, texts, rules)
	if err != nil {
		return "", err
	}
	var old map[string]json.RawMessage
	if was != "" && json.Unmarshal([]byte(was), &old) == nil && old != nil {
		oldTexts := launchedTexts(old)
		kept := false
		for _, k := range append(slices.Clone(PromptParts), rulesPart) { // the rules too: a session is told of them, by message
			var h string
			if json.Unmarshal(old[k], &h) != nil {
				continue // a record without the part: the launch's own stands
			}
			parts[k], kept = h, true
			if t, ok := oldTexts[k]; ok {
				texts[k] = t
			} else {
				delete(texts, k) // no text kept: a text beside the old hash would read as unchanged
			}
		}
		var s int64
		if kept && json.Unmarshal(old[LaunchedInstructions], &s) == nil {
			seen = s
		}
	}
	var items []string
	for _, k := range partOrder {
		v, _ := json.Marshal(parts[k])
		kk, _ := json.Marshal(k)
		items = append(items, string(kk)+": "+string(v))
	}
	values, err := l.readableValues(agent)
	if err != nil {
		return "", err
	}
	v, _ := json.Marshal(values)
	items = append(items, `"`+LaunchedValues+`": `+string(v))
	r, _ := json.Marshal(texts[rulesPart])
	items = append(items, `"`+LaunchedRules+`": `+string(r))
	delete(texts, rulesPart)
	tx, _ := json.Marshal(texts)
	items = append(items, `"`+LaunchedTexts+`": `+string(tx))
	b, _ := json.Marshal(version.Sha)
	items = append(items, `"`+LaunchedBuild+`": `+string(b))
	items = append(items, `"`+LaunchedInstructions+`": `+strconv.FormatInt(seen, 10))
	stamp := float64(l.now().UnixNano()) / 1e9
	items = append(items, `"`+LaunchedAt+`": `+strconv.FormatFloat(stamp, 'f', -1, 64))
	return "{" + strings.Join(items, ", ") + "}", nil
}

// readableValues are the settings of the next launch whose value says something to a person.
func (l *Launcher) readableValues(agent string) (map[string]string, error) {
	cfg, err := l.Roster.Read(agent)
	if err != nil {
		return nil, err
	}
	return map[string]string{"model": modelOf(cfg), "effort": l.effortOf(cfg)}, nil
}

// LineChange is the lines a launch adds to one text part, and the ones it takes out, since the session recorded before it: at most
// changedBytesMax over all of a start's parts, More the rest. Text is the part's whole text as this launch starts with it.
type LineChange struct {
	Part    string   `json:"part"`
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	More    int      `json:"more,omitempty"`
	Text    string   `json:"text,omitempty"`
}

// whatChanged says what this launch's settings change from the session recorded before it, in launch order and in the words a person
// reads: "model changed (opus-5-5 -> sonnet-5-5), team rules changed (workspaces/w1/base.md)". Nothing for no earlier record.
// A record from before the values were kept names the setting alone. lines are the changed text parts line by line, for each whose
// earlier text the record kept.
func (l *Launcher) whatChanged(agent string) (string, []LineChange, error) {
	row, err := db.GetAgentState(l.Q, agent)
	if err != nil || row == nil {
		return "", nil, err
	}
	var old map[string]json.RawMessage
	if json.Unmarshal([]byte(row.LaunchParts.String), &old) != nil {
		return "", nil, nil
	}
	var was map[string]string
	_ = json.Unmarshal(old[LaunchedValues], &was)
	wasText := launchedTexts(old)
	nowText, rules := l.texts(agent)
	parts, err := l.partsSince(agent, old, nowText, rules)
	if err != nil {
		return "", nil, err
	}
	now, err := l.readableValues(agent)
	if err != nil {
		return "", nil, err
	}
	var changed []string
	var lines []LineChange
	textBudget := textBytesMax
	budget := changedBytesMax // once a line does not fit, it and every line after it are counted, so what is kept has no gaps
	keep := func(side []string) (kept []string, more int) {
		kept = []string{}
		for i, ln := range side {
			if budget < len(ln) {
				budget = -1
				return kept, len(side) - i
			}
			budget -= len(ln)
			kept = append(kept, ln)
		}
		return kept, 0
	}
	for _, k := range partOrder {
		hash, ok := old[k]
		if !ok {
			continue
		}
		var h string
		if json.Unmarshal(hash, &h) == nil && h == parts[k] {
			continue
		}
		words := k + " changed"
		if by := l.changedBy(agent, k, old); by != "" {
			words += " " + by
		}
		if before, ok := was[k]; ok {
			words += " (" + readable(before) + " -> " + readable(now[k]) + ")"
		} else if where := l.sourceOf(agent, k); where != "" {
			words += " (" + where + ")"
		}
		changed = append(changed, words)
		if before, ok := wasText[k]; ok {
			if added, removed := lineChanges(before, nowText[k]); len(added)+len(removed) > 0 {
				c := LineChange{Part: k}
				var ma, mr int
				c.Added, ma = keep(added)
				c.Removed, mr = keep(removed)
				c.More = ma + mr
				if len(nowText[k]) <= textBudget {
					c.Text = nowText[k]
					textBudget -= len(c.Text)
				}
				lines = append(lines, c)
			}
		}
	}
	return strings.Join(changed, ", "), lines, nil
}

// changedBy says who changed part since the launch old records, as "by <who>": the writers of its versions since then, a Handline update
// for team instructions a newer build wrote, or who set a setting; "on disk" for model, effort or browser no one set through Handline.
func (l *Launcher) changedBy(agent, part string, old map[string]json.RawMessage) string {
	switch part {
	case "ROLE.md", "base.md", rulesPart:
		return l.versionsBy(agent, part, old)
	case "model", "effort", "browser": // kept in agent.yaml alone
		if by, _, _ := db.Meta(l.Q, db.SettingsByMeta(agent)); by != "" {
			return "by " + l.personWords(by)
		}
		return "on disk"
	}
	return ""
}

// versionsBy is changedBy of an instruction part: who wrote each version after the launch, with the founder's OK where given. Handline's
// own seeding of the house rules is no one's change.
func (l *Launcher) versionsBy(agent, part string, old map[string]json.RawMessage) string {
	var owners []string
	if part == rulesPart {
		if l.Ports.Routing != nil {
			for _, ws := range team.RuleTeams(l.Q, agent, l.Ports.Routing()) {
				if k, err := paths.WorkspaceKeyIn(l.Q, ws); err == nil && k != "" {
					owners = append(owners, k)
				}
			}
		}
	} else if k, err := paths.AgentKeyIn(l.Q, agent); err == nil && k != "" {
		owners = append(owners, k)
	}
	seen, err := launchedInstructions(l.Q, old)
	if err != nil {
		return ""
	}
	instr := team.InstructionParts[part]
	if part == rulesPart {
		instr = db.PartRules
	}
	var who []string
	for _, o := range owners {
		vs, err := db.InstructionsAfter(l.Q, o, instr, seen)
		if err != nil {
			return ""
		}
		for _, v := range vs {
			if v.Actor == team.SeedActor {
				continue
			}
			w := l.personWords(v.Actor)
			if v.Approver != "" && v.Approver != v.Actor {
				w += " with " + l.personWords(v.Approver) + "'s OK"
			}
			if !slices.Contains(who, w) {
				who = append(who, w)
			}
		}
	}
	if len(who) == 0 {
		return ""
	}
	return "by " + strings.Join(who, " and ")
}

// launchedInstructions is the newest instruction version the launch old records had seen: as it recorded it, or for a record from before
// that was kept, the newest written before it launched.
func launchedInstructions(q db.Q, old map[string]json.RawMessage) (int64, error) {
	var seen int64
	if json.Unmarshal(old[LaunchedInstructions], &seen) == nil {
		return seen, nil
	}
	var since float64
	_ = json.Unmarshal(old[LaunchedAt], &since)
	return db.LastInstructionID(q, int64(since))
}

// personWords is who as the chat names it: an agent by its handle (paths.AgentHandle), the founder as the founder, anything else as
// it is.
func (l *Launcher) personWords(who string) string {
	switch actor.KindOf(who) {
	case actor.Agent:
		if n := paths.AgentHandle(l.Home, who); n != who {
			return n
		}
	case actor.Person:
		if who == actor.OwnerKey {
			return "the founder"
		}
	}
	return who
}

// sourceOf names where a person edits a part that has no value to show.
func (l *Launcher) sourceOf(agent, part string) string {
	switch part {
	case "team instructions":
		return "team.yaml or the agent list"
	case rulesPart:
		if l.Ports.Routing != nil {
			if teams := team.RuleTeams(l.Q, agent, l.Ports.Routing()); len(teams) > 0 {
				return strings.Join(teams, ", ")
			}
		}
	}
	return ""
}

// WhatChanged is whatChanged for the restart notice, "" when it cannot be read.
func (l *Launcher) WhatChanged(agent string) string {
	s, _, _ := l.whatChanged(agent)
	return s
}

// readable is a setting's value as the line says it: a model without its vendor's prefix, no value as the default.
func readable(v string) string {
	if v == "" {
		return "default"
	}
	return strings.TrimPrefix(v, "claude-")
}

// LaunchedAtOf is the stamp of the launch this agent_state row records, as written; "" when none.
func LaunchedAtOf(row *db.AgentState) string {
	var parts map[string]json.RawMessage
	if json.Unmarshal([]byte(row.LaunchParts.String), &parts) != nil {
		return ""
	}
	v, ok := parts[LaunchedAt]
	if !ok || string(v) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	return string(v)
}

// TakePrompt reads and removes a prompt file the daemon left for this launch. Taken only after Build has let the launch go ahead: a
// launch that refuses leaves the file, which is how the daemon tells its own session from one somebody else started. Removed as it is
// read, so no later launch picks it up: a prompt belongs to one launch.
func TakePrompt(path string) string {
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if os.Remove(path) != nil {
		return ""
	}
	return string(b)
}

// CommandFor is the --command string to hand to the host; account fixes where a move lands, why is what the session's start is
// recorded as, by who asked for it.
func CommandFor(account, why, by, agent string) string {
	exe := paths.HlExe()
	if v, ok := paths.Getenv("BIN"); ok && v != "" {
		exe = v
	}
	if exe == "" {
		exe = "hl"
	}
	cmd := exe + " launch " + agent
	if account != "" {
		cmd += " --account " + shlexQuote(account)
	}
	if why != "" {
		cmd += " --why " + shlexQuote(why)
	}
	if by != "" {
		cmd += " --by " + shlexQuote(by)
	}
	return cmd
}

// LaunchPort is a Launcher as the Launch the manager and spawn read it through.
type LaunchPort struct{ L *Launcher }

func (p LaunchPort) WakeWait(q db.Q, agent string, row *db.AgentState) *WakeWait {
	return p.L.WakeWait(q, agent, row)
}

func (p LaunchPort) WorkspaceWait(q db.Q, agent string) *WakeWait { return p.L.WorkspaceWait(q, agent) }

func (p LaunchPort) AccountFor(q db.Q, agent string, resuming bool) string {
	return p.L.ChooseAccount(agent, resuming, "")
}

func (p LaunchPort) Placing() func() { return p.L.Placing() }

func (p LaunchPort) ManualSplit(agent string) bool { return p.L.ManualSplit(agent) }

func (p LaunchPort) AccountUsage(acct string) AccountStatus { return p.L.Ports.Usage.Read(acct) }

func (p LaunchPort) SlowestPaced(q db.Q, agent string, skip string) string {
	return p.L.SlowestPaced(q, agent, skip)
}

func (p LaunchPort) RoomElsewhere(q db.Q, agent string, exclude map[string]string, fitOnly bool) (string, string) {
	return p.L.RoomElsewhere(q, agent, exclude, fitOnly)
}

func (p LaunchPort) ClosestRoom(q db.Q, agent string, row *db.AgentState, exclude map[string]string) string {
	return p.L.ClosestRoom(q, agent, row, exclude)
}

func (p LaunchPort) RoomPastFit(q db.Q, agent string, row *db.AgentState, acct string) string {
	return p.L.RoomPastFit(q, agent, row, acct)
}

func (p LaunchPort) RoomOn(q db.Q, agent string, row *db.AgentState, acct string) string {
	return p.L.RoomOn(q, agent, row, acct)
}

func (p LaunchPort) WorthRepick(own, to string) bool { return p.L.WorthRepick(own, to) }

func (p LaunchPort) RoomNow(q db.Q, agent string, skip string) (string, string) {
	return p.L.RoomNow(q, agent, skip)
}

var _ Launch = LaunchPort{}

func (LaunchPort) DescribeWakeWait(w *WakeWait) string { return DescribeWakeWait(w) }

func (LaunchPort) CommandFor(agent, account, why, by string) (string, error) {
	return CommandFor(account, why, by, agent), nil
}

// ChooseAccount is whether this agent's next fresh launch has an account to run on.
func (p LaunchPort) ChooseAccount(_ db.Q, agent string) bool {
	return p.L.ChooseAccount(agent, false, "") != ""
}

func (p LaunchPort) NoRoom(agent string) string { return p.L.NoRoom(agent) }

// SandboxAllowWrite are the folders a sandboxed session of agent may write beyond its worktrees and its cache folder: its repo's .git,
// where a worktree's commits and refs live (the CLI's sandbox keeps .git/hooks and .git/config read-only within it).
func (l *Launcher) SandboxAllowWrite(agent string) []string {
	if repo := l.Roster.RepoOf(agent); repo != "" {
		if fi, err := os.Stat(filepath.Join(repo, ".git")); err == nil && fi.IsDir() {
			return []string{filepath.Join(repo, ".git")}
		}
	}
	return nil
}

// SandboxCacheDir is agent's own build caches, made here. Any sandboxed session may write the whole home (the shared settings allow it),
// so this keeps shared caches out of its reach, not its caches from other agents.
func (l *Launcher) SandboxCacheDir(agent string) string {
	d := paths.CacheDir(l.Home, agent)
	if err := os.MkdirAll(d, 0o755); err != nil {
		l.warn("%s: cannot make its cache folder %s: %v", agent, d, err)
	}
	return d
}
