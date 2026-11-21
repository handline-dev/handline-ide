package lifecycle

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	cli "github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/agentctl/verdict"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/yamlcompat"
	"github.com/handline/handline/go/protocol/sessiontoken"
	"github.com/handline/handline/go/scheduler/access"
)

// fakeDriver has no account facts of its own: the launch tests never ask.
type fakeDriver struct {
	vendor string
	cli.Account
}

func (d fakeDriver) Name() string                       { return d.vendor }
func (fakeDriver) IsResume(extra []string) bool         { return len(extra) > 0 && extra[0] == "--resume" }
func (fakeDriver) Screen() term.Driver                  { return term.Blind{} }
func (fakeDriver) Binary() string                       { return "/bin/vendor" }
func (fakeDriver) Hooks() cli.Hooks                     { return nil }
func (fakeDriver) Deliver(cli.DeliverEnv) cli.Port      { return nil }
func (fakeDriver) AutoModeArgv() []string               { return []string{"--permission-mode", "auto"} }
func (fakeDriver) PermissionModeArgv(m string) []string { return []string{"--permission-mode", m} }
func (fakeDriver) LaunchArgv(exe string, o cli.LaunchArgs) []string {
	return []string{exe, "--effort", o.Effort, "--disallowed", o.Disallowed}
}
func (fakeDriver) LaunchEnv(string, string, string) map[string]string { return nil }
func (fakeDriver) PromptArgv(p string) []string                       { return []string{"--", p} }
func (fakeDriver) TranscriptDir(string, string) (string, bool)        { return "", false }

type fakeVendors struct{ account string }

func (fakeVendors) ForAgent(string) (cli.Agent, error) { return fakeDriver{vendor: "claude"}, nil }
func (fakeVendors) AgentVendor(string) string          { return "claude" }
func (v fakeVendors) AccountVendor(string) string      { return v.account }
func (fakeVendors) ConfigDir(string) string            { return "" }

type fakeUsage struct {
	st      AccountStatus
	by      map[string]AccountStatus
	pick    string
	others  int
	exclude *string        // the agent ActiveOn was asked to leave out
	active  map[string]int // agents mid-turn by account, over others
	midTurn []string       // the agents MidTurn names
	head    map[string]float64
	urgent  map[string]float64 // Urgency by account; unlisted is unknown
	reasons []usage.Reason     // PickReasons
	manual  bool               // account_split manual
	unfit   map[string]int     // StartFit: room of each account that does not fit
}

func (u fakeUsage) ManualSplit() bool { return u.manual }

func (u fakeUsage) Urgency(acct string) (float64, bool) {
	v, ok := u.urgent[acct]
	return v, ok
}
func (u fakeUsage) Headroom(acct string) (float64, bool) {
	h, ok := u.head[acct]
	return h, ok
}
func (u fakeUsage) MidTurn(_ db.Q, agent string) []string {
	return slices.DeleteFunc(slices.Clone(u.midTurn), func(a string) bool { return a == agent })
}

func (u fakeUsage) ActiveOn(_ db.Q, acct, agent string) (int, error) {
	if u.exclude != nil {
		*u.exclude = agent
	}
	return u.others + u.active[acct], nil
}

// StartFit: an account listed in unfit does not fit, with its room; any other fits.
func (u fakeUsage) StartFit(acct, _ string) (string, int) {
	if r, ok := u.unfit[acct]; ok {
		return "unfit", r
	}
	return "", 100
}

func (u fakeUsage) Read(acct string) AccountStatus {
	if st, ok := u.by[acct]; ok {
		return st
	}
	return u.st
}
func (fakeUsage) ShortBlock() int                               { return 95 }
func (fakeUsage) LongBlock() int                                { return 98 }
func (u fakeUsage) PickAccount([]string, string) string         { return u.pick }
func (u fakeUsage) PickReasons([]string, string) []usage.Reason { return u.reasons }
func (fakeUsage) NotFor(string, string) bool                    { return false }

// Admit hands verdict.Admit the facts the fixtures stand for: a trusted fill at the block is the wall.
func (u fakeUsage) Admit(acct, _, _ string) verdict.Admission {
	in := verdict.Account{Assignable: true}
	if st := u.Read(acct); st.ShortTrusted != nil && *st.ShortTrusted >= u.ShortBlock() {
		in.Wall = fmt.Sprintf("5h usage at the wall (%d%%)", *st.ShortTrusted)
	}
	return verdict.Admit(in)
}

type fakeAccounts struct{ loggedIn bool }

func (a fakeAccounts) MayStart(string) bool                   { return a.loggedIn }
func (fakeAccounts) LoginCommand(acct string) (string, error) { return "login " + acct, nil }
func (fakeAccounts) EnvFrom(_ map[string]string, acct, agent string) map[string]string {
	return map[string]string{"HANDLINE_ACCOUNT": acct, "HANDLINE_AGENT": agent}
}

type launchRig struct {
	l     *Launcher
	warns []string
	execd []string
	argv  []string
}

func newLaunchRig(t *testing.T) *launchRig {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HANDLINE_AGENT", "")
	t.Setenv("HANDLINE_HOME", home)
	r := &launchRig{}
	teamtest.Set(t, home, "ann", "vendor", "claude", "model", "m1", "effort", "high")
	teamtest.Account(t, home, "main") // the account the tests launch on
	roster := team.NewRoster(home)
	r.l = &Launcher{Q: teamtest.DB(t, home), Home: home, Roster: roster, Tokens: sessiontoken.New(home), Ports: LaunchPorts{
		Usage:          fakeUsage{st: AccountStatus{Exists: true}},
		Accounts:       fakeAccounts{true},
		Vendors:        fakeVendors{"claude"},
		CurrentAccount: func(db.Q, string) string { return "" },
		TeamRules:      func(string) []string { return nil },
		Routing:        func() *team.Team { return nil },
		RegisterWorker: func(string, string) error { return nil },
	}}
	return r
}

// site is the caller the rig launches for: this process's folder, in a terminal.
func (r *launchRig) site() *Site {
	cwd, _ := os.Getwd()
	return &Site{Cwd: cwd, Env: map[string]string{}, Handle: "term_1", WorktreeID: "wt_1", PID: os.Getpid(), HlExe: "hl"}
}

func (r *launchRig) build(agent string, extra []string, force bool, account string) ([]string, map[string]string, error) {
	return r.l.Build(r.site(), agent, extra, force, account)
}

// prepare is a launch to its end; what a caller would exec is kept in execd (the command, then its environment) and argv.
func (r *launchRig) prepare(agent string, extra []string, force bool, promptFile, account, why, by string) error {
	site := r.site()
	site.Warn = func(s string) { r.warns = append(r.warns, s) }
	got, err := r.l.Prepare(site, agent, extra, force, promptFile, account, why, by)
	if err != nil {
		return err
	}
	r.execd = append([]string{got.Argv[0]}, got.Env...)
	r.argv = got.Argv
	return nil
}

func TestBuildRefusesWhatMustNotGoAhead(t *testing.T) {
	r := newLaunchRig(t)
	cases := []struct {
		name string
		set  func()
		want string
	}{
		{"account of another vendor", func() { r.l.Ports.Vendors = fakeVendors{"codex"} }, "is a codex account - refusing to start it there"},
		{"account not logged in", func() { r.l.Ports.Accounts = fakeAccounts{false} }, "is not logged in.\nA human has to do this once"},
		{"short window at the wall", func() {
			pct := 96
			r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true, ShortTrusted: &pct, ShortLabel: "5h", ShortReset: 1700000000}}
		}, "is at 96% of the 5h limit (>= 95%) - refusing to start.\nresets at 1700000000; use --force to override"},
	}
	for _, c := range cases {
		r.l.Ports.Vendors, r.l.Ports.Accounts = fakeVendors{"claude"}, fakeAccounts{true}
		r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}}
		c.set()
		_, _, err := r.build("ann", nil, false, "main")
		ref, ok := err.(*Refusal)
		if !ok || !strings.Contains(ref.Msg, c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	// --force passes the wall.
	pct := 96
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true, ShortTrusted: &pct}}
	if _, _, err := r.build("ann", nil, true, "main"); err != nil {
		t.Fatalf("force: %v", err)
	}
}

