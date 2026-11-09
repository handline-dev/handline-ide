package lifecycle

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

func (d fakeDriver) Name() string                     { return d.vendor }
func (fakeDriver) IsResume(extra []string) bool       { return len(extra) > 0 && extra[0] == "--resume" }
func (fakeDriver) Screen() term.Driver                { return term.Blind{} }
func (fakeDriver) Binary() string                     { return "/bin/vendor" }
func (fakeDriver) Hooks() cli.Hooks                   { return nil }
func (fakeDriver) Deliver(cli.DeliverEnv) cli.Port    { return nil }
func (fakeDriver) AutoModeArgv() []string             { return []string{"--permission-mode", "auto"} }
func (fakeDriver) PermissionModeArgv(string) []string { return nil }
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
	cap     int
	unpaced int // 0: cap
	pace    *usage.Pace
	others  int
	exclude *string        // the agent ActiveOn was asked to leave out
	active  map[string]int // agents mid-turn by account, over others
	head    map[string]float64
	share   map[string]float64 // FiveHourShare by account; unlisted is unknown
	urgent  map[string]float64 // Urgency by account; unlisted is unknown
	reasons []usage.Reason     // PickReasons
}

func (u fakeUsage) CapOf(string) int { return u.cap }
func (u fakeUsage) CapUnpaced(string) int {
	if u.unpaced != 0 {
		return u.unpaced
	}
	return u.cap
}
func (u fakeUsage) Pace(string) *usage.Pace { return u.pace }
func (u fakeUsage) Urgency(acct string) (float64, bool) {
	v, ok := u.urgent[acct]
	return v, ok
}
func (u fakeUsage) PaceShare(_ db.Q, acct, _ string) (float64, bool) {
	share, ok := u.share[acct]
	return share, ok
}
func (u fakeUsage) Headroom(acct string) (float64, bool) {
	h, ok := u.head[acct]
	return h, ok
}
func (u fakeUsage) ActiveOn(_ db.Q, acct, agent string) int {
	if u.exclude != nil {
		*u.exclude = agent
	}
	return u.others + u.active[acct]
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

// Admit hands verdict.Admit the facts the fixtures stand for: a trusted fill at the block is the wall, a five-hour share under 1 is no room.
func (u fakeUsage) Admit(acct, _, _ string) verdict.Admission {
	in := verdict.Account{Assignable: true}
	if st := u.Read(acct); st.ShortTrusted != nil && *st.ShortTrusted >= u.ShortBlock() {
		in.Wall = fmt.Sprintf("5h usage at the wall (%d%%)", *st.ShortTrusted)
	}
	if share, ok := u.share[acct]; ok && share < 1 {
		in.NoRoomFit = "short of its reset"
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
	roster := team.NewRoster(home)
	if err := os.MkdirAll(filepath.Dir(roster.AgentFile("ann")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(roster.AgentFile("ann"), []byte("vendor: claude\nmodel: m1\neffort: high\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.l = &Launcher{Q: openDB(t), Home: home, Roster: roster, Tokens: sessiontoken.New(home), Ports: LaunchPorts{
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
		if err := os.MkdirAll(filepath.Join(paths.AccountsDir(r.l.Home), n), 0o755); err != nil {
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

func TestAResumeWaitsForASlotOnItsOwnAccountWhenTheOthersAndOlderWaitersFillIt(t *testing.T) {
	r := newLaunchRig(t)
	var left string
	r.l.Ports.AccountReading = func(db.Q, string) (string, string, bool) { return "a", "recorded", true }
	r.l.Ports.CurrentAccount = func(_ db.Q, agent string) string {
		if agent == "elsewhere" {
			return "b"
		}
		return "a"
	}
	r.l.Ports.WaitingFor = func(_ db.Q, agent string) bool { return agent != "quiet" }
	q := r.l.Q
	db.SetAgentState(q, "ann", states.Idle, 0, "")
	db.SetClosed(q, "ann", "s-ann")
	older := func(agent, state string, failures int) {
		db.SetAgentState(q, agent, state, 0, "")
		db.SetClosed(q, agent, "s-"+agent)
		for i := 0; i < failures; i++ {
			db.BumpCounter(q, agent, "wake_failures", 1)
		}
		q.Exec("UPDATE agent_state SET closed_at = 1 WHERE agent = ?", agent)
	}
	q.Exec("UPDATE agent_state SET closed_at = 10 WHERE agent = 'ann'")
	row, _ := db.GetAgentState(q, "ann")
	wait := func(cap, others int) *WakeWait {
		r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}, cap: cap, others: others, exclude: &left}
		return r.l.WakeWait(q, "ann", row)
	}
	if w := wait(2, 2); w == nil || w.Full != 2 || w.Busy != 2 || w.Older != 0 || w.Account != "a" {
		t.Fatalf("a full account (busy fill the cap): %+v", w)
	}
	if left != "ann" {
		t.Fatalf("the agent placed is left out of the count: %q", left)
	}
	if wait(2, 1) != nil {
		t.Fatal("a free slot is no wait")
	}
	older("elsewhere", states.Idle, 0)
	if wait(2, 1) != nil {
		t.Fatal("an older waiter for another account takes no slot here")
	}
	older("bob", states.Idle, 0)
	older("quiet", states.Idle, 0)
	older("stuck", states.Stalled, 0)
	older("failing", states.Idle, states.MaxFailedAttempts)
	w := wait(2, 1)
	if w == nil || w.Full != 2 || w.Busy != 1 || w.Older != 1 {
		t.Fatalf("one older waiter takes the free slot (not stalled, not exhausted, not unwanted): %+v", w)
	}
	if got := DescribeWakeWait(w); got != "waiting for a slot on a (1 of 2 mid-turn, 1 older waiting go first)" {
		t.Fatal(got)
	}
	if got := DescribeWakeWait(&WakeWait{Account: "a", Slot: true, Full: 2, Busy: 2}); got != "waiting for a slot on a (2 of 2 mid-turn)" {
		t.Fatal(got)
	}
	if wait(3, 1) != nil {
		t.Fatal("room for the agent and the older waiter")
	}
	pace := &usage.Pace{State: usage.PaceOver, Line: 40, Used: 60}
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}, cap: 0, unpaced: 3, others: 1, pace: pace}
	if w := r.l.WakeWait(q, "ann", row); w == nil || w.Pace != pace {
		t.Fatalf("a slot only the pace holds says so: %+v", w)
	} else if got := DescribeWakeWait(w); got != "waiting for a's weekly pace: 60% used, 20 points ahead of its line (40%)" {
		t.Fatal(got)
	}
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}, cap: 0, unpaced: 3, others: 2, pace: pace}
	if w := r.l.WakeWait(q, "ann", row); w == nil || w.Pace != nil {
		t.Fatalf("a slot the others fill is not the pace's: %+v", w)
	}
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}, cap: 0, unpaced: 3, others: 1, pace: pace}
	r.l.PassPace("ann")
	if w := r.l.WakeWait(q, "ann", row); w != nil {
		t.Fatalf("a person's force start lets it past the pace: %+v", w)
	}
	r.l.Now = func() time.Time { return time.Now().Add(PacePassFor + time.Second) }
	if w := r.l.WakeWait(q, "ann", row); w == nil || w.Pace == nil {
		t.Fatalf("the pass is for one start, not for good: %+v", w)
	}
	r.l.Now = nil
	// An account that would run out before it resets has a cap of none: nothing is free however quiet it is, and the wait says why.
	w = wait(0, 0)
	if w == nil || !w.Slot || w.Full != 0 {
		t.Fatalf("a cap of none is a full account: %+v", w)
	}
	if got := DescribeWakeWait(w); got != "waiting for a: it would run out before it resets and takes no new agent" {
		t.Fatal(got)
	}
}

