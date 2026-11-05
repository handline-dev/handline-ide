package claudecode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/agentctl/agent"
)

// fixture is tests/fixtures/claude_driver.json: what the Python Claude driver writes (tests/support.py claude_driver_cases).
type fixture struct {
	Wanted []string `json:"wanted"`
	Launch []struct {
		Name        string   `json:"name"`
		Exe         *string  `json:"exe"`
		Model       *string  `json:"model"`
		Effort      *string  `json:"effort"`
		Disallowed  *string  `json:"disallowed"`
		Preamble    *string  `json:"preamble"`
		MemoryDir   *string  `json:"memory_dir"`
		MCP         []string `json:"mcp"`
		Hide        []string `json:"hide"`
		NoWrite     []string `json:"no_write"`
		Trusted     []string `json:"trusted"`
		MCPApproved []string `json:"mcp_approved"`
		ExtraDirs   []string `json:"extra_dirs"`
		AllowWrite  []string `json:"allow_write"`
		AllowDomain []string `json:"allow_domains"`
		CacheDir    string   `json:"cache_dir"`
		Weaker      bool     `json:"weaker_network"`
		Excluded    []string `json:"excluded"`
		Chrome      bool     `json:"chrome"`
		Sandbox     *bool    `json:"sandbox"`
		Argv        []string `json:"argv"`
	} `json:"launch"`
	Settings []struct {
		Name           string  `json:"name"`
		Before         *string `json:"before"`
		After          *string `json:"after"`
		EnsureRefused  bool    `json:"ensure_refused"`
		Missing        bool    `json:"missing"`
		StatusLineKept bool    `json:"statusline_kept"`
	} `json:"settings"`
	IsResume      map[string]bool `json:"is_resume"`
	TranscriptDir string          `json:"transcript_dir"`
}

func load(t *testing.T) fixture {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "tests", "fixtures", "claude_driver.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func str(p *string, def string) string {
	if p == nil {
		return def
	}
	return *p
}

func TestLaunchArgvIsWhatPythonWrites(t *testing.T) {
	f := load(t)
	if len(f.Launch) < 8 {
		t.Fatalf("only %d cases", len(f.Launch))
	}
	for _, c := range f.Launch {
		o := agent.LaunchArgs{Model: str(c.Model, ""), Effort: str(c.Effort, "high"), Disallowed: str(c.Disallowed, "Task"),
			MCPArgs: c.MCP, Preamble: str(c.Preamble, ""), Hide: c.Hide, NoWrite: c.NoWrite, MemoryDir: str(c.MemoryDir, ""),
			Trusted: c.Trusted, Chrome: c.Chrome, SandboxOff: c.Sandbox != nil && !*c.Sandbox, MCPApproved: c.MCPApproved,
			ExtraDirs: c.ExtraDirs, AllowWrite: c.AllowWrite, AllowDomains: c.AllowDomain, CacheDir: c.CacheDir,
			WeakerNetwork: c.Weaker, Excluded: c.Excluded}
		if got := (Driver{}).LaunchArgv(str(c.Exe, "/bin/claude"), o); !reflect.DeepEqual(got, c.Argv) {
			t.Errorf("%s:\n got  %q\n want %q", c.Name, got, c.Argv)
		}
	}
}

func TestTheSmallMethodsAreWhatPythonSays(t *testing.T) {
	f := load(t)
	want := slices.Clone(Wanted)
	slices.Sort(want)
	if !reflect.DeepEqual(want, f.Wanted) {
		t.Errorf("wanted events %v, Python's %v", want, f.Wanted)
	}
	for line, resume := range f.IsResume {
		if got := (Driver{}).IsResume(strings.Fields(line)); got != resume {
			t.Errorf("IsResume(%q) = %v", line, got)
		}
	}
	if got, _ := (Driver{}).TranscriptDir("/c", "/Users/user/my repo.v2"); got != f.TranscriptDir {
		t.Errorf("TranscriptDir %q, Python's %q", got, f.TranscriptDir)
	}
}

