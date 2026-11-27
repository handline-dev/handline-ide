package lifecycle

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	cli "github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/agent/claudecode"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/yamlcompat"
)

// chatDriver is the fake vendor with Claude's Chat mode.
type chatDriver struct{ fakeDriver }

var cc = claudecode.Driver{}

func (chatDriver) ChatArgv(argv []string) []string              { return cc.ChatArgv(argv) }
func (chatDriver) Init() ([]byte, string)                       { return cc.Init() }
func (chatDriver) UserLine(uuid, text string) []byte            { return cc.UserLine(uuid, text) }
func (chatDriver) InterruptLine(id string) []byte               { return cc.InterruptLine(id) }
func (chatDriver) StopTaskLine(id, task string) []byte          { return cc.StopTaskLine(id, task) }
func (chatDriver) Env() []string                                { return cc.Env() }
func (chatDriver) Read(line []byte) cli.Event                   { return cc.Read(line) }
func (chatDriver) Answer(ask cli.Event, a cli.AskAnswer) []byte { return cc.Answer(ask, a) }
func (chatDriver) Unserved(req cli.Event) []byte                { return cc.Unserved(req) }

type chatVendors struct{ fakeVendors }

func (chatVendors) ForAgent(string) (cli.Agent, error) {
	return chatDriver{fakeDriver{vendor: "claude"}}, nil
}

func TestAChatAgentsLaunchRunsUnderTheHolderWithItsPromptAsTheFirstMessage(t *testing.T) {
	r := newLaunchRig(t)
	r.l.Ports.Vendors = chatVendors{fakeVendors{"claude"}}
	prompt := func() string {
		f := filepath.Join(t.TempDir(), "p")
		os.WriteFile(f, []byte("-take the task"), 0o600)
		return f
	}
	got, err := r.l.Prepare(r.site(), "ann", nil, false, prompt(), "main", "", "")
	if err != nil || got.Hold != nil {
		t.Fatalf("Terminal is the default: %+v %v", got, err)
	}
	if err := r.l.Roster.SetKeys("ann", []yamlcompat.Pair{{Key: "session_mode", Val: "chat"}}); err != nil {
		t.Fatal(err)
	}
	if cfg, _ := r.l.Roster.Read("ann"); team.SessionMode(cfg) != "chat" {
		t.Fatal("the setting did not take")
	}
	got, err = r.l.Prepare(r.site(), "ann", nil, false, prompt(), "main", "", "")
	if err != nil || got.Hold == nil {
		t.Fatalf("%+v %v", got, err)
	}
	if slices.Contains(got.Argv, "-take the task") || !slices.Contains(got.Argv, "--print") || !slices.Contains(got.Argv, "--effort") {
		t.Fatalf("the terminal line headless, without the prompt: %v", got.Argv)
	}
	if !slices.Contains(got.Env, cc.Env()[0]) {
		t.Fatalf("the CLI's Chat env: %v", got.Env)
	}
	h := got.Hold
	if len(h.Init) != 2 || !strings.Contains(h.Init[0], h.Secret) || !strings.Contains(h.Init[1], "-take the task") {
		t.Fatalf("the opening line, then the prompt: %+v", h)
	}
	if fi, err := os.Stat(filepath.Dir(h.Socket)); err != nil || !fi.IsDir() || !strings.HasPrefix(h.Socket, r.l.Home) {
		t.Fatalf("the socket's folder is made under the home: %s %v", h.Socket, err)
	}
	site := r.site()
	site.Node, site.Handle = "box", "box:%3"
	r.l.Ports.Devices = fakeDevices{"box": {"main": "/runner/accounts/k1/.claude"}}
	if got, err := r.l.Prepare(site, "ann", nil, false, prompt(), "main", "", ""); err != nil || got.Hold != nil {
		t.Fatalf("a runner's session stays a terminal one for now: %+v %v", got, err)
	}
}

// A Chat session works in its granted folders beside its own, as a terminal one does once /add-dir gave them; a terminal launch does not.
func TestAChatLaunchAddsTheGrantedFolders(t *testing.T) {
	r := newLaunchRig(t)
	r.l.Ports.Vendors = chatVendors{fakeVendors{"claude"}}
	dir, file := t.TempDir(), filepath.Join(t.TempDir(), "f")
	os.WriteFile(file, nil, 0o600)
	granted := []string{dir, file, "/missing"}
	if got := r.l.addDirs(r.site(), "ann", granted); slices.Contains(got, dir) {
		t.Fatalf("a terminal launch: %v", got)
	}
	if err := r.l.Roster.SetKeys("ann", []yamlcompat.Pair{{Key: "session_mode", Val: "chat"}}); err != nil {
		t.Fatal(err)
	}
	if got := r.l.addDirs(r.site(), "ann", granted); !slices.Contains(got, dir) || slices.Contains(got, file) || slices.Contains(got, "/missing") {
		t.Fatalf("a Chat launch: %v", got)
	}
	site := r.site()
	site.Node = "box"
	if got := r.l.addDirs(site, "ann", granted); slices.Contains(got, dir) {
		t.Fatalf("a runner's launch: %v", got)
	}
}