func TestAColdWakeMovesToTheSlowestPacedAccountWithRoomNotItsOwnNorOneWithOlderWaiters(t *testing.T) {
	r := newLaunchRig(t)
	pct := 10
	ok := AccountStatus{Exists: true, ShortTrusted: &pct}
	hot := AccountStatus{Exists: true, ShortTrusted: func() *int { n := 99; return &n }()}
	r.l.Ports.Accounts = fakeAccounts{true}
	r.l.Ports.CurrentAccount = func(_ db.Q, agent string) string {
		if agent == "waiting-on-b" {
			return "b"
		}
		return "a"
	}
	r.l.Ports.WaitingFor = func(db.Q, string) bool { return true }
	r.l.Ports.Usage = fakeUsage{st: ok, cap: 1, by: map[string]AccountStatus{"walled": hot},
		head: map[string]float64{"a": 0.99, "b": 0.9, "c": 0.5, "ahead": 0.02, "walled": 0.95}}
	home, q := r.l.Home, r.l.Q
	for _, a := range []string{"a", "b", "c", "ahead", "walled"} {
		os.MkdirAll(filepath.Join(paths.AccountsDir(home), a), 0o755)
	}
	db.SetAgentState(q, "ann", states.Idle, 0, "")
	db.SetClosed(q, "ann", "s-ann")
	db.SetAgentState(q, "waiting-on-b", states.Idle, 0, "")
	db.SetClosed(q, "waiting-on-b", "s-b")
	q.Exec("UPDATE agent_state SET closed_at = 10 WHERE agent = 'ann'")
	q.Exec("UPDATE agent_state SET closed_at = 1 WHERE agent = 'waiting-on-b'")
	row, _ := db.GetAgentState(q, "ann")
	// "a" has the most headroom but is ann's own, "b" the next but an older closed agent is waiting for its one slot, "walled" does not fit.
	if got := r.l.SlowestPaced(q, "ann", row, "a"); got != "c" {
		t.Fatalf("the furthest behind among the other accounts with a free slot: %q", got)
	}
	r.l.Ports.Usage = fakeUsage{st: ok, cap: 1, by: map[string]AccountStatus{"walled": hot}, head: map[string]float64{"a": 0.99, "b": 0.9}}
	for _, a := range []string{"c", "ahead", "walled"} {
		os.RemoveAll(filepath.Join(paths.AccountsDir(home), a))
	}
	if got := r.l.SlowestPaced(q, "ann", row, "a"); got != "" {
		t.Fatalf("an account an older waiter holds the slot of is not room, and the agent's own is left out: %q", got)
	}
}