func TestBuildPutsTheModeAfterTheVendorsLineAndTheCallersArgsLast(t *testing.T) {
	r := newLaunchRig(t)
	r.l.Ports.CurrentAccount = func(db.Q, string) string { return "main" } // a resume goes back to its recorded account
	argv, env, err := r.build("ann", []string{"--resume", "s1"}, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(argv, " "); !strings.HasPrefix(got, "/bin/vendor --effort high --disallowed "+DisallowedTools+" --permission-mode auto") || !strings.HasSuffix(got, "--resume s1") {
		t.Fatalf("%s", got)
	}
	if env["HANDLINE_ACCOUNT"] != "main" {
		t.Fatalf("%v", env)
	}
	// A mode of the caller's own wins and is not doubled.
	argv, _, _ = r.build("ann", []string{"--permission-mode=plan"}, false, "main")
	if strings.Count(strings.Join(argv, " "), "permission-mode") != 1 {
		t.Fatalf("%v", argv)
	}
}

func TestALaunchFromAPaneStampedForAnotherAgentIsRefused(t *testing.T) {
	r := newLaunchRig(t)
	stamp := "bob"
	r.l.Ports.PaneAgent = func(string) string { return stamp }
	if err := r.prepare("ann", nil, false, "", "main", "", ""); err == nil || !strings.Contains(err.Error(), "opened for bob") || r.execd != nil {
		t.Fatalf("refused: %v, exec %v", err, r.execd)
	}
	for _, own := range []string{"ann", ""} { // her own pane, or one the daemon did not stamp
		stamp = own
		if err := r.prepare("ann", nil, false, "", "main", "", ""); err != nil || r.execd == nil {
			t.Fatalf("stamp %q: %v", own, err)
		}
	}
}

func TestExecAgentKeepsHowItStartedAndWhereItRunsThenExecs(t *testing.T) {
	r := newLaunchRig(t)
	if err := db.SetHandoff(r.l.Q, "ann", "left off here"); err != nil {
		t.Fatal(err)
	}
	if err := r.prepare("ann", nil, false, "", "main", "wake", "handline"); err != nil {
		t.Fatal(err)
	}
	if len(r.execd) == 0 || r.execd[0] != "/bin/vendor" {
		t.Fatalf("not exec'd: %v", r.execd)
	}
	if h, _ := db.GetHandoff(r.l.Q, "ann"); h != "" {
		t.Fatalf("a launch nobody waited on leaves no handoff behind: %q", h)
	}
	row, err := db.GetAgentState(r.l.Q, "ann")
	if err != nil || row == nil {
		t.Fatalf("%v %v", row, err)
	}
	if row.Terminal.String != "term_1" || row.Worktree.String != "wt_1" || row.Account.String != "main" || row.LaunchFingerprint.String == "" {
		t.Fatalf("%+v", row)
	}
	// With a prompt file the handoff is left for the daemon, and the file is taken.
	_ = db.SetHandoff(r.l.Q, "ann", "again")
	pf := filepath.Join(t.TempDir(), "p")
	_ = os.WriteFile(pf, []byte("hello"), 0o644)
	if err := r.prepare("ann", nil, false, pf, "main", "", ""); err != nil {
		t.Fatal(err)
	}
	if h, _ := db.GetHandoff(r.l.Q, "ann"); h != "again" {
		t.Fatalf("%q", h)
	}
	if _, err := os.Stat(pf); err == nil {
		t.Fatal("prompt file kept")
	}
}

func TestAHandLaunchStartsTheFirstTurnWhenThereIsWorkToPickUp(t *testing.T) {
	r := newLaunchRig(t)
	last := func() string { return r.argv[len(r.argv)-1] }
	_ = db.SetHandoff(r.l.Q, "ann", "h")
	if err := r.prepare("ann", nil, false, "", "main", "", ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(last(), "handoff is in your system prompt") {
		t.Fatalf("with a handoff: %v", r.argv)
	}
	tid, _ := db.NewTask(r.l.Q, "held", "ann", "boss", 0, 2, "", 0, "")
	if err := db.SetCurrentTask(r.l.Q, "ann", tid, true); err != nil {
		t.Fatal(err)
	}
	if err := r.prepare("ann", nil, false, "", "main", "", ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(last(), fmt.Sprintf("hl task show T%d", tid)) {
		t.Fatalf("holding a task: %v", r.argv)
	}
	_ = db.SetCurrentTask(r.l.Q, "ann", 0, false)
	if err := r.prepare("ann", nil, false, "", "main", "", ""); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(r.argv, "--") {
		t.Fatalf("nothing to pick up, no prompt: %v", r.argv)
	}
}

func TestARefusedLaunchLeavesThePromptFileAndExecsNothing(t *testing.T) {
	r := newLaunchRig(t)
	r.l.Ports.Accounts = fakeAccounts{false}
	pf := filepath.Join(t.TempDir(), "p")
	_ = os.WriteFile(pf, []byte("hello"), 0o644)
	if err := r.prepare("ann", nil, false, pf, "main", "", ""); err == nil {
		t.Fatal("expected refusal")
	}
	if _, err := os.Stat(pf); err != nil || r.execd != nil {
		t.Fatalf("%v %v", err, r.execd)
	}
}

func addAccounts(t *testing.T, r *launchRig, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.MkdirAll(teamtest.AccountDir(t, r.l.Home, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRoomIsTheLoggedInAccountNothingSaysIsBad(t *testing.T) {
	r := newLaunchRig(t)
	if got := r.l.NoRoom("ann"); got != "no account is registered - add one in the app's Accounts page first" {
		t.Fatal(got)
	}
	addAccounts(t, r, "a", "b")
	r.l.Ports.Accounts = fakeAccounts{false}
	if got := r.l.NoRoom("ann"); !strings.Contains(got, "no account is logged in") || !strings.Contains(got, "login a") {
		t.Fatal(got)
	}
	r.l.Ports.Accounts = fakeAccounts{true}
	if got := r.l.FindRoom("ann"); got != "a" {
		t.Fatalf("first fit: %q", got)
	}
	r.l.Ports.Usage = fakeUsage{pick: "b"}
	if got := r.l.ChooseAccount("ann", false, ""); got != "b" {
		t.Fatalf("picked: %q", got)
	}
	if got := r.l.ChooseAccount("ann", false, "a"); got != "a" {
		t.Fatalf("a move's destination stands: %q", got)
	}
	r.l.Ports.CurrentAccount = func(db.Q, string) string { return "b" }
	if got := r.l.ChooseAccount("ann", true, ""); got != "b" {
		t.Fatalf("a resume goes back: %q", got)
	}
	if got := r.l.ChooseAccount("ann", true, "a"); got != "a" {
		t.Fatalf("a moved resume goes where it was moved: %q", got)
	}
}

func TestAWalledWakeNamesTheAccountThatFreesFirst(t *testing.T) {
	r := newLaunchRig(t)
	addAccounts(t, r, "a", "b")
	walled := func(pct int, reset int64) AccountStatus {
		return AccountStatus{Exists: true, ShortTrusted: &pct, ShortLabel: "5h", ShortReset: reset}
	}
	r.l.Ports.Usage = fakeUsage{by: map[string]AccountStatus{"a": walled(97, 2000), "b": walled(99, 1000)}}
	if r.l.FindRoom("ann") != "" {
		t.Fatal("a full account has no room")
	}
	row := &db.AgentState{Agent: "ann", ClosedAt: sql.NullInt64{Int64: 5, Valid: true}}
	w := r.l.WakeWait(r.l.Q, "ann", row)
	if w == nil || w.Account != "b" || w.Pct != 99 || w.Assumed {
		t.Fatalf("%+v", w)
	}
	if got := DescribeWakeWait(w); !strings.HasPrefix(got, "waiting for b's 5h window (99%, rolls over ") {
		t.Fatal(got)
	}
	if got := r.l.NoRoom("ann"); !strings.HasPrefix(got, "no account can take one more agent - waiting for b's") {
		t.Fatal(got)
	}
	// Each account's reason from the pick is named, before the account that frees first.
	r.l.Ports.Usage = fakeUsage{by: map[string]AccountStatus{"a": walled(97, 2000), "b": walled(99, 1000)},
		reasons: []usage.Reason{{Account: "a", Why: "5h usage at the wall (97%)"}, {Account: "b", Why: "5h usage at the wall (99%)"}}}
	if got := r.l.NoRoom("ann"); !strings.HasPrefix(got, "no account can take one more agent (a: 5h usage at the wall (97%); b: 5h usage at the wall (99%)) - waiting for b's") {
		t.Fatal(got)
	}
	// One person-chosen account says what stands against it.
	if got := r.l.RoomOn(r.l.Q, "ann", row, "a"); got != "is not a place for it: 5h usage at the wall (97%)" {
		t.Fatal(got)
	}
	// A start with every account paused waits on one, as at a wall, rather than being refused at the launch.
	r.l.Ports.Usage = fakeUsage{by: map[string]AccountStatus{"a": {Exists: true, Paused: true}, "b": {Exists: true, Paused: true}}}
	if pw := r.l.WakeWait(r.l.Q, "ann", row); pw == nil || !pw.Paused {
		t.Fatalf("every account paused: %+v", pw)
	}
	r.l.Ports.Usage = fakeUsage{by: map[string]AccountStatus{"a": walled(97, 2000), "b": walled(99, 1000)}}
	if got := r.l.WakeWaitReason(r.l.Q, "ann", row); got != DescribeWakeWait(w) {
		t.Fatal(got)
	}
	// A resume is pinned to the account it is tied to, and a guess says so.
	row.ResumeSession = sql.NullString{String: "s1", Valid: true}
	r.l.Ports.AccountReading = func(db.Q, string) (string, string, bool) { return "a", "assumed", true }
	if w := r.l.WakeWait(r.l.Q, "ann", row); w == nil || w.Account != "a" || !w.Assumed ||
		!strings.Contains(DescribeWakeWait(w), "assumed account a") {
		t.Fatalf("%+v", w)
	}
	// Open and live agents are never walled; a longest-window wall reads as a date.
	if r.l.WakeWait(r.l.Q, "ann", &db.AgentState{}) != nil {
		t.Fatal("an open agent was walled")
	}
	if got := WaitClock(&WakeWait{}, "?"); got != "?" {
		t.Fatal(got)
	}
	if got := WaitClock(&WakeWait{Reset: 86400 * 3, IsLong: true}, "?"); len(got) != len("01-02 15:04") {
		t.Fatal(got)
	}
}

func TestCommandForNamesTheLaunchAndQuotesWhatIsGiven(t *testing.T) {
	t.Setenv("HANDLINE_BIN", "/x/hl")
	if got := CommandFor("", "", "", "ann"); got != "/x/hl launch ann" {
		t.Fatal(got)
	}
	if got := CommandFor("acct 2", "wake up", "boss", "ann"); got != "/x/hl launch ann --account 'acct 2' --why 'wake up' --by boss" {
		t.Fatal(got)
	}
}

func TestDriftedIsAFingerprintThatDiffersFromTheRecordedOne(t *testing.T) {
	r := newLaunchRig(t)
	fp, err := r.l.Fingerprint("ann")
	if err != nil || fp == "" {
		t.Fatal(fp, err)
	}
	rec, _ := r.l.LaunchRecord("ann")
	row := &db.AgentState{LaunchFingerprint: sql.NullString{String: fp, Valid: true}, LaunchParts: sql.NullString{String: rec, Valid: true}}
	if r.l.Drifted("ann", row) {
		t.Fatal("drifted from itself")
	}
	row.LaunchFingerprint.String = "other"
	if !r.l.Drifted("ann", row) || !(LaunchPort{r.l}).Drifted("ann", row) {
		t.Fatal("a different recipe did not drift")
	}
	if LaunchedAtOf(row) == "" {
		t.Fatal("the launch stamp was not read back")
	}
	if r.l.Drifted("nobody", row) {
		t.Fatal("an unreadable recipe is not drift")
	}
}

func TestLaunchPortAnswersForTheLauncher(t *testing.T) {
	r := newLaunchRig(t)
	p := LaunchPort{r.l}
	if p.ChooseAccount(r.l.Q, "ann") {
		t.Fatal("no account registered")
	}
	addAccounts(t, r, "a")
	if !p.ChooseAccount(r.l.Q, "ann") || p.NoRoom("ann") == "" {
		t.Fatal("room expected")
	}
	if cmd, err := p.CommandFor("ann", "a", "why", ""); err != nil || !strings.Contains(cmd, "launch ann --account a --why why") {
		t.Fatal(cmd, err)
	}
	if p.WakeWait(r.l.Q, "ann", &db.AgentState{}) != nil {
		t.Fatal("walled")
	}
	if p.DescribeWakeWait(&WakeWait{Account: "a", Window: "5h", Pct: 99}) == "" {
		t.Fatal("empty")
	}
}

func TestALaunchWithoutTheTokenStoreIsRefusedNotRunBare(t *testing.T) {
	r := newLaunchRig(t)
	r.l.Tokens = nil
	if _, _, err := r.build("ann", nil, false, "main"); err == nil || !strings.Contains(err.Error(), "token store") {
		t.Fatalf("%v", err)
	}
	if err := r.prepare("ann", nil, false, "", "main", "", ""); err == nil || r.execd != nil {
		t.Fatalf("%v %v", err, r.execd)
	}
}

func TestTheFounderKeyFolderIsHiddenFromTheSession(t *testing.T) {
	r := newLaunchRig(t)
	hide := ""
	r.l.Ports.Vendors = hidingVendors{&hide}
	if err := r.prepare("ann", nil, false, "", "main", "", ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hide, r.l.Tokens.HiddenFolders()[0]) {
		t.Fatalf("hidden %q", hide)
	}
}

type hidingVendors struct{ hide *string }

func (v hidingVendors) ForAgent(string) (cli.Agent, error) {
	return hidingDriver{fakeDriver{vendor: "claude"}, v.hide}, nil
}
func (hidingVendors) AgentVendor(string) string   { return "claude" }
func (hidingVendors) AccountVendor(string) string { return "claude" }
func (hidingVendors) ConfigDir(string) string     { return "" }

type hidingDriver struct {
	fakeDriver
	hide *string
}

func (d hidingDriver) LaunchArgv(exe string, o cli.LaunchArgs) []string {
	*d.hide = strings.Join(o.Hide, "\n")
	return d.fakeDriver.LaunchArgv(exe, o)
}

// No cap: however many are on its own account, a resume waits there only while it is paused.
func TestAResumeWaitsOnItsOwnAccountOnlyWhileItIsPaused(t *testing.T) {
	r := newLaunchRig(t)
	r.l.Ports.AccountReading = func(db.Q, string) (string, string, bool) { return "a", "recorded", true }
	q := r.l.Q
	db.SetAgentState(q, "ann", states.Idle, 0, "")
	db.SetClosed(q, "ann", "s-ann")
	row, _ := db.GetAgentState(q, "ann")
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}, others: 9}
	if w := r.l.WakeWait(q, "ann", row); w != nil {
		t.Fatalf("nine agents on it held the resume: %+v", w)
	}
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true, Paused: true}}
	if w := r.l.WakeWait(q, "ann", row); w == nil || !w.Paused {
		t.Fatalf("a paused account held nothing: %+v", w)
	}
}

// A move off its own account goes to the other account with the most headroom (no week can be read here), never a walled or paused one;
// no older waiter is counted ahead, and how many are on an account holds nothing.
func TestAMoveGoesToTheMostHeadroomPastWallsAndPausesNotPastAnyCount(t *testing.T) {
	r := newLaunchRig(t)
	pct := 10
	ok := AccountStatus{Exists: true, ShortTrusted: &pct}
	hot := AccountStatus{Exists: true, ShortTrusted: func() *int { n := 99; return &n }()}
	r.l.Ports.Accounts = fakeAccounts{true}
	r.l.Ports.CurrentAccount = func(db.Q, string) string { return "a" }
	r.l.Ports.WaitingFor = func(db.Q, string) bool { return true }
	r.l.Ports.Usage = fakeUsage{st: ok, by: map[string]AccountStatus{"walled": hot, "paused": {Exists: true, ShortTrusted: &pct, Paused: true}},
		head: map[string]float64{"a": 0.99, "walled": 0.95, "paused": 0.95, "b": 0.9, "c": 0.5}, others: 9}
	home, q := r.l.Home, r.l.Q
	for _, a := range []string{"a", "b", "c", "walled", "paused"} {
		os.MkdirAll(teamtest.AccountDir(t, home, a), 0o755)
	}
	db.SetAgentState(q, "ann", states.Idle, 0, "")
	db.SetClosed(q, "ann", "s-ann")
	row, _ := db.GetAgentState(q, "ann")
	if got := r.l.SlowestPaced(q, "ann", "a"); got != "b" {
		t.Fatalf("the most headroom among the others that can take it: %q", got)
	}
	if got, why := r.l.RoomNow(q, "ann", "a"); got != "b" {
		t.Fatalf("a person's start: %q %s", got, why)
	}
	if why := r.l.RoomOn(q, "ann", row, "paused"); why != "is paused" {
		t.Fatalf("a move onto a paused account: %q", why)
	}
	for _, a := range []string{"b", "c"} {
		os.RemoveAll(teamtest.AccountDir(t, home, a))
	}
	if got, why := r.l.RoomElsewhere(q, "ann", map[string]string{"a": ""}, false); got != "" || !strings.HasPrefix(why, "no account can take one more agent") {
		t.Fatalf("only walled and paused left: %q %s", got, why)
	}
}

func TestAnAgentIsToldItsTeamFromTheRoster(t *testing.T) {
	r := newLaunchRig(t)
	home := r.l.Home
	for _, n := range []string{"manager", "manager.marina", "manager.keeper", "lone"} {
		teamtest.Set(t, home, n, "vendor", "claude")
	}
	k := func(n string) string { return teamtest.Key(t, home, n) }
	mgrK, devK, keeperK, loneK := k("manager"), k("manager.marina"), k("manager.keeper"), k("lone")
	tm, err := team.LoadTeam(home)
	if err != nil {
		t.Fatal(err)
	}
	r.l.Ports.Routing = func() *team.Team { return tm }
	r.l.Ports.TeamRules = func(agent string) []string {
		if err := team.SeedRules(r.l.Q, tm, 1); err != nil {
			t.Fatal(err)
		}
		return team.Rules(r.l.Q, agent, tm, func(string) string { return "" })
	}
	context := func(agent string) string {
		text, err := r.l.ContextFor(agent)
		if err != nil {
			t.Fatal(err)
		}
		return text
	}
	culture := []string{"Keep messages short", "no progress or FYI messages", "Never schedule your own wakeup", "Working with peers",
		"Check main before you open work", "founder's own words"}
	// A team that empties its rules file is not seeded again: it gets only the facts.
	context(loneK)
	lone, _ := paths.WorkspaceKeyIn(r.l.Q, "lone")
	if _, err := db.SetInstruction(r.l.Q, lone, db.PartRules, "", "p1", "", 2); err != nil {
		t.Fatal(err)
	}
	mgr, dev, scout := context(mgrK), context(devK), context(loneK)
	rules := "# Rules for team manager\n" + strings.TrimSpace(team.DefaultRules)
	for who, text := range map[string]string{"manager": mgr, "manager.marina": dev} {
		if !strings.Contains(text, rules) {
			t.Errorf("%s: the seeded team file is not in the prompt:\n%s", who, text)
		}
	}
	for _, w := range culture {
		if strings.Contains(r.l.yourTeam(mgrK), w) || strings.Contains(scout, w) {
			t.Errorf("%q is written by code, not read from the team file:\n%s", w, scout)
		}
	}
	// No one reports to anyone: the lead and a member are told the same, bar who the contact is and the contact's rule for a story.
	contact := "The workspace's contact is `lead`: work from another workspace comes to it, and it passes that work to the peer it fits."
	lead := "You are the workspace's contact: work from another workspace comes to you. Pass it to the peer it fits, and do your own work too.\n" +
		team.Protocol("story") + "\n"
	want := map[string]string{
		mgrK:  "# Your team\nYour peers in workspace `manager`: `marina`, `keeper`.\n" + lead,
		devK:  "# Your team\nYour peers in workspace `manager`: `lead`, `keeper`.\n" + contact + "\n",
		loneK: "# Your team\n" + lead,
	}
	// The prompt names agents by name: no agent's key (aN) is in it.
	keyRe := regexp.MustCompile(`\ba[0-9]+\b`)
	for _, k := range []string{mgrK, devK, keeperK, loneK} {
		if got := context(k); keyRe.MatchString(got) {
			t.Errorf("%s: an agent's key in its prompt: %q", k, keyRe.FindAllString(got, -1))
		}
	}
	if got := context(devK); !strings.Contains(got, "You are `marina`, one agent in a Handline team.") {
		t.Errorf("marina is not told its name:\n%s", got)
	}
	for k, head := range want {
		got := r.l.yourTeam(k)
		if !strings.HasPrefix(got, head+team.Protocol("ends")+"\nGive anyone work with `hl task new --assign <name>") {
			t.Errorf("%s: the team text does not open with its peers and contact:\n%s", k, got)
		}
		if strings.Contains(got, "report to") || strings.Contains(got, "Your reports") || !strings.Contains(got, "--blocked-by T<n>") ||
			!strings.Contains(got, "hl task discard T<n>") || !strings.Contains(got, "its done goes back to whoever opened the task") {
			t.Errorf("%s: a reporting line is left, or the work commands are missing:\n%s", k, got)
		}
		if strings.Contains(got, "or for the contact when you do not know who") != (k == devK) {
			t.Errorf("%s: only an agent that is not the contact is pointed at the contact when blocked:\n%s", k, got)
		}
		for _, p := range []string{"founder", "land", "progress", "terminal"} {
			if !strings.Contains(got, team.Protocol(p)) {
				t.Errorf("%s: the %s line is missing:\n%s", k, p, got)
			}
		}
	}
	for who, text := range map[string]string{"manager": mgr, "manager.marina": dev, "lone": scout} {
		if !strings.Contains(text, r.l.yourTeam(map[string]string{"manager": mgrK, "manager.marina": devK, "lone": loneK}[who])) {
			t.Errorf("%s: the team text is not in the prompt:\n%s", who, text)
		}
	}
	// The founder's words typed at an agent: it is told to answer them with hl say, and the others are not told of typing.
	r.l.FounderTyped = func(a string) bool { return a == devK }
	if typed, other := r.l.yourTeam(devK), r.l.yourTeam(mgrK); !strings.Contains(typed, "answer with `hl say` too; your terminal text is not shown") ||
		!strings.Contains(typed, "what you write in your terminal reaches no one") ||
		!strings.Contains(typed, "delivery of a task comment is not typing: answer it on that task") || strings.Contains(other, "type straight into your terminal") {
		t.Errorf("typed:\n%s\n---\nnot typed:\n%s", typed, other)
	}
}

// argsDriver keeps what Build hands the driver.
type argsDriver struct {
	fakeDriver
	got *cli.LaunchArgs
}

func (d argsDriver) LaunchArgv(exe string, o cli.LaunchArgs) []string {
	*d.got = o
	return d.fakeDriver.LaunchArgv(exe, o)
}

type argsVendors struct {
	fakeVendors
	d cli.Agent
}

func (v argsVendors) ForAgent(string) (cli.Agent, error) { return v.d, nil }

func TestBuildHandsTheDriverTheAllowancesOfTheAgentsRepoOnly(t *testing.T) {
	r := newLaunchRig(t)
	var got cli.LaunchArgs
	r.l.Ports.Vendors = argsVendors{fakeVendors{"claude"}, argsDriver{fakeDriver{vendor: "claude"}, &got}}
	if err := r.l.Roster.Store.Put(team.RepoAllowanceNS, "handline", "weaker-network\npnpm-install"); err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	for name, want := range map[string]bool{"handline": true, "harbor-app": false} {
		repo := filepath.Join(ws, name) // a workspace's folder is a git checkout
		gitIn(t, ws, "init", "-q", repo)
		if err := r.l.Roster.SetKeys("ann", []yamlcompat.Pair{{Key: "sandbox", Val: true}, {Key: "repo", Val: repo}}); err != nil {
			t.Fatal(err)
		}
		got = cli.LaunchArgs{}
		if _, _, err := r.build("ann", nil, false, "main"); err != nil {
			t.Fatal(err)
		}
		if got.WeakerNetwork != want || (len(got.Excluded) > 0) != want {
			t.Errorf("%s: weaker %v, excluded %v", repo, got.WeakerNetwork, got.Excluded)
		}
	}
}

// The founder's grants are read rules of the session, after the attachments folder; a once grant goes into one session only.
func TestBuildHandsTheDriverTheGrants(t *testing.T) {
	r := newLaunchRig(t)
	var got cli.LaunchArgs
	r.l.Ports.Vendors = argsVendors{fakeVendors{"claude"}, argsDriver{fakeDriver{vendor: "claude"}, &got}}
	access.Put(r.l.Q, "ann", "/once", "once", 2)
	access.Put(r.l.Q, "ann", "/always", "always", 1)
	for i, want := range [][]string{{"/once", "/always"}, {"/always"}} {
		if i > 0 { // the first session started
			r.l.Q.Exec("INSERT INTO session_starts(agent, ts, cause, resumed, text, prompt) VALUES('ann', 1, 'wake', 0, '', '')")
		}
		if _, _, err := r.build("ann", nil, false, "main"); err != nil {
			t.Fatal(err)
		}
		if g := got.ReadAllow[1:]; !slices.Equal(g, want) || got.ReadAllow[0] != filepath.Join(r.l.Home, "attachments") {
			t.Fatalf("%v, want %v after the attachments", got.ReadAllow, want)
		}
	}
}

// A resume Build puts on an account that does not hold the session finds it there all the same: carried over from the old account.
func TestAResumeBuildCarriesTheSessionToItsAccount(t *testing.T) {
	r := newLaunchRig(t)
	r.l.Ports.CurrentAccount = func(db.Q, string) string { return "main" }
	main, _ := team.AccountConfigDir(r.l.Home, teamtest.Account(t, r.l.Home, "main"))
	old, _ := team.AccountConfigDir(r.l.Home, teamtest.Account(t, r.l.Home, "old"))
	os.MkdirAll(filepath.Join(old, "projects", "-wt"), 0o755)
	os.WriteFile(filepath.Join(old, "projects", "-wt", "s1.jsonl"), []byte("{}\n"), 0o600)
	if _, _, err := r.build("ann", []string{"--resume", "s1"}, false, ""); err != nil {
		t.Fatal(err)
	}
	if sessionIn(main, "s1") == "" {
		t.Fatal("the session was not carried to the account the resume runs on")
	}
}

// A fresh start is given the daemon's own record of where it was, with a handoff or without one.
func TestAFreshStartIsToldWhereItWas(t *testing.T) {
	r := newLaunchRig(t)
	tid, _ := db.NewTask(r.l.Q, "the held task", "ann", "boss", 0, 2, "", 0, "")
	if _, err := db.SetTaskQueued(r.l.Q, tid, "boss"); err != nil {
		t.Fatal(err)
	}
	for _, record := range []string{"", "left off here"} {
		_ = db.SetHandoff(r.l.Q, "ann", record)
		text, err := r.l.ContextFor("ann")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(text, "# Where you were (the daemon's record)") || !strings.Contains(text, "- the held task") {
			t.Errorf("record %q:\n%s", record, text)
		}
	}
}

// The handoff lists messages with the agent that opened the task it holds, not with a boss.
func TestAFreshStartListsMessagesWithItsTasksOpener(t *testing.T) {
	r := newLaunchRig(t)
	tid, _ := db.NewTask(r.l.Q, "the held task", "ann", "cleo", 0, 2, "", 0, "")
	if _, err := db.SetTaskQueued(r.l.Q, tid, "cleo"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddRequest(r.l.Q, db.NewRequest{Requester: "cleo", Target: "ann", Body: "take this"}); err != nil {
		t.Fatal(err)
	}
	text, err := r.l.ContextFor("ann")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "last messages with the founder and cleo") || !strings.Contains(text, "take this") {
		t.Fatalf("%s", text)
	}
}

func TestAMoveGoesWhereTheWeekRunsOutFirstAndAColdRepickNeedsAClearGain(t *testing.T) {
	r := newLaunchRig(t)
	for _, a := range []string{"a", "b", "c"} {
		if err := os.MkdirAll(teamtest.AccountDir(t, r.l.Home, a), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	q := r.l.Q
	db.SetAgentState(q, "ann", states.Idle, 0, "")
	db.SetClosed(q, "ann", "s-ann")
	// a has the most headroom against its pace line but its week lasts longest; c's week runs out first.
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}, head: map[string]float64{"a": 0.3, "b": 0.1, "c": 0.05},
		urgent: map[string]float64{"a": 0.8, "b": 1.5, "c": 2.4}}
	if got, why := r.l.RoomElsewhere(q, "ann", map[string]string{"x": "not here"}, false); got != "c" {
		t.Errorf("the account whose unused week runs out first: %q %q", got, why)
	}
	if got, _ := r.l.RoomElsewhere(q, "ann", map[string]string{"c": "full"}, false); got != "b" {
		t.Errorf("then the next: %q", got)
	}
	// As a new start: c's week is the most urgent, but past the move lines it is not a first choice while b fits.
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}, urgent: map[string]float64{"a": 0.8, "b": 1.5, "c": 2.4},
		unfit: map[string]int{"a": 20, "c": 5}}
	if got, why := r.l.RoomElsewhere(q, "ann", nil, false); got != "b" {
		t.Errorf("the one that fits, before a more urgent one that does not: %q %q", got, why)
	}
	if got, _ := r.l.RoomElsewhere(q, "ann", map[string]string{"b": "full"}, false); got != "a" {
		t.Errorf("none fits: the most room under its wall: %q", got)
	}
	if got, why := r.l.RoomElsewhere(q, "ann", map[string]string{"b": "full"}, true); got != "" || !strings.Contains(why, "a: unfit") {
		t.Errorf("a usage move with none that fits goes nowhere: %q %q", got, why)
	}
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}, head: map[string]float64{"a": 0.3, "b": 0.1}}
	if got, _ := r.l.RoomElsewhere(q, "ann", nil, false); got != "a" {
		t.Errorf("with no week readable, the most headroom as before: %q", got)
	}
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}, urgent: map[string]float64{"a": 1.0, "b": 1.4, "c": 1.6}}
	for to, want := range map[string]bool{"b": false, "c": true, "x": true} {
		if got := r.l.WorthRepick("a", to); got != want {
			t.Errorf("a cold wake on a, offered %s: %v, want %v", to, got, want)
		}
	}
}

