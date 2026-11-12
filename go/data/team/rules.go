package team

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/textsafe"
)

//go:embed default_rules.md
var rulesFile string

const protocolMark = "\n<!-- protocol -->\n"

// DefaultRules is the house culture each team's rules file starts as: default_rules.md above its protocol mark.
var DefaultRules = strings.TrimSpace(rulesFile[:strings.Index(rulesFile, protocolMark)]) + "\n"

// protocol is the team protocol, the "## <key>" statements of default_rules.md below its mark: the one place it is written.
var protocol = parseProtocol(rulesFile[strings.Index(rulesFile, protocolMark)+len(protocolMark):])

func parseProtocol(text string) map[string]string {
	out := map[string]string{}
	for _, sec := range strings.Split(text, "\n## ")[1:] {
		key, body, _ := strings.Cut(sec, "\n")
		out[strings.TrimSpace(key)] = strings.TrimSpace(body)
	}
	return out
}

// Protocol is the protocol statement named key. An unknown key is a mistake in the code, so it panics.
func Protocol(key string) string {
	text, ok := protocol[key]
	if !ok {
		panic("team.Protocol: no statement " + key + " in default_rules.md")
	}
	return text
}

// ProtocolBlock is the body of a generated doc block: each statement named, as one Markdown bullet.
func ProtocolBlock(keys []string) string {
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = "- " + Protocol(k)
	}
	return strings.Join(lines, "\n")
}

// RulesTextLimit is the most bytes a workspace's base.md should hold (docs/design/people-workspaces.md, Workspace rules).
const RulesTextLimit = 2 << 10

// RulesFile is the team's rules, its workspace's base.md (docs/design/people-workspaces.md, Disk); ws is its name or key.
func RulesFile(home, ws string) string { return filepath.Join(paths.WorkspaceDir(home, ws), "base.md") }

// teamRules is the rules file of each team the agent is in that has rules, by team name, sorted by name.
func teamRules(home, agent string, t *Team) (names, files []string) {
	if t == nil {
		return nil, nil
	}
	for name, members := range t.Teams {
		b, err := os.ReadFile(RulesFile(home, name))
		if slices.Contains(members, agent) && err == nil && textsafe.Strip(string(b)) != "" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	for _, name := range names {
		files = append(files, RulesFile(home, name))
	}
	return names, files
}

// RuleFiles is the rules file of each team the agent is in that has rules, as paths under home.
func RuleFiles(home, agent string, t *Team) []string {
	_, files := teamRules(home, agent, t)
	for i, f := range files {
		if rel, err := filepath.Rel(home, f); err == nil {
			files[i] = rel
		}
	}
	return files
}

// Rules is each of the agent's teams' rules, as "# Rules for team X\n<text>" (launch._team_rules): its rules file, then kept(team), the
// kept rules in force (RulesBlock). A team with neither has none.
func Rules(home, agent string, t *Team, kept func(team string) string) []string {
	if t == nil {
		return nil
	}
	var names []string
	for name, members := range t.Teams {
		if slices.Contains(members, agent) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	var out []string
	for _, name := range names {
		b, _ := os.ReadFile(RulesFile(home, name))
		text := textsafe.Strip(string(b))
		if block := kept(name); block != "" {
			if text != "" {
				text += "\n\n"
			}
			text += block
		}
		if text != "" {
			out = append(out, fmt.Sprintf("# Rules for team %s\n%s", name, text))
		}
	}
	return out
}

// SeedRules writes DefaultRules as the rules file of each team never seeded before, and marks its workspace folder seeded, so a
// new team starts with the house rules while a file its team emptied or deleted stays that way.
func SeedRules(home string, t *Team) error {
	if t == nil {
		return nil
	}
	names := make([]string, 0, len(t.Teams))
	for name := range t.Teams {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		k := paths.WorkspaceKey(home, name)
		if k == "" {
			continue // no workspace row: nothing is written under .unregistered
		}
		if err := seedWorkspace(home, k); err != nil {
			return err
		}
	}
	return nil
}

// seedWorkspace is SeedRules for the workspace whose key is k.
func seedWorkspace(home, k string) error {
	dir := filepath.Join(home, "workspaces", k)
	marker := filepath.Join(dir, ".seeded")
	if _, err := os.Stat(marker); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "base.md"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err == nil {
		_, err = f.WriteString(DefaultRules)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	} else if os.IsExist(err) {
		err = nil // a team that already wrote its own rules keeps them
	}
	if err != nil {
		return err
	}
	return os.WriteFile(marker, nil, 0o644)
}