func TestAPersonsStartGoesToTheAccountItWasLastOnElseTheMostHeadroomAndQueuesBehindNobody(t *testing.T) {
	r := newLaunchRig(t)
	pct := 10
	hot := AccountStatus{Exists: true, ShortTrusted: func() *int { n := 99; return &n }()}
	r.l.Ports.Accounts = fakeAccounts{true}
	r.l.Ports.CurrentAccount = func(_ db.Q, agent string) string {
		if agent == "waiting-on-b" {
			return "b"
		}
		return "a"
	}
	r.l.Ports.WaitingFor = func(db.Q, string) bool { return true }
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true, ShortTrusted: &pct}, cap: 1, by: map[string]AccountStatus{"walled": hot},
		head: map[string]float64{"a": 0.99, "b": 0.9, "c": 0.5, "walled": 0.95}}
	home, q := r.l.Home, r.l.Q
	for _, a := range []string{"a", "b", "c", "walled"} {
		os.MkdirAll(filepath.Join(paths.AccountsDir(home), a), 0o755)
	}
	db.SetAgentState(q, "ann", states.Idle, 0, "")
	db.SetClosed(q, "ann", "s-ann")
	db.SetAgentState(q, "waiting-on-b", states.Idle, 0, "")
	db.SetClosed(q, "waiting-on-b", "s-b")
	q.Exec("UPDATE agent_state SET closed_at = 10 WHERE agent = 'ann'")
	q.Exec("UPDATE agent_state SET closed_at = 1 WHERE agent = 'waiting-on-b'")
	row, _ := db.GetAgentState(q, "ann")
	if got, why := r.l.RoomNow(q, "ann", row, "a"); got != "b" {
		t.Fatalf("the most headroom, the older waiter on b not ahead of a person: %q %s", got, why)
	}
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true, ShortTrusted: &pct}, cap: 0}
	if got, why := r.l.RoomNow(q, "ann", row, "a"); got != "" || !strings.HasPrefix(why, "no account can take one more agent") {
		t.Fatalf("nowhere has room: %q %s", got, why)
	}
	// Only the pace holds every slot: the daemon's move waits, a person's start goes where the most headroom is.
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true, ShortTrusted: &pct}, cap: 0, unpaced: 2, head: map[string]float64{"a": 0.1, "b": 0.5}}
	if got, _ := r.l.RoomElsewhere(q, "ann", row, map[string]string{"a": ""}); got != "" {
		t.Fatalf("the pace holds the daemon's move: %q", got)
	}
	if got, why := r.l.RoomNow(q, "ann", row, "a"); got != "b" {
		t.Fatalf("a person's start is not held by the pace: %q %s", got, why)
	}
}

