package reads

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/addr"

	"github.com/handline/handline/go/kit/yamlcompat"
)

// flat is a proposal file: a map of scalars (the roster's dump writes nothing nested), keys in file order.
type flat struct {
	keys []string
	m    map[string]any
}

func (f flat) get(k string) any { return f.m[k] }

// text is a field as text: booleans as "True" and "False", "" for a missing one.
func (f flat) text(k string) string {
	switch v := f.m[k].(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		if v {
			return "True"
		}
		return "False"
	default:
		return fmt.Sprint(v)
	}
}

// truthy is whether a field is set: not missing, "", false or 0.
func (f flat) truthy(k string) bool {
	switch v := f.m[k].(type) {
	case nil:
		return false
	case string:
		return v != ""
	case bool:
		return v
	case int64:
		return v != 0
	}
	return true
}

// parseFlat reads one proposal file: a map of null, booleans, integers and text, as the roster's dump writes it. A nested, flow
// or float value is an error, and the file is skipped.
func parseFlat(text string) (flat, error) {
	f := flat{m: map[string]any{}}
	pairs, ok, err := yamlcompat.ParsePairs(text)
	if err != nil {
		return f, err
	}
	if !ok {
		return f, fmt.Errorf("not a map")
	}
	for _, p := range pairs {
		switch p.Val.(type) {
		case nil, bool, int64, string:
		case float64:
			return f, fmt.Errorf("%s: number that is not an integer", p.Key)
		default:
			return f, fmt.Errorf("%s: nested or flow value", p.Key)
		}
		f.keys = append(f.keys, p.Key)
		f.m[p.Key] = p.Val
	}
	return f, nil
}

var itemKey = regexp.MustCompile(`^i(\d+)\.(.+)$`)

// items are a team diff's steps: the `i<n>.<field>` keys grouped by n, in order of n.
func (f flat) items() []flat {
	groups := map[int]*flat{}
	var order []int
	for _, k := range f.keys {
		m := itemKey.FindStringSubmatch(k)
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		g, ok := groups[n]
		if !ok {
			g = &flat{m: map[string]any{}}
			groups[n] = g
			order = append(order, n)
		}
		g.keys = append(g.keys, m[2])
		g.m[m[2]] = f.m[k]
	}
	sort.Ints(order)
	out := make([]flat, len(order))
	for i, n := range order {
		out[i] = *groups[n]
	}
	return out
}

// sets are the `set.<key>` fields, key to text, in file order.
func (f flat) sets() ([]string, map[string]string) {
	var keys []string
	vals := map[string]string{}
	for _, k := range f.keys {
		if strings.HasPrefix(k, "set.") {
			keys = append(keys, k[4:])
			vals[k[4:]] = f.text(k)
		}
	}
	return keys, vals
}

var stepOrder = []string{"team", "add", "rename", "settings", "schedule", "retire", "context", "global"}

func stepIndex(kind string) int {
	for i, k := range stepOrder {
		if k == kind {
			return i
		}
	}
	return len(stepOrder)
}

func orStr(s, alt string) string {
	if s == "" {
		return alt
	}
	return s
}

func members(p flat) []string {
	var out []string
	for _, m := range strings.Split(p.text("members"), ",") {
		if m != "" {
			out = append(out, m)
		}
	}
	return out
}

// runCommand is a run proposal's command and cwd; the command is kept JSON-encoded, as a proposal file holds no line break.
func (f flat) runCommand() (command, cwd string) {
	_, vals := f.sets()
	if json.Unmarshal([]byte(vals["command"]), &command) != nil {
		command = vals["command"]
	}
	return command, vals["cwd"]
}

// title is the card's head: `retire a`, `bundle 3 changes`.
func (f flat) title() string {
	if f.text("kind") == "bundle" {
		return fmt.Sprintf("bundle %d changes", len(f.items()))
	}
	return f.text("kind") + " " + addr.Ref(f.text("agent"))
}