func TestALaunchAllowsSubagentsAndKeepsTheOtherToolsDisallowed(t *testing.T) {
	got := strings.Split(DisallowedTools, ",")
	if slices.Contains(got, "Agent") {
		t.Fatalf("Agent is disallowed: %v", got)
	}
	for _, tool := range []string{"SendMessage", "ScheduleWakeup", "CronCreate"} {
		if !slices.Contains(got, tool) {
			t.Errorf("%s is allowed: %v", tool, got)
		}
	}
}

// A start says what its settings changed from the session recorded before it: a value a person can read as old -> new, the rest by name.
func TestWhatChangedSaysOldAndNewValues(t *testing.T) {
	r := newLaunchRig(t)
	if got, _, err := r.l.whatChanged("ann"); got != "" || err != nil {
		t.Fatalf("no record yet: %q %v", got, err)
	}
	record := func(parts string) {
		t.Helper()
		if err := db.SetLaunchFingerprint(r.l.Q, "ann", "fp", parts); err != nil {
			t.Fatal(err)
		}
	}
	rec, err := r.l.LaunchRecord("ann")
	if err != nil {
		t.Fatal(err)
	}
	record(rec)
	if got, _, err := r.l.whatChanged("ann"); got != "" || err != nil {
		t.Fatalf("nothing changed: %q %v", got, err)
	}
	set := func(model, effort string) {
		t.Helper()
		if err := r.l.Roster.SetKeys("ann", []yamlcompat.Pair{{Key: "model", Val: model}, {Key: "effort", Val: effort}}); err != nil {
			t.Fatal(err)
		}
	}
	set("claude-sonnet-5-5", "low")
	r.setPart(t, db.PartRole, "a new role")
	if got, _, err := r.l.whatChanged("ann"); got != "model changed on disk (m1 -> sonnet-5-5), effort changed on disk (high -> low), ROLE.md changed by the founder" || err != nil {
		t.Fatalf("%q %v", got, err)
	}
	// A session recorded before values were kept: the hash says it changed, not from what.
	var old map[string]any
	if err := json.Unmarshal([]byte(rec), &old); err != nil {
		t.Fatal(err)
	}
	delete(old, LaunchedValues)
	b, _ := json.Marshal(old)
	record(string(b))
	set("m1", "high")
	if got, _, _ := r.l.whatChanged("ann"); got != "ROLE.md changed by the founder" {
		t.Fatalf("back to the old model and effort, only the role differs: %q", got)
	}
	set("m2", "high")
	if got, _, _ := r.l.whatChanged("ann"); got != "model changed on disk, ROLE.md changed by the founder" {
		t.Fatalf("%q", got)
	}
}

