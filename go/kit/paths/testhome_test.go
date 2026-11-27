package paths

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A test binary started with HANDLINE_HOME at the live home never resolves it: the binary is run again with it set, and its Home is
// another folder, with the variable gone for anything it starts. A scratch home it inherits stays.
func TestATestBinaryDropsAnInheritedLiveHome(t *testing.T) {
	if os.Getenv("HL_PATHS_CHILD") == "1" {
		v, set := os.LookupEnv("HANDLINE_HOME")
		os.Stdout.WriteString("home=" + Home() + "\nset=" + map[bool]string{true: v, false: "<unset>"}[set] + "\n")
		return
	}
	live := filepath.Join(RealUserHome(), ".handline")
	scratch := t.TempDir()
	for _, tc := range []struct{ inherited, home, set string }{
		{live, "", "<unset>"},
		{scratch, scratch, scratch},
	} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestATestBinaryDropsAnInheritedLiveHome$")
		cmd.Env = append(os.Environ(), "HL_PATHS_CHILD=1", "HANDLINE_HOME="+tc.inherited)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		home, set := between(string(out), "home=", "\n"), between(string(out), "set=", "\n")
		if IsLive(home) || (tc.home != "" && home != tc.home) || set != tc.set {
			t.Fatalf("inherited %s: home %q, set %q", tc.inherited, home, set)
		}
	}
}

func between(s, from, to string) string {
	_, rest, _ := strings.Cut(s, from)
	v, _, _ := strings.Cut(rest, to)
	return v
}
