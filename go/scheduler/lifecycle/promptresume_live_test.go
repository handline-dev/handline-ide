package lifecycle

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/handline/handline/go/accounts/livetest"
)

// TestLiveAResumeKeepsItsSystemPrompt is the fact the settings rule stands on: claude --resume answers from the appended system prompt
// its conversation began with, not the one the resume passes, so the daemon owes prompt parts until a fresh start instead of restarting
// for them (PromptParts). If this fails, Claude Code now takes a new prompt on a resume, and that rule can be looked at again.
// HANDLINE_LIVE=1 go test -run TestLiveAResumeKeepsItsSystemPrompt -v ./scheduler/lifecycle
func TestLiveAResumeKeepsItsSystemPrompt(t *testing.T) {
	acct := livetest.Pick(t)
	path, err := exec.LookPath("claude")
	if err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80 // a version 4 uuid: claude takes no other session id
	sid := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
	dir := t.TempDir() // a resume finds its session by the folder it began in
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(path, append([]string{"-p", "--model", "haiku"}, args...)...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+acct.ConfigDir)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("claude %v: %v", args, err)
		}
		return strings.TrimSpace(string(out))
	}
	run("--session-id", sid, "--append-system-prompt", "The codeword is CHARLIE-3.", "Reply with only: OK")
	got := run("--resume", sid, "--append-system-prompt", "The codeword is DELTA-5.", "What is the codeword? Reply with only the codeword.")
	t.Logf("the resumed session says %q", got)
	if strings.Contains(got, "DELTA-5") {
		t.Fatal("a resume took the new system prompt: Claude Code now applies it, so owing prompt parts until a fresh start may be revisited")
	}
	if !strings.Contains(got, "CHARLIE-3") {
		t.Fatalf("the resumed session knows neither codeword: %q", got)
	}
}