// The team's rules and its instructions are named apart, the rules by the files a person edits.
func TestWhatChangedNamesTheTeamRulesFile(t *testing.T) {
	r := newLaunchRig(t)
	rules := "old rules"
	r.l.Ports.TeamRules = func(string) []string { return []string{rules} }
	r.l.Ports.Routing = func() *team.Team { return &team.Team{Teams: map[string][]string{"core": {"ann"}, "other": {"bob"}}} }
	r.rulesOf(t, "rules", "core", "other")
	core := "core"
	rec, err := r.l.LaunchRecord("ann")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetLaunchFingerprint(r.l.Q, "ann", "fp", rec); err != nil {
		t.Fatal(err)
	}
	rules = "new rules"
	if got, _, _ := r.l.whatChanged("ann"); got != "team rules changed ("+core+")" {
		t.Fatalf("%q", got)
	}
	// A session launched when both were one "team" part: its record has neither name, so nothing reads as changed and no one restarts.
	var old map[string]any
	if err := json.Unmarshal([]byte(rec), &old); err != nil {
		t.Fatal(err)
	}
	delete(old, "team instructions")
	delete(old, "team rules")
	old["team"] = "0badc0de"
	b, _ := json.Marshal(old)
	if err := db.SetLaunchFingerprint(r.l.Q, "ann", "fp", string(b)); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := r.l.whatChanged("ann"); got != "" {
		t.Fatalf("an old record's team part: %q", got)
	}
}