func TestKeepingTheSharedSettingsCurrentWritesWhatPythonWrites(t *testing.T) {
	f := load(t)
	// The home is reached through a symlink, so its real path differs on every platform (a macOS tmp dir does by itself, a Linux one does not).
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.Settings {
		t.Run(c.Name, func(t *testing.T) {
			home := filepath.Join(root, "link", c.Name)
			real, err := filepath.EvalSymlinks(filepath.Join(root, "link"))
			if err != nil {
				t.Fatal(err)
			}
			real = filepath.Join(real, c.Name)
			h := hooksAt(home)
			if c.Before != nil {
				if err := os.MkdirAll(filepath.Dir(h.Target()), 0o755); err != nil {
					t.Fatal(err)
				}
				text := strings.NewReplacer("{home}", home, "{realhome}", real).Replace(*c.Before)
				if err := os.WriteFile(h.Target(), []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(filepath.Dir(h.Target()), "statusline.sh"), []byte("old"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			// What a launch does to keep them current (lifecycle.Launcher.Build).
			if err := h.EnsureShared(); (err != nil) != c.EnsureRefused {
				t.Fatalf("EnsureShared: %v, Python refused: %v", err, c.EnsureRefused)
			}
			if !c.EnsureRefused {
				if m := h.Missing(); m != c.Missing {
					t.Errorf("Missing %v, Python's %v", m, c.Missing)
				} else if m {
					if err := h.Install("/opt/bin/hl"); err != nil {
						t.Fatal(err)
					}
				}
				if err := h.AllowHomeSocket(); err != nil {
					t.Fatal(err)
				}
			}
			var after *string
			if b, err := os.ReadFile(h.Target()); err == nil {
				s := strings.ReplaceAll(strings.ReplaceAll(string(b), real, "{realhome}"), home, "{home}")
				after = &s
			}
			// Where Go departs from Python: the permission hook waits up to an hour for the founder (permission.ask).
			want := strings.Replace(str(c.After, "<none>"), "hook PermissionRequest\",\n            \"timeout\": 10",
				fmt.Sprintf("hook PermissionRequest\",\n            \"timeout\": %d", hookTimeoutS("PermissionRequest")), 1)
			if str(after, "<none>") != want {
				t.Errorf("settings\n got  %s\n want %s", str(after, "<none>"), want)
			}
			_, err = os.Stat(filepath.Join(filepath.Dir(h.Target()), "statusline.sh"))
			if (err == nil) != c.StatusLineKept {
				t.Errorf("statusline.sh kept: %v, Python's %v", err == nil, c.StatusLineKept)
			}
			// A second launch changes nothing.
			if !c.EnsureRefused {
				before := str(after, "")
				_ = h.EnsureShared()
				if h.Missing() {
					t.Error("still missing after Install")
				}
				_ = h.AllowHomeSocket()
				if b, _ := os.ReadFile(h.Target()); strings.ReplaceAll(strings.ReplaceAll(string(b), real, "{realhome}"), home, "{home}") != before {
					t.Error("a second launch changed the settings")
				}
			}
		})
	}
}

// An install from before the permission hook waited for the founder has its 10 s timeout: it counts as missing, and installing again
// gives it the hour.
func TestAnOldPermissionHookTimeoutIsPutRight(t *testing.T) {
	h := hooksAt(t.TempDir())
	if err := h.EnsureShared(); err != nil {
		t.Fatal(err)
	}
	if err := h.Install("/opt/bin/hl"); err != nil || h.Missing() {
		t.Fatalf("%v %v", err, h.Missing())
	}
	b, _ := os.ReadFile(h.Target())
	old := strings.Replace(string(b), fmt.Sprintf(`"timeout": %d`, hookTimeoutS("PermissionRequest")), `"timeout": 10`, 1)
	if old == string(b) {
		t.Fatal("no permission hook timeout to age")
	}
	os.WriteFile(h.Target(), []byte(old), 0o644)
	if !h.Missing() {
		t.Fatal("an old timeout reads as current")
	}
	if err := h.Install("/opt/bin/hl"); err != nil || h.Missing() {
		t.Fatalf("%v %v", err, h.Missing())
	}
}

// A path the session may read is allowed as itself, for a granted file, and with everything under it, for a folder.
func TestReadAllowAllowsThePathAndWhatIsUnderIt(t *testing.T) {
	argv := (Driver{}).LaunchArgv("/bin/claude", agent.LaunchArgs{Effort: "high", Disallowed: "Task", ReadAllow: []string{"/h/attachments/", "/etc/hosts"}})
	want := `"allow":["Read(//h/attachments)","Read(//h/attachments/**)","Read(//etc/hosts)","Read(//etc/hosts/**)"]`
	if s := argv[len(argv)-1]; !strings.Contains(strings.ReplaceAll(s, " ", ""), want) {
		t.Fatalf("%s\nwant %s", s, want)
	}
}

// Both keychain reads ask with keychainFind.
func TestBothKeychainReadsUseKeychainFind(t *testing.T) {
	t.Setenv("USER", "ron")
	oldGoos, oldRun := goos, run
	t.Cleanup(func() { goos, run = oldGoos, oldRun })
	var asked [][]string
	goos = "darwin"
	run = func(_ time.Duration, _ []string, argv ...string) (int, string) {
		asked = append(asked, argv)
		return 0, `    "mdat"<timedate>=0x00  "20261106000000Z\000"`
	}
	dir := t.TempDir()
	if present, known := (Driver{}).CredentialPresent(dir); !present || !known {
		t.Fatalf("present %v known %v", present, known)
	}
	if got := (Driver{}).CredentialModified(dir); got == nil || *got != 1793923200 {
		t.Fatalf("modified %v", got)
	}
	want := keychainFind(dir)
	if len(asked) != 2 || !slices.Equal(asked[0], want) || !slices.Equal(asked[1], want) {
		t.Fatalf("asked %q, want %q twice", asked, want)
	}
}

// The keychain item is asked for under the user's name, as claude files it.
func TestTheKeychainIsAskedUnderTheUsersName(t *testing.T) {
	dir := "/home/x/accounts/a/.claude"
	for user, want := range map[string]string{"ron": "ron", "a b": "claude-code-user"} {
		t.Setenv("USER", user)
		got := keychainFind(dir)
		if !slices.Equal(got, []string{"security", "find-generic-password", "-a", want, "-s", KeychainService(dir)}) {
			t.Errorf("USER=%q: %q", user, got)
		}
	}
}
