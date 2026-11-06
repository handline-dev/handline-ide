package team

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docBlock is a generated protocol block in a doc: the opening comment names the statements, the body is ProtocolBlock of them.
var docBlock = regexp.MustCompile(`(?ms)^<!-- protocol: ([a-z -]+) -->\n(.*?)<!-- /protocol -->`)

// TestProtocolDocsAreCurrent: AGENTS.md and skill/SKILL.md carry the protocol only as blocks generated from default_rules.md.
// HANDLINE_WRITE_DOCS=1 rewrites the blocks instead of failing.
func TestProtocolDocsAreCurrent(t *testing.T) {
	write := os.Getenv("HANDLINE_WRITE_DOCS") == "1"
	for _, name := range []string{"AGENTS.md", "skill/SKILL.md"} {
		path := filepath.Join("..", "..", "..", name)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(b)
		found := 0
		fresh := docBlock.ReplaceAllStringFunc(text, func(block string) string {
			found++
			head := docBlock.FindStringSubmatch(block)[1]
			return "<!-- protocol: " + head + " -->\n" + ProtocolBlock(strings.Fields(head)) + "\n<!-- /protocol -->"
		})
		if found == 0 {
			t.Errorf("%s has no protocol block", name)
		}
		if fresh == text {
			continue
		}
		if write {
			if err := os.WriteFile(path, []byte(fresh), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		t.Errorf("%s: a protocol block differs from go/data/team/default_rules.md; HANDLINE_WRITE_DOCS=1 go test ./data/team -run TestProtocolDocsAreCurrent rewrites it", name)
	}
}

func TestDefaultRulesSeedOnlyTheHouseRules(t *testing.T) {
	if strings.Contains(DefaultRules, "protocol") || strings.Contains(DefaultRules, "## ") {
		t.Errorf("the seed carries the protocol: %q", DefaultRules)
	}
	if !strings.HasPrefix(DefaultRules, "- Keep messages short.") || !strings.HasSuffix(DefaultRules, "does not mean healthy.\n") {
		t.Errorf("seed: %q", DefaultRules)
	}
}

func TestProtocolReadsEachStatementByKey(t *testing.T) {
	got := parseProtocol("intro\n\n## a\none line\n\n## b-c\ntwo\nlines\n")
	if len(got) != 2 || got["a"] != "one line" || got["b-c"] != "two\nlines" {
		t.Errorf("got %q", got)
	}
	if ProtocolBlock([]string{"terminal"}) != "- "+Protocol("terminal") {
		t.Error("block")
	}
	defer func() {
		if recover() == nil {
			t.Error("an unknown key did not panic")
		}
	}()
	Protocol("no-such-statement")
}