// A changed text part is said line by line, from the text the earlier launch kept; a record that kept none names the part alone.
func TestWhatChangedSaysTheLinesOfTheTextParts(t *testing.T) {
	r := newLaunchRig(t)
	rules := "# Rules\n- keep it short\n- reply in English"
	r.l.Ports.TeamRules = func(string) []string { return []string{rules} }
	r.setPart(t, db.PartRole, "builds\nreviews")
	rec, err := r.l.LaunchRecord("ann")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetLaunchFingerprint(r.l.Q, "ann", "fp", rec); err != nil {
		t.Fatal(err)
	}
	rules = "# Rules\n- keep it short\n- reply in Korean"
	r.setPart(t, db.PartRole, "builds\nreviews\ntests")
	_, lines, err := r.l.whatChanged("ann")
	want := []LineChange{
		{Part: "ROLE.md", Added: []string{"tests"}, Removed: []string{}, Text: "builds\nreviews\ntests"},
		{Part: "team rules", Added: []string{"- reply in Korean"}, Removed: []string{"- reply in English"}, Text: rules},
	}
	if err != nil || !reflect.DeepEqual(lines, want) {
		t.Fatalf("%+v %v", lines, err)
	}
	// Lines are kept in order up to changedBytesMax over the whole start (ROLE.md's 5 bytes first) and every line after counted;
	// a whole text past textBytesMax is not kept, and the parts after it still are.
	line := strings.Repeat("x", 1000)
	n := textBytesMax/1000 + 1
	r.setPart(t, db.PartBase, strings.Repeat(line+"\n", n))
	_, lines, _ = r.l.whatChanged("ann")
	if len(lines) != 3 || lines[1].Part != "base.md" || len(lines[1].Added) != changedBytesMax/1000 || lines[1].More != n-changedBytesMax/1000 ||
		len(lines[2].Added)+len(lines[2].Removed) != 0 || lines[2].More != 2 {
		t.Fatalf("%d %d %+v", len(lines[1].Added), lines[1].More, lines[2])
	}
	if lines[0].Text != "builds\nreviews\ntests" || lines[1].Text != "" || lines[2].Text != rules {
		t.Fatalf("texts kept: %q %d %q", lines[0].Text, len(lines[1].Text), lines[2].Text)
	}
	// Texts that fill textBytesMax exactly are all kept.
	r.setPart(t, db.PartBase, strings.Repeat("y", textBytesMax-len(lines[0].Text)-len(rules)))
	if _, lines, _ = r.l.whatChanged("ann"); lines[1].Text == "" || lines[2].Text != rules {
		t.Fatalf("an exact fit: %d %q", len(lines[1].Text), lines[2].Text)
	}
	var old map[string]any
	if err := json.Unmarshal([]byte(rec), &old); err != nil {
		t.Fatal(err)
	}
	delete(old, LaunchedTexts)
	delete(old, LaunchedRules)
	b, _ := json.Marshal(old)
	if err := db.SetLaunchFingerprint(r.l.Q, "ann", "fp", string(b)); err != nil {
		t.Fatal(err)
	}
	if got, lines, _ := r.l.whatChanged("ann"); got != "ROLE.md changed by the founder, base.md changed by the founder, team rules changed" || lines != nil {
		t.Fatalf("no texts kept: %q %+v", got, lines)
	}
}