func TestAnOpenAgentsMoveDoesNotTakeTheSlotAnOlderClosedWaiterHolds(t *testing.T) {
	r := newLaunchRig(t)
	pct := 10
	r.l.Ports.Accounts = fakeAccounts{true}
	r.l.Ports.CurrentAccount = func(_ db.Q, agent string) string {
		if agent == "waiting-on-b" {
			return "b"
		}
		return "a"
	}
	r.l.Ports.WaitingFor = func(db.Q, string) bool { return true }
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true, ShortTrusted: &pct}, cap: 1, head: map[string]float64{"a": 0.9, "b": 0.9}}
	for _, a := range []string{"a", "b"} {
		os.MkdirAll(filepath.Join(paths.AccountsDir(r.l.Home), a), 0o755)
	}
	q := r.l.Q
	db.SetAgentState(q, "ann", states.Idle, 0, "") // open: never closed
	db.SetAgentState(q, "waiting-on-b", states.Idle, 0, "")
	db.SetClosed(q, "waiting-on-b", "s-b")
	row, _ := db.GetAgentState(q, "ann")
	if got, why := r.l.RoomElsewhere(q, "ann", row, map[string]string{"a": "its own"}); got != "" {
		t.Fatalf("took the slot a closed agent waits for: %q %s", got, why)
	}
	if why := r.l.RoomOn(q, "ann", row, "b"); why == "" {
		t.Fatal("a forced move to b was allowed past the waiter")
	}
}

