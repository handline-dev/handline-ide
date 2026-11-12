package weblogin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/data/team/teamtest"
)

// deviceVendor is a CLI whose web login prints a URL and a one-time code (as codex login --device-auth does), logged in once its
// config dir holds auth.json.
type deviceVendor struct{}

func (deviceVendor) Name() string          { return "devicefake" }
func (deviceVendor) ConfigDirName() string { return ".devicefake" }
func (deviceVendor) ConfigDirEnv() string  { return "DEVICEFAKE_HOME" }
func (deviceVendor) LoginArgv() []string   { return []string{"claude", "login"} }
func (deviceVendor) WebLoginArgv() ([]string, bool) {
	return []string{"claude", "login", "--device-auth"}, true
}
func (deviceVendor) LogoutArgv() []string               { return nil }
func (deviceVendor) CredentialModified(string) *float64 { return nil }
func (deviceVendor) DeviceCodeLogin() bool              { return true }
func (deviceVendor) CredentialPresent(dir string) (bool, bool) {
	_, err := os.Stat(filepath.Join(dir, "auth.json"))
	return err == nil, true
}

func init() { agent.Register(deviceVendor{}) }

const (
	deviceURL = "https://auth.example/codex/device"
	fakeCode  = "AB12-CD3EF"
)

// deviceSays is codex's device-auth text, coloured as it prints it.
const deviceSays = `printf 'Follow these steps to sign in with ChatGPT using device code authorization:\n'
printf '1. Open this link in your browser and sign in to your account\n   \033[94m` + deviceURL + `\033[0m\n'
printf '2. Enter this one-time code \033[90m(expires in 15 minutes)\033[0m\n   \033[94m` + fakeCode + `\033[0m\n'
`

func newDeviceRig(t *testing.T, script string) *rig {
	t.Helper()
	r := newRig(t, script)
	if err := os.WriteFile(filepath.Join(teamtest.AccountDir(t, r.home, "main"), "vendor"), []byte("devicefake\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestADeviceLoginAnswersItsURLAndCodeAndTakesNoCodeBack(t *testing.T) {
	r := newDeviceRig(t, deviceSays+"sleep 0.3\ntouch \"$DEVICEFAKE_HOME/auth.json\"\n")
	lines := r.logged()
	l, err := r.m.Start("main")
	if err != nil {
		t.Fatal(err)
	}
	if l.URL != deviceURL || l.DeviceCode != fakeCode || l.State != Waiting {
		t.Fatalf("got %+v", l)
	}
	if got, _ := r.m.ByID(l.ID); got.DeviceCode != fakeCode {
		t.Fatalf("a retried start lost the code: %+v", got)
	}
	if got := r.m.Of("main"); got.DeviceCode != "" {
		t.Fatalf("the login row carries the code: %+v", got)
	}
	var no *Refusal
	if err := r.m.SendCode(l.ID, "whatever"); !errors.As(err, &no) {
		t.Fatalf("a code was taken: %v", err)
	}
	r.settles("main", Done)
	if got, _ := r.m.ByID(l.ID); got.DeviceCode != "" || got.URL != "" {
		t.Fatalf("the code outlived the login: %+v", got)
	}
	r.eventually("its end is logged", func() bool { return len(lines()) == 1 })
	if strings.Contains(strings.Join(lines(), "\n"), fakeCode) {
		t.Fatalf("logged the code: %q", lines())
	}
}

func TestAFailedDeviceLoginNeverGivesItsCodeAsTheReason(t *testing.T) {
	r := newDeviceRig(t, deviceSays+"exit 1\n") // no pause: the exit may be read before Start has the code
	if _, err := r.m.Start("main"); err != nil {
		t.Fatal(err)
	}
	r.settles("main", Failed)
	if got := r.m.Of("main").Reason; got != "the login command exited with an error" {
		t.Fatalf("%q", got)
	}
}

func TestADeviceLoginThatPrintsNoCodeIsNotAnswered(t *testing.T) {
	r := newDeviceRig(t, "echo "+deviceURL+"\n")
	if _, err := r.m.Start("main"); err == nil {
		t.Fatal("answered with no code")
	}
}

func TestAColouredURLIsReadForAnyLogin(t *testing.T) {
	r := newRig(t, "printf '\\033[94m"+fakeURL+"\\033[0m\\n'\ncat > /dev/null\n")
	l, err := r.m.Start("main")
	if err != nil || l.URL != fakeURL || l.DeviceCode != "" {
		t.Fatalf("%+v %v", l, err)
	}
}