// What a launch finds changed is kept on its session start.
func TestAStartKeepsWhatItsSettingsChanged(t *testing.T) {
	r := newLaunchRig(t)
	rec, _ := r.l.LaunchRecord("ann")
	if err := db.SetLaunchFingerprint(r.l.Q, "ann", "fp", rec); err != nil {
		t.Fatal(err)
	}
	if err := r.l.Roster.SetKeys("ann", []yamlcompat.Pair{{Key: "model", Val: "m2"}}); err != nil {
		t.Fatal(err)
	}
	r.setPart(t, db.PartBase, "a new line")
	if err := r.l.keepWhatItStartsWith(&Site{}, "ann", []string{"--resume", "s1"}, "", "settings_applied", "the daemon"); err != nil {
		t.Fatal(err)
	}
	var changed, lines string
	var resumed int
	if err := r.l.Q.QueryRow("SELECT changed, changed_lines, resumed FROM session_starts WHERE agent = 'ann'").Scan(&changed, &lines, &resumed); err != nil {
		t.Fatal(err)
	}
	if changed != "model changed on disk (m1 -> m2), base.md changed by the founder" || resumed != 1 ||
		lines != `[{"part":"base.md","added":["a new line"],"removed":[],"text":"a new line"}]` {
		t.Fatalf("%q %q %d", changed, lines, resumed)
	}
}

// pastFitRig has five accounts: c with the most headroom of those a person may choose; d with less; walled at its wall, with the most
// headroom of all; paused; own, the one left out.
func pastFitRig(t *testing.T) (*launchRig, *db.AgentState) {
	t.Helper()
	r := newLaunchRig(t)
	hot := AccountStatus{Exists: true, ShortTrusted: func() *int { n := 99; return &n }()}
	r.l.Ports.Accounts = fakeAccounts{true}
	r.l.Ports.CurrentAccount = func(db.Q, string) string { return "paused" }
	r.l.Ports.WaitingFor = func(db.Q, string) bool { return false }
	pct := 10
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true, ShortTrusted: &pct}, by: map[string]AccountStatus{"walled": hot,
		"paused": {Exists: true, ShortTrusted: &pct, Paused: true}}, head: map[string]float64{"c": 0.5, "d": 0.2, "walled": 0.9, "paused": 0.8, "own": 0.95}}
	for _, a := range []string{"c", "d", "walled", "paused", "own"} {
		os.MkdirAll(teamtest.AccountDir(t, r.l.Home, a), 0o755)
	}
	db.SetAgentState(r.l.Q, "ann", states.Idle, 0, "")
	row, _ := db.GetAgentState(r.l.Q, "ann")
	return r, row
}

func TestTheClosestAccountIsTheMostHeadroomAPersonMayChoose(t *testing.T) {
	r, row := pastFitRig(t)
	if got := r.l.ClosestRoom(r.l.Q, "ann", row, map[string]string{"own": ""}); got != "c" {
		t.Fatalf("most headroom past a wall, a paused account and the one left out: %q", got)
	}
	if got := r.l.ClosestRoom(r.l.Q, "ann", row, map[string]string{"own": "", "c": "", "d": ""}); got != "" {
		t.Fatalf("nothing past a wall or a paused account: %q", got)
	}
}

func TestAPinOffersNothingAndRefusesAnotherAccount(t *testing.T) {
	r, row := pastFitRig(t)
	teamtest.Set(t, r.l.Home, "ann", "account", "own")
	if got := r.l.ClosestRoom(r.l.Q, "ann", row, nil); got != "" {
		t.Fatalf("a pinned agent goes nowhere: %q", got)
	}
	if why := r.l.RoomPastFit(r.l.Q, "ann", row, "c"); !strings.Contains(why, "pinned") {
		t.Fatalf("%q", why)
	}
}

func TestAPersonsChoiceStillMeetsTheWallAndThePause(t *testing.T) {
	r, row := pastFitRig(t)
	for acct, ok := range map[string]bool{"c": true, "walled": false, "paused": false, "nowhere": false} {
		if why := r.l.RoomPastFit(r.l.Q, "ann", row, acct); (why == "") != ok {
			t.Errorf("%s: %q", acct, why)
		}
	}
}

