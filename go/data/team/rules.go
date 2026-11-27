package team

import (
	_ "embed"
	"fmt"
	"slices"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
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

// RulesTextLimit is the most bytes a workspace's rules should hold (docs/design/people-workspaces.md, Workspace rules).
const RulesTextLimit = 2 << 10

// RulesText is the workspace's rules in force (db.PartRules); ws is its name or key. "" for none.
func RulesText(q db.Q, ws string) (string, error) {
	k, err := workspaceKeyOf(q, ws)
	if k == "" || err != nil {
		return "", err
	}
	return db.InstructionText(q, k, db.PartRules)
}

func workspaceKeyOf(q db.Q, ws string) (string, error) {
	if actor.KindOf(ws) == actor.Workspace {
		return ws, nil
	}
	return paths.WorkspaceKeyIn(q, ws)
}

// RuleTeams is each team the agent is in that has rules, sorted by name.
func RuleTeams(q db.Q, agent string, t *Team) []string {
	if t == nil {
		return nil
	}
	var names []string
	for name, members := range t.Teams {
		if text, err := RulesText(q, name); slices.Contains(members, agent) && err == nil && textsafe.Strip(text) != "" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// Rules is each of the agent's teams' rules, as "# Rules for team X\n<text>": its rules in force, then kept(team),
// the kept rules in force (RulesBlock). A team with neither has none.
func Rules(q db.Q, agent string, t *Team, kept func(team string) string) []string {
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
		rules, _ := RulesText(q, name)
		text := textsafe.Strip(rules)
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

// SeedRules gives each team that has never had rules DefaultRules as its first version, so a new team starts with the house rules while
// rules its team emptied stay that way.
func SeedRules(q db.Q, t *Team, at int64) error {
	if t == nil {
		return nil
	}
	names := make([]string, 0, len(t.Teams))
	for name := range t.Teams {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		k, err := paths.WorkspaceKeyIn(q, name)
		if err != nil {
			return err
		}
		if k == "" {
			continue // no workspace row: nothing to seed
		}
		if err := seedWorkspace(q, k, at); err != nil {
			return err
		}
	}
	return nil
}

// SeedActor is who writes what Handline itself puts in place: the house rules a new team starts with.
const SeedActor = "Handline"

// seedWorkspace is SeedRules for the workspace whose key is k.
func seedWorkspace(q db.Q, k string, at int64) error {
	if was, err := db.CurrentInstruction(q, k, db.PartRules); err != nil || was != nil {
		return err
	}
	_, err := db.SetInstruction(q, k, db.PartRules, DefaultRules, SeedActor, "", at)
	return err
}
