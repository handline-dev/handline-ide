package agentview

import "testing"

func TestAskLineDropsALeadingCdIntoTheAgentsOwnFolderOnly(t *testing.T) {
	const dir = "/w/repo/desk-2"
	for _, c := range []struct{ cmd, want string }{
		{"cd /w/repo/desk-2; git status", "Bash: git status"},
		{"cd /w/repo/desk-2 && go test ./...", "Bash: go test ./..."},
		{`cd "/w/repo/desk-2/" &&  make`, "Bash: make"},
		{"cd '/w/repo/desk-2';ls", "Bash: ls"},
		{"cd /w/other; ls", "Bash: cd /w/other; ls"},             // another folder says something
		{"cd /w/repo/desk-2", "Bash: cd /w/repo/desk-2"},         // nothing after it: the cd is the command
		{"cd /w/repo/desk-2 | x", "Bash: cd /w/repo/desk-2 | x"}, // not a ; or &&
		{"cd /w/repo/desk-2/sub; ls", "Bash: cd /w/repo/desk-2/sub; ls"},
		{`cd "/w/repo/desk-2; ls`, `Bash: cd "/w/repo/desk-2; ls`},
		{"git status", "Bash: git status"},
	} {
		if got := AskLine("Bash", c.cmd, dir); got != c.want {
			t.Errorf("%q: %q, want %q", c.cmd, got, c.want)
		}
	}
	if got := AskLine("Bash", "cd /w/repo/desk-2; ls", ""); got != "Bash: cd /w/repo/desk-2; ls" {
		t.Errorf("no folder known: %q", got)
	}
}