func TestAResumeOntoAPausedAccountWaitsAndSaysItIsPaused(t *testing.T) {
	r := newLaunchRig(t)
	r.l.Ports.AccountReading = func(db.Q, string) (string, string, bool) { return "a", "recorded", true }
	r.l.Ports.CurrentAccount = func(db.Q, string) string { return "a" }
	q := r.l.Q
	db.SetAgentState(q, "ann", states.Idle, 0, "")
	db.SetClosed(q, "ann", "s-ann")
	row, _ := db.GetAgentState(q, "ann")
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true, Paused: true}}
	w := r.l.WakeWait(q, "ann", row)
	if w == nil || !w.Paused || DescribeWakeWait(w) != "waiting for a: it is paused; resume it in the app's settings" {
		t.Fatalf("%+v", w)
	}
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}}
	if w := r.l.WakeWait(q, "ann", row); w != nil {
		t.Fatalf("resumed, the account takes it: %+v", w)
	}
}

// An agent's folder is by key, so a launch for no agent row (an unknown name or key) would read a folder sessions may write.
func TestBuildRefusesAnAgentWithNoRow(t *testing.T) {
	r := newLaunchRig(t)
	for _, who := range []string{"nobody", "a99"} {
		_, _, err := r.build(who, nil, false, "main")
		if ref, ok := err.(*Refusal); !ok || !strings.Contains(ref.Msg, "no agent row has it") {
			t.Errorf("%s: %v", who, err)
		}
	}
}