func TestAnAgentIsToldItsTeamFromTeamYAML(t *testing.T) {
	r := newLaunchRig(t)
	v, err := yamlcompat.ParseOrdered(`
teams:
  manager: [manager, marina, keeper]
  lone: [scout]
agents:
  manager: {}
  marina: {}
  keeper: {}
  scout: {}
edges:
  - { from: "human", to: "*" }
  - { from: "*", to: "human" }
`)
	if err != nil {
		t.Fatal(err)
	}
	tm, err := team.NewTeam(v.(*yamlcompat.OMap))
	if err != nil {
		t.Fatal(err)
	}
	home := r.l.Home
	r.l.Ports.Routing = func() *team.Team { return tm }
	r.l.Ports.TeamRules = func(agent string) []string {
		if err := team.SeedRules(home, tm); err != nil {
			t.Fatal(err)
		}
		return team.Rules(home, agent, tm)
	}
	for _, n := range []string{"manager", "marina", "keeper", "scout"} {
		os.MkdirAll(filepath.Dir(r.l.Roster.AgentFile(n)), 0o755)
		os.WriteFile(r.l.Roster.AgentFile(n), []byte("vendor: claude\n"), 0o644)
	}
	reportLine := func(text string) string {
		for _, ln := range strings.Split(text, "\n") {
			if strings.HasPrefix(ln, "You report to") {
				return ln
			}
		}
		return ""
	}
	context := func(agent string) string {
		text, err := r.l.ContextFor(agent)
		if err != nil {
			t.Fatal(err)
		}
		return text
	}
	culture := []string{"Keep messages short", "no progress or FYI messages", "Never schedule your own wakeup", "You do not implement",
		"Check main before you assign", "founder's own words", "Finding the bottleneck is your job"}
	// A team that empties its rules file is not seeded again: it gets only the facts.
	context("scout")
	if err := os.Remove(filepath.Join(home, "shared", "team", "lone.md")); err != nil {
		t.Fatal(err)
	}
	mgr, dev, scout := context("manager"), context("marina"), context("scout")
	rules := "# Rules for team manager\n" + strings.TrimSpace(team.DefaultRules)
	for who, text := range map[string]string{"manager": mgr, "marina": dev} {
		if !strings.Contains(text, rules) {
			t.Errorf("%s: the seeded team file is not in the prompt:\n%s", who, text)
		}
	}
	for _, w := range culture {
		if strings.Contains(r.l.yourTeam("manager"), w) || strings.Contains(scout, w) {
			t.Errorf("%q is written by code, not read from the team file:\n%s", w, scout)
		}
	}
	if !strings.Contains(mgr, "Your reports: `keeper`, `marina`.") || !strings.Contains(mgr, "--blocked-by T<n>") {
		t.Errorf("a supervisor is not told its reports and how to give them work:\n%s", mgr)
	}
	if strings.Contains(dev, "Your reports") {
		t.Errorf("a report is told it has reports:\n%s", dev)
	}
	for who, text := range map[string]string{"manager": mgr, "marina": dev, "scout": scout} {
		line := reportLine(text)
		bossIsHuman := strings.HasPrefix(line, "You report to `human`")
		if !strings.Contains(line, "`hl task done T<n>") || !strings.Contains(line, "that is your report") {
			t.Errorf("%s: the report-to line does not say task done is the report: %s", who, line)
		}
		if strings.Contains(line, "a task for it: `hl task new --assign") == bossIsHuman || strings.Contains(line, "hl say") {
			t.Errorf("%s: an agent with a boss must be told a block is a task for it, and no say: %s", who, line)
		}
		if !strings.Contains(text, team.Protocol("founder")) {
			t.Errorf("%s: the founder line is missing:\n%s", who, text)
		}
		if !strings.Contains(text, team.Protocol("land")) || !strings.Contains(team.Protocol("land"), "gh pr create") {
			t.Errorf("%s: the landing line (the repo's rules, else a pull request, else commit and stop) is missing:\n%s", who, text)
		}
	}
	if !strings.HasPrefix(reportLine(dev), "You report to `manager`") || !strings.HasPrefix(reportLine(scout), "You report to `human`") {
		t.Errorf("wrong boss:\n%s\n---\n%s", dev, scout)
	}
	// The founder's words typed at an agent: it is told to answer them with hl say, and the others are not told of typing.
	r.l.FounderTyped = func(a string) bool { return a == "marina" }
	if typed, other := r.l.yourTeam("marina"), r.l.yourTeam("manager"); !strings.Contains(typed, "answer with `hl say` too; your terminal text is not shown") ||
		!strings.Contains(typed, "what you write in your terminal reaches no one") || strings.Contains(other, "type straight into your terminal") {
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
	for repo, want := range map[string]bool{"/ws/handline": true, "/ws/harbor-app": false} {
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
	old, _ := team.AccountConfigDir(r.l.Home, "old")
	os.MkdirAll(filepath.Join(old, "projects", "-wt"), 0o755)
	os.WriteFile(filepath.Join(old, "projects", "-wt", "s1.jsonl"), []byte("{}\n"), 0o600)
	if _, _, err := r.build("ann", []string{"--resume", "s1"}, false, ""); err != nil {
		t.Fatal(err)
	}
	main, _ := team.AccountConfigDir(r.l.Home, "main")
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

func TestAStartIsHeldOnlyWhenNoAccountLastsToItsFiveHourReset(t *testing.T) {
	r := newLaunchRig(t)
	for _, a := range []string{"a", "b"} {
		if err := os.MkdirAll(filepath.Join(r.l.Home, "accounts", a), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	r.l.Ports.AccountReading = func(db.Q, string) (string, string, bool) { return "a", "recorded", true }
	r.l.Ports.CurrentAccount = func(db.Q, string) string { return "a" }
	r.l.Now = func() time.Time { return time.Unix(1000, 0) }
	q := r.l.Q
	db.SetAgentState(q, "ann", states.Idle, 0, "")
	db.SetClosed(q, "ann", "s-ann")
	row, _ := db.GetAgentState(q, "ann")
	resets := map[string]AccountStatus{"a": {Exists: true, ShortReset: 5000}, "b": {Exists: true, ShortReset: 3000}}
	for _, c := range []struct {
		name  string
		share map[string]float64
		held  bool
	}{
		{"every account short of its reset", map[string]float64{"a": 0.5, "b": 0.9}, true},
		{"the other account lasts", map[string]float64{"a": 0.5, "b": 1.2}, false},
		{"its own account lasts", map[string]float64{"a": 1, "b": 0.5}, false},
		{"nothing is known", nil, false},
	} {
		r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}, by: resets, cap: 2, share: c.share}
		for _, resume := range []bool{true, false} {
			row.ResumeSession.String = map[bool]string{true: "s-ann"}[resume]
			w := r.l.WakeWait(q, "ann", row)
			// A resume is held on its own account's share even when the other lasts (the wake then moves it); a start only when none does.
			want := c.held || (resume && c.share["a"] != 0 && c.share["a"] < 1)
			if (w != nil && w.FiveHour) != want {
				t.Errorf("%s, resume %v: %+v, held want %v", c.name, resume, w, want)
			}
			if w != nil && w.FiveHour && w.Reset != 3000 {
				t.Errorf("%s: until %d, want the earliest reset among the accounts, 3000", c.name, w.Reset)
			}
		}
	}
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}, by: resets, cap: 2, share: map[string]float64{"a": 0.5, "b": 0.9}}
	r.l.PassPace("ann")
	row.ResumeSession.String = ""
	if w := r.l.WakeWait(q, "ann", row); w != nil {
		t.Errorf("a start let past the hold is not held: %+v", w)
	}
	if got := r.l.FindRoom("ann"); got == "" {
		t.Error("and it still has an account to go to")
	}
	if got := DescribeWakeWait(&WakeWait{FiveHour: true, Reset: 3000}); !strings.HasPrefix(got, "Held so Claude's 5-hour limit lasts until it starts over at ") || !strings.HasSuffix(got, ". This starts then.") {
		t.Errorf("the card says what is held and when: %q", got)
	}
}

func TestAMoveGoesWhereTheWeekRunsOutFirstAndAColdRepickNeedsAClearGain(t *testing.T) {
	r := newLaunchRig(t)
	for _, a := range []string{"a", "b", "c"} {
		if err := os.MkdirAll(filepath.Join(r.l.Home, "accounts", a), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	q := r.l.Q
	db.SetAgentState(q, "ann", states.Idle, 0, "")
	db.SetClosed(q, "ann", "s-ann")
	row, _ := db.GetAgentState(q, "ann")
	// a has the most headroom against its pace line but its week lasts longest; c's week runs out first.
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}, cap: 2, head: map[string]float64{"a": 0.3, "b": 0.1, "c": 0.05},
		urgent: map[string]float64{"a": 0.8, "b": 1.5, "c": 2.4}}
	if got, why := r.l.RoomElsewhere(q, "ann", row, map[string]string{"x": "not here"}); got != "c" {
		t.Errorf("the account whose unused week runs out first: %q %q", got, why)
	}
	if got, _ := r.l.RoomElsewhere(q, "ann", row, map[string]string{"c": "full"}); got != "b" {
		t.Errorf("then the next: %q", got)
	}
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}, cap: 2, head: map[string]float64{"a": 0.3, "b": 0.1}}
	if got, _ := r.l.RoomElsewhere(q, "ann", row, nil); got != "a" {
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
	if got, err := r.l.whatChanged("ann"); got != "" || err != nil {
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
	if got, err := r.l.whatChanged("ann"); got != "" || err != nil {
		t.Fatalf("nothing changed: %q %v", got, err)
	}
	set := func(model, effort string) {
		t.Helper()
		if err := r.l.Roster.SetKeys("ann", []yamlcompat.Pair{{Key: "model", Val: model}, {Key: "effort", Val: effort}}); err != nil {
			t.Fatal(err)
		}
	}
	set("claude-sonnet-5-5", "low")
	os.WriteFile(filepath.Join(paths.AgentDir(r.l.Home, "ann"), "ROLE.md"), []byte("a new role"), 0o644)
	if got, err := r.l.whatChanged("ann"); got != "model changed (m1 -> sonnet-5-5), effort changed (high -> low), ROLE.md changed" || err != nil {
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
	if got, _ := r.l.whatChanged("ann"); got != "ROLE.md changed" {
		t.Fatalf("back to the old model and effort, only the role differs: %q", got)
	}
	set("m2", "high")
	if got, _ := r.l.whatChanged("ann"); got != "model changed, ROLE.md changed" {
		t.Fatalf("%q", got)
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
	if err := r.l.keepWhatItStartsWith(&Site{}, "ann", []string{"--resume", "s1"}, "", "settings_applied", "the daemon"); err != nil {
		t.Fatal(err)
	}
	var changed string
	var resumed int
	if err := r.l.Q.QueryRow("SELECT changed, resumed FROM session_starts WHERE agent = 'ann'").Scan(&changed, &resumed); err != nil {
		t.Fatal(err)
	}
	if changed != "model changed (m1 -> m2)" || resumed != 1 {
		t.Fatalf("%q %d", changed, resumed)
	}
}

// pastFitRig has five accounts: c short only of the room test with the most headroom of those; d short too with less; walled at its wall, short
// with the most headroom of all; full with no slot; own, the one left out.
func pastFitRig(t *testing.T) (*launchRig, *db.AgentState) {
	t.Helper()
	r := newLaunchRig(t)
	hot := AccountStatus{Exists: true, ShortTrusted: func() *int { n := 99; return &n }()}
	r.l.Ports.Accounts = fakeAccounts{true}
	r.l.Ports.CurrentAccount = func(db.Q, string) string { return "full" }
	r.l.Ports.WaitingFor = func(db.Q, string) bool { return false }
	pct := 10
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true, ShortTrusted: &pct}, cap: 1, by: map[string]AccountStatus{"walled": hot},
		head:  map[string]float64{"c": 0.5, "d": 0.2, "walled": 0.9, "full": 0.8, "own": 0.95},
		share: map[string]float64{"c": 0.5, "d": 0.5, "walled": 0.5, "own": 0.5}, active: map[string]int{"full": 1}}
	for _, a := range []string{"c", "d", "walled", "full", "own"} {
		os.MkdirAll(filepath.Join(paths.AccountsDir(r.l.Home), a), 0o755)
	}
	db.SetAgentState(r.l.Q, "ann", states.Idle, 0, "")
	row, _ := db.GetAgentState(r.l.Q, "ann")
	return r, row
}

func TestTheClosestAccountIsShortOnlyOfTheRoomTest(t *testing.T) {
	r, row := pastFitRig(t)
	if got := r.l.ClosestRoom(r.l.Q, "ann", row, map[string]string{"own": ""}); got != "c" {
		t.Fatalf("most headroom past a wall, a full account and the one left out: %q", got)
	}
	if got := r.l.ClosestRoom(r.l.Q, "ann", row, map[string]string{"own": "", "c": "", "d": ""}); got != "" {
		t.Fatalf("nothing past a wall or a full account: %q", got)
	}
}

func TestAPinOffersNothingAndRefusesAnotherAccount(t *testing.T) {
	r, row := pastFitRig(t)
	yaml := "vendor: claude\nmodel: m1\neffort: high\naccount: own\n"
	if err := os.WriteFile(r.l.Roster.AgentFile("ann"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := r.l.ClosestRoom(r.l.Q, "ann", row, nil); got != "" {
		t.Fatalf("a pinned agent goes nowhere: %q", got)
	}
	if why := r.l.RoomPastFit(r.l.Q, "ann", row, "c"); !strings.Contains(why, "pinned") {
		t.Fatalf("%q", why)
	}
}

func TestAPersonsChoicePastTheFitStillMeetsTheWallAndTheSlot(t *testing.T) {
	r, row := pastFitRig(t)
	for acct, ok := range map[string]bool{"c": true, "walled": false, "full": false, "nowhere": false} {
		if why := r.l.RoomPastFit(r.l.Q, "ann", row, acct); (why == "") != ok {
			t.Errorf("%s: %q", acct, why)
		}
	}
}