// changeLines is what approving one change does, in words, a line for each thing it changes.
func changeLines(it flat) []string {
	kind, agent, to := it.text("kind"), it.text("agent"), it.text("to")
	switch kind {
	case "add":
		line := "register " + addr.Ref(agent)
		for _, k := range []string{"team", "repo", "model", "effort", "vendor"} {
			if it.truthy(k) {
				line += ", " + k + " " + it.text(k)
			}
		}
		return []string{line}
	case "team":
		line := "create team " + agent
		if it.truthy("repo") {
			line += " (working in " + it.text("repo") + ")"
		}
		if m := members(it); len(m) > 0 {
			line += " with members " + strings.Join(m, ", ")
		}
		return []string{line}
	case "settings":
		keys, vals := it.sets()
		if len(keys) == 0 {
			return []string{"change the settings of " + addr.Ref(agent)}
		}
		out := make([]string, len(keys))
		withWas := slices.ContainsFunc(it.keys, func(k string) bool { return strings.HasPrefix(k, "was.") })
		for i, k := range keys {
			if _, had := it.m["was."+k]; had { // the value it had when proposed
				out[i] = fmt.Sprintf("%s: %s -> %s", k, orStr(it.text("was."+k), "(none)"), orStr(vals[k], "(none)"))
				continue
			}
			if withWas { // its agent was read when proposed, and this one was already so
				out[i] = fmt.Sprintf("%s: %s (already so)", k, orStr(vals[k], "(none)"))
				continue
			}
			out[i] = fmt.Sprintf("set %s of %s to %s", k, addr.Ref(agent), orStr(vals[k], "(none)"))
		}
		return out
	case "context":
		keys, _ := it.sets()
		if len(keys) == 0 {
			return []string{"replace the standing context of " + addr.Ref(agent)}
		}
		out := make([]string, len(keys))
		for i, k := range keys {
			out[i] = fmt.Sprintf("replace %s of %s", k, addr.Ref(agent))
		}
		return out
	case "schedule":
		_, v := it.sets()
		switch v["action"] {
		case "remove":
			return []string{fmt.Sprintf("stop %s's schedule %s: %s", addr.Ref(agent), v["cron_was"], v["message_was"])}
		case "edit":
			return []string{fmt.Sprintf("change %s's schedule %s: %s\n  to %s: %s", addr.Ref(agent), v["cron_was"], v["message_was"], v["cron"], v["message"])}
		}
		return []string{fmt.Sprintf("schedule %s: %s for %s", v["cron"], v["message"], addr.Ref(agent))}
	case "rename":
		return []string{fmt.Sprintf("rename %s to %s", addr.Ref(agent), to)}
	case "retire":
		return []string{"retire " + addr.Ref(agent)}
	case "sandbox":
		_, v := it.sets()
		return []string{team.SandboxEntry{Kind: v["kind"], Value: v["value"]}.Phrase()}
	case "run":
		command, cwd := it.runCommand()
		return []string{fmt.Sprintf("run in %s, with its output told to %s: %s", cwd, addr.Ref(agent), command)}
	}
	return []string{kind + " " + addr.Ref(agent)}
}

// diff is what approving does, in words, one change per line (the spec's Proposal.diff; not a unified diff): the changes of a bundle in the
// order they are applied.
func (f flat) diff() string {
	var lines []string
	bundled := f.text("kind") == "bundle"
	for _, it := range f.steps() {
		for _, l := range changeLines(it) {
			if bundled && it.text("kind") == "settings" && !strings.HasPrefix(l, "set ") && !strings.HasPrefix(l, "change ") {
				l = addr.Ref(it.text("agent")) + " " + l // a bundle's steps may be several agents': each line says whose
			}
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "\n")
}

// steps are the proposal's changes in the order they are applied: itself, or a bundle's items.
func (f flat) steps() []flat {
	if f.text("kind") != "bundle" {
		return []flat{f}
	}
	items := f.items()
	sort.SliceStable(items, func(i, j int) bool { return stepIndex(items[i].text("kind")) < stepIndex(items[j].text("kind")) })
	return items
}

// subject is a digest of the fields that change what applying the proposal does: what proposal.decide checks back.
func (f flat) subject() string {
	fields := map[string]any{}
	for _, k := range []string{"kind", "agent", "to", "team", "repo", "model", "effort", "vendor",
		"response", "relation", "requested_by", "why", "members"} {
		fields[k] = f.get(k)
	}
	for _, k := range f.keys {
		if strings.HasPrefix(k, "repo.") || strings.HasPrefix(k, "set.") || itemKey.MatchString(k) {
			fields[k] = f.get(k)
		}
	}
	return sha256Hex(sortedASCIIJSON(fields))
}