// A daemon started from an agent's session is one whose store refuses the record: the launch is refused before its prompt is taken.
func TestPrepareRefusesASessionItCannotRecord(t *testing.T) {
	r := newLaunchRig(t)
	prompt := filepath.Join(t.TempDir(), "wake.txt")
	if err := os.WriteFile(prompt, []byte("wake"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HANDLINE_AGENT", "a5")
	err := r.prepare("ann", nil, false, prompt, "main", "", "")
	if _, ok := err.(*Refusal); !ok || !strings.Contains(err.Error(), "could not be recorded") {
		t.Fatalf("%v", err)
	}
	if _, err := os.Stat(prompt); err != nil {
		t.Fatal("the prompt file was used up:", err)
	}
}

// A launch on a runner is proven there: the store here, which refuses the record, does not stop it, and the terminal it records is
// the runner's handle as given.
func TestARunnersLaunchIsNotRecordedHere(t *testing.T) {
	r := newLaunchRig(t)
	t.Setenv("HANDLINE_AGENT", "a5") // the store here refuses every record
	site := r.site()
	site.Runner, site.Handle = true, "box:%3"
	if _, err := r.l.Prepare(site, "ann", nil, false, "", "main", "", ""); err != nil {
		t.Fatal(err)
	}
	if h, _ := db.Terminal(r.l.Q, "ann"); h != "box:%3" {
		t.Fatalf("recorded terminal %q", h)
	}
}

func TestBuildAddsTheWorkspacesDeniedToolsAndHoldsTheModeToItsCap(t *testing.T) {
	r := newLaunchRig(t)
	r.l.Ports.CurrentAccount = func(db.Q, string) string { return "main" }
	base, _ := r.l.Fingerprint("ann")
	r.l.Ports.Held = func(string) team.Held {
		return team.Held{Deny: []string{"WebFetch", "Bash(rm:*)"}, ModeMax: "plan", ModeSetBy: "p1"}
	}
	argv, _, err := r.build("ann", nil, false, "main")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(argv, " ")
	if !strings.Contains(got, "--disallowed "+DisallowedTools+",WebFetch,Bash(rm:*)") || !strings.Contains(got, "--permission-mode plan") || strings.Contains(got, "--permission-mode auto") {
		t.Fatalf("%s", got)
	}
	if now, _ := r.l.Fingerprint("ann"); now == base {
		t.Error("a rule that changes what a session starts with leaves its fingerprint as it was")
	}
}

// A session start keeps what the agent was given: its handoff and the daemon's record under it, so the chat's handoff shows both.
func TestAStartKeepsTheHandoffAndTheDaemonsRecord(t *testing.T) {
	r := newLaunchRig(t)
	if err := db.SetHandoff(r.l.Q, "ann", "# handoff · ann\nbranch: x"); err != nil {
		t.Fatal(err)
	}
	if err := r.l.keepWhatItStartsWith(&Site{}, "ann", nil, "", "move", "the daemon"); err != nil {
		t.Fatal(err)
	}
	var text string
	if err := r.l.Q.QueryRow("SELECT text FROM session_starts WHERE agent = 'ann'").Scan(&text); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(text, "# handoff · ann\nbranch: x\n\n# Where you were (the daemon's record)") {
		t.Fatalf("%q", text)
	}
}

// A founder's grant of another agent's memory folder, or of a folder above it, takes that folder out of what the session is hidden
// from, and is named as lifted. A folder no grant holds stays hidden, its Read tool asking the founder; the key folders stay hidden
// and refused under a grant of everything.
func TestAGrantLiftsTheHideOfAnotherAgentsMemoryButNeverOfTheKeyFolders(t *testing.T) {
	r := newLaunchRig(t)
	teamtest.Set(t, r.l.Home, "bob", "vendor", "claude")
	teamtest.Set(t, r.l.Home, "cy", "vendor", "claude")
	drv := fakeDriver{vendor: "claude"}
	memories, err := r.l.HiddenMemories("ann")
	if err != nil || len(memories) != 2 {
		t.Fatalf("%v %v", memories, err)
	}
	bob, cy := memories[0], memories[1]
	keys := r.l.Tokens.HiddenFolders()

	c, err := r.l.Confinement("ann", "", drv, nil)
	if err != nil || !slices.Equal(c.AskRead, memories) || !slices.Equal(c.Hide, append(slices.Clone(memories), keys...)) || c.Lifted != nil {
		t.Fatalf("no grant: %+v, %v", c, err)
	}
	for _, g := range []string{bob, filepath.Dir(bob)} {
		c, _ = r.l.Confinement("ann", "", drv, []string{g, "/unrelated"})
		if slices.Contains(c.Hide, bob) || !slices.Equal(c.AskRead, []string{cy}) || !slices.Equal(c.Lifted, []string{g}) {
			t.Errorf("grant %s: %+v", g, c)
		}
	}
	c, _ = r.l.Confinement("ann", "", drv, []string{"/"})
	if !slices.Equal(c.Hide, keys) || len(c.AskRead) != 0 {
		t.Fatalf("a grant of /: %+v, want only the key folders %v hidden and none asked", c, keys)
	}
}

// The classifier is told of a read grant only when the agent holds one.
func TestTheTrustedLinesNameTheReadGrantOnlyWhenThereIsOne(t *testing.T) {
	r := newLaunchRig(t)
	if lines := strings.Join(r.l.TrustedEnvironment("", nil), "\n"); strings.Contains(lines, "granted") {
		t.Fatalf("no grant, yet: %s", lines)
	}
	lines := strings.Join(r.l.TrustedEnvironment("", []string{"/agents/bob/memory"}), "\n")
	if !strings.Contains(lines, "The founder granted this agent read access to /agents/bob/memory") {
		t.Fatalf("%s", lines)
	}
}

// A kept grant is a settings change: it changes the permissions part, so the session restarts to take it; a once grant does not.
func TestAnAlwaysGrantChangesThePermissionsPartAndAOnceGrantDoesNot(t *testing.T) {
	r := newLaunchRig(t)
	part := func() string {
		parts, err := r.l.FingerprintParts("ann")
		if err != nil {
			t.Fatal(err)
		}
		return parts["permissions"]
	}
	before := part()
	access.Put(r.l.Q, "ann", "/once", "once", 1)
	if part() != before {
		t.Fatal("a once grant changed the permissions part")
	}
	access.Put(r.l.Q, "ann", "/agents", "always", 2)
	if part() == before {
		t.Fatal("an always grant left the permissions part as it was")
	}
}

// An agent's live lessons ride in its next launch prompt, after the team rules and before its handoff; an add does not change the
// fingerprint, so the running session is not restarted for it.
func TestALaunchPromptCarriesTheAgentsLessons(t *testing.T) {
	r := newLaunchRig(t)
	before, err := r.l.Fingerprint("ann")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddLesson(r.l.Q, "ann", "show the screen in both themes", 0, "ann"); err != nil {
		t.Fatal(err)
	}
	gone, _ := db.AddLesson(r.l.Q, "ann", "a removed one", 0, "ann")
	_, _ = db.RemoveLesson(r.l.Q, gone, "p1")
	_ = db.SetHandoff(r.l.Q, "ann", "left off here")
	text, err := r.l.ContextFor("ann")
	if err != nil {
		t.Fatal(err)
	}
	at, handoff := strings.Index(text, "# What you learned (your lessons)\n"), strings.Index(text, "# Where you left off")
	if at < 0 || handoff < at || !strings.Contains(text[at:handoff], "- show the screen in both themes") || strings.Contains(text, "a removed one") {
		t.Fatalf("%s", text)
	}
	if after, _ := r.l.Fingerprint("ann"); after != before {
		t.Error("a lesson changed the fingerprint")
	}
}

// The founder's preferences (lessons kept under their key) ride in the launch prompt of every agent they own, before its own lessons.
func TestEveryAgentsLaunchPromptCarriesItsFoundersPreferences(t *testing.T) {
	r := newLaunchRig(t)
	teamtest.Set(t, r.l.Home, "bo", "vendor", "claude", "model", "m1", "effort", "high")
	if _, err := r.l.Q.Exec("INSERT INTO people(display_name, role, created_at) SELECT 'Ron', 'owner', 0 WHERE NOT EXISTS (SELECT 1 FROM people WHERE role = 'owner')"); err != nil {
		t.Fatal(err)
	}
	founder, err := addr.OwnerOf(r.l.Q, "ann")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddLesson(r.l.Q, founder, "write to the founder in Korean (the founder, comment 7)", 0, "ann"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddLesson(r.l.Q, "ann", "show the screen in both themes", 0, "ann"); err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"ann", "bo"} {
		text, err := r.l.ContextFor(agent)
		if err != nil {
			t.Fatal(err)
		}
		at := strings.Index(text, "# The founder's preferences\n")
		if at < 0 || !strings.Contains(text[at:], "- write to the founder in Korean (the founder, comment 7)") {
			t.Fatalf("%s: no founder's preferences\n%s", agent, text)
		}
		if own := strings.Index(text, "# What you learned"); agent == "ann" && own < at {
			t.Fatalf("ann: the founder's preferences come before its own lessons\n%s", text)
		}
	}
}

// A start names who changed each part: an agent's proposal the founder OK'd is one version with its diff, and the start says the part
// and who; team instructions a newer build wrote are a Handline update.
func TestAStartNamesWhoChangedEachPart(t *testing.T) {
	r := newLaunchRig(t)
	teamtest.Set(t, r.l.Home, "bob", "vendor", "claude")
	r.setPart(t, db.PartRole, "builds")
	rec, _ := r.l.LaunchRecord("ann")
	if err := db.SetLaunchFingerprint(r.l.Q, "ann", "fp", rec); err != nil {
		t.Fatal(err)
	}
	ann, bob := teamtest.Key(t, r.l.Home, "ann"), teamtest.Key(t, r.l.Home, "bob")
	if _, err := db.SetInstruction(r.l.Q, ann, db.PartRole, "builds\nreviews", bob, actor.OwnerKey, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	if err := r.l.keepWhatItStartsWith(&Site{}, "ann", []string{"--resume", "s1"}, "", "settings_applied", "the daemon"); err != nil {
		t.Fatal(err)
	}
	var changed, lines string
	if err := r.l.Q.QueryRow("SELECT changed, changed_lines FROM session_starts WHERE agent = 'ann'").Scan(&changed, &lines); err != nil {
		t.Fatal(err)
	}
	if changed != "ROLE.md changed by bob with the founder's OK" || !strings.Contains(lines, `"added":["reviews"]`) {
		t.Fatalf("%q %q", changed, lines)
	}
	hist, err := db.InstructionsAfter(r.l.Q, ann, db.PartRole, 0)
	if err != nil || len(hist) != 2 || hist[1].Actor != bob || hist[1].Approver != actor.OwnerKey || hist[0].Text != "builds" || hist[1].Text != "builds\nreviews" {
		t.Fatalf("history: %+v %v", hist, err)
	}
}

// Handline's own wording is no change to a running session: the fingerprint and the team part hash the team's facts, never the
// sentences a build writes around them; a peer joining the team drifts the session and is named.
func TestHandlinesWordingIsNoChangeButATeamChangeIs(t *testing.T) {
	r := newLaunchRig(t)
	ann := teamtest.Key(t, r.l.Home, "ann")
	members := []string{ann}
	r.l.Ports.Routing = coreTeam(&members)
	static, err := r.l.staticPreamble(ann)
	if err != nil || !strings.Contains(static, team.Protocol("ends")) {
		t.Fatalf("the preamble lacks the team section: %v", err)
	}
	for _, sentence := range []string{team.Protocol("ends"), "Stay inside", "# Your team"} {
		if strings.Contains(r.l.preambleMaterial(ann), sentence) {
			t.Fatalf("Handline's wording %q is in what the fingerprint hashes", sentence)
		}
	}
	fp, err := r.l.Fingerprint(ann)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := r.l.LaunchRecord(ann)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetLaunchFingerprint(r.l.Q, ann, fp, rec); err != nil {
		t.Fatal(err)
	}
	row, _ := db.GetAgentState(r.l.Q, ann)
	if r.l.Drifted(ann, row) || r.l.WhatChanged(ann) != "" {
		t.Fatalf("drifted from itself: %q", r.l.WhatChanged(ann))
	}
	members = append(members, teamtest.Key(t, r.l.Home, "bob"))
	if !r.l.Drifted(ann, row) || r.l.WhatChanged(ann) != "team instructions changed (team.yaml or the agent list)" {
		t.Fatalf("a new peer is no change: %q", r.l.WhatChanged(ann))
	}
}

// The fingerprint of a fixed agent is a fixed value: what it hashes changes only on purpose, since a new value restarts every running
// session at its next idle on the build that brings it. A build that only rewords Handline's sentences must leave it alone.
func TestFingerprintIsPinned(t *testing.T) {
	r := newLaunchRig(t)
	ann := teamtest.Key(t, r.l.Home, "ann")
	bob := teamtest.Key(t, r.l.Home, "bob")
	r.l.Ports.Routing = func() *team.Team {
		return &team.Team{Agents: map[string]team.Member{ann: {Workspace: "core"}, bob: {Workspace: "core"}},
			Teams: map[string][]string{"core": {ann, bob}}}
	}
	fp, err := r.l.Fingerprint(ann)
	if err != nil {
		t.Fatal(err)
	}
	if fp != "74b151da8541c48b" {
		t.Fatalf("the fingerprint is %s: if its inputs changed on purpose, pin the new value; if only Handline's wording changed, it must not", fp)
	}
}

// A launch recorded under another way of hashing the team section (before T9237) is held to its text: the same text is no change and
// restarts nothing, while a peer joining changes the text, and the line that names the peers is what the restart shows.
func TestARehashedPartWithTheSameTextIsNoChange(t *testing.T) {
	r := newLaunchRig(t)
	ann := teamtest.Key(t, r.l.Home, "ann")
	members := []string{ann, "cat"}
	r.l.Ports.Routing = coreTeam(&members)
	fp, _ := r.l.Fingerprint(ann)
	rec, err := r.l.LaunchRecord(ann)
	if err != nil {
		t.Fatal(err)
	}
	var parts map[string]json.RawMessage
	json.Unmarshal([]byte(rec), &parts)
	parts["team instructions"], _ = json.Marshal("0ldsha1e") // hashed the old way: the whole section's text
	b, _ := json.Marshal(parts)
	if err := db.SetLaunchFingerprint(r.l.Q, ann, fp, string(b)); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.l.PartsSince(ann); got["team instructions"] != "0ldsha1e" {
		t.Fatalf("the same text reads as changed: %q", got["team instructions"])
	}
	if w := r.l.WhatChanged(ann); w != "" {
		t.Fatalf("named a change: %q", w)
	}
	members = append(members, "dan")
	got, _ := r.l.PartsSince(ann)
	words, lines, _ := r.l.whatChanged(ann)
	if got["team instructions"] == "0ldsha1e" || !strings.HasPrefix(words, "team instructions changed") || len(lines) != 1 ||
		len(lines[0].Added) != 1 || !strings.Contains(lines[0].Added[0], "`dan`") {
		t.Fatalf("a new peer: %q %+v", words, lines)
	}
}

// A record whose texts are null still reads its rules.
func TestLaunchedTextsOfANullRecord(t *testing.T) {
	old := map[string]json.RawMessage{LaunchedTexts: json.RawMessage("null"), LaunchedRules: json.RawMessage(`"r"`)}
	if got := launchedTexts(old); got[rulesPart] != "r" {
		t.Fatalf("%v", got)
	}
}

// coreTeam is a routing of one workspace, core, whose members are *members as they are when it is read.
func coreTeam(members *[]string) func() *team.Team {
	return func() *team.Team {
		tm := &team.Team{Agents: map[string]team.Member{}, Teams: map[string][]string{"core": *members}}
		for _, m := range *members {
			tm.Agents[m] = team.Member{Workspace: "core"}
		}
		return tm
	}
}
