package transcript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func bashLine(t *testing.T, uuid, parent, content string, extra map[string]any) string {
	t.Helper()
	e := map[string]any{"type": "user", "uuid": uuid, "parentUuid": parent, "isSidechain": false,
		"timestamp": "2026-11-06T12:00:51.095Z", "message": map[string]any{"role": "user", "content": content}}
	for k, v := range extra {
		e[k] = v
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return string(b) + "\n"
}

func bashFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing the transcript: %v", err)
	}
	return path
}

// The lines a real session wrote (T8684): input, then output naming it as parent, with other lines around them.
func TestAPersonsBashLineIsOneCommandWithItsOutput(t *testing.T) {
	path := bashFile(t, `{"type":"mode"}`+"\n"+
		bashLine(t, "a", "", "<bash-input>echo out; echo err 1>&2</bash-input>", nil)+
		bashLine(t, "b", "a", "<bash-stdout>out\nerr</bash-stdout><bash-stderr></bash-stderr>", nil)+
		`{"type":"api-request-blob","message":{"role":"user","content":"<bash-input>a copy</bash-input>"}}`+"\n"+
		bashLine(t, "c", "b", "<bash-input>ls /no; exit 3</bash-input>", nil)+
		bashLine(t, "d", "c", "<bash-stdout></bash-stdout><bash-stderr>ls: /no: No such file\n</bash-stderr>", nil)+
		bashLine(t, "e", "d", "an ordinary prompt", nil))
	got, err := BashCommands(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("%d commands: %+v", len(got), got)
	}
	if b := got[0]; b.Command != "echo out; echo err 1>&2" || b.Stdout != "out\nerr" || b.Stderr != "" || !b.Done || b.At != 1793966451 {
		t.Fatalf("first: %+v", b)
	}
	if b := got[1]; b.Command != "ls /no; exit 3" || b.Stdout != "" || b.Stderr != "ls: /no: No such file\n" || !b.Done {
		t.Fatalf("second: %+v", b)
	}
	if got[0].ID == got[1].ID {
		t.Fatalf("both are item %d", got[0].ID)
	}
	again, err := BashCommands(path)
	if err != nil || again[0].ID != got[0].ID {
		t.Fatalf("the id moved: %d, was %d (%v)", again[0].ID, got[0].ID, err)
	}
}

func TestABashLineWithNoOutputYetIsNotDone(t *testing.T) {
	got, err := BashCommands(bashFile(t, bashLine(t, "a", "", "<bash-input>sleep 100</bash-input>", nil)))
	if err != nil || len(got) != 1 || got[0].Done || got[0].Command != "sleep 100" {
		t.Fatalf("%+v, %v", got, err)
	}
}

// An output line belongs to the input it names: not to the last one, and a line whose input is gone is no command.
func TestBashOutputGoesToTheInputItNames(t *testing.T) {
	got, err := BashCommands(bashFile(t, bashLine(t, "x", "gone", "<bash-stdout>orphan</bash-stdout><bash-stderr></bash-stderr>", nil)+
		bashLine(t, "a", "", "<bash-input>first</bash-input>", nil)+
		bashLine(t, "b", "", "<bash-input>second</bash-input>", nil)+
		bashLine(t, "c", "a", "<bash-stdout>one</bash-stdout><bash-stderr></bash-stderr>", nil)))
	if err != nil || len(got) != 2 || got[0].Stdout != "one" || got[1].Done {
		t.Fatalf("%+v, %v", got, err)
	}
}

func TestASubagentsBashLineIsNotThePersons(t *testing.T) {
	got, err := BashCommands(bashFile(t, bashLine(t, "a", "", "<bash-input>theirs</bash-input>", map[string]any{"isSidechain": true})))
	if err != nil || len(got) != 0 {
		t.Fatalf("%+v, %v", got, err)
	}
}

// The session escapes output but not the command; the escape of an escape comes back as what was printed. No output is recorded as words.
func TestBashOutputReadsAsPrinted(t *testing.T) {
	got, err := BashCommands(bashFile(t, bashLine(t, "a", "", "<bash-input>echo '<x>&amp;' && true</bash-input>", nil)+
		bashLine(t, "b", "a", "<bash-stdout>&lt;x&gt;&amp;amp;</bash-stdout><bash-stderr>a &lt; b</bash-stderr>", nil)+
		bashLine(t, "c", "", "<bash-input>true</bash-input>", nil)+
		bashLine(t, "d", "c", "<bash-stdout>(Bash completed with no output)</bash-stdout><bash-stderr></bash-stderr>", nil)))
	if err != nil || len(got) != 2 {
		t.Fatalf("%+v, %v", got, err)
	}
	if b := got[0]; b.Command != "echo '<x>&amp;' && true" || b.Stdout != "<x>&amp;" || b.Stderr != "a < b" {
		t.Fatalf("%+v", b)
	}
	if b := got[1]; b.Stdout != "" || !b.Done {
		t.Fatalf("%+v", b)
	}
}

func TestLongBashOutputIsCutAndSaysSo(t *testing.T) {
	long := strings.Repeat("a line\n", ResultMax)
	got, err := BashCommands(bashFile(t, bashLine(t, "a", "", "<bash-input>yes</bash-input>", nil)+
		bashLine(t, "b", "a", "<bash-stdout>"+long+"</bash-stdout><bash-stderr>"+long+"</bash-stderr>", nil)))
	if err != nil || len(got) != 1 {
		t.Fatalf("%+v, %v", got, err)
	}
	if b := got[0]; len(b.Stdout) > ResultMax || len(b.Stderr) > ResultMax || b.Dropped < 2 {
		t.Fatalf("stdout %d, stderr %d, dropped %d", len(b.Stdout), len(b.Stderr), b.Dropped)
	}
}

// Past the tail: the line the read began inside is not parsed, and a command whose input was cut off is not there.
func TestBashCommandsReadOnlyTheTail(t *testing.T) {
	old := bashLine(t, "a", "", "<bash-input>old</bash-input>", nil)
	path := bashFile(t, old+`{"type":"filler","x":"`+strings.Repeat("y", BashTail)+`"}`+"\n"+
		bashLine(t, "b", "a", "<bash-stdout>late</bash-stdout><bash-stderr></bash-stderr>", nil)+
		bashLine(t, "c", "", "<bash-input>new</bash-input>", nil))
	got, err := BashCommands(path)
	if err != nil || len(got) != 1 || got[0].Command != "new" {
		t.Fatalf("%+v, %v", got, err)
	}
}

// A line still being written is read next time.
func TestAnUnfinishedBashLineIsLeft(t *testing.T) {
	line := bashLine(t, "a", "", "<bash-input>half</bash-input>", nil)
	got, err := BashCommands(bashFile(t, line[:len(line)-1]))
	if err != nil || len(got) != 0 {
		t.Fatalf("%+v, %v", got, err)
	}
}
