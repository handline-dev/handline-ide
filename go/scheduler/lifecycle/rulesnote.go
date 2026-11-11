package lifecycle

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/handline/handline/go/data/db"
)

// LaunchedRules is beside the setting hashes too: the team rules text the session was given, for telling it what changed.
const LaunchedRules = "rules"

const rulesPart = "team rules"

// rulesText is the team rules as a launch gives them.
func (l *Launcher) rulesText(agent string) string {
	return strings.Join(l.Ports.TeamRules(agent), "\n\n")
}

// RulesNote is the changed lines of the agent's team rules since the launch row records (all of them when it kept no text), and the
// record that says it was told, for KeepRulesTold once the note is sent.
func (l *Launcher) RulesNote(agent string, row *db.AgentState) (note, record string, err error) {
	var rec map[string]json.RawMessage
	if json.Unmarshal([]byte(row.LaunchParts.String), &rec) != nil || rec == nil {
		return "", "", nil
	}
	rules := l.Ports.TeamRules(agent) // read once: the hash and the text are of the same version
	hash := rulesHash(rules)
	var was string
	if json.Unmarshal(rec[rulesPart], &was) != nil || was == hash {
		return "", "", nil
	}
	now := strings.Join(rules, "\n\n")
	var before string
	hadText := json.Unmarshal(rec[LaunchedRules], &before) == nil
	rec[rulesPart], _ = json.Marshal(hash)
	rec[LaunchedRules], _ = json.Marshal(now)
	b, err := json.Marshal(rec)
	if err != nil {
		return "", "", err
	}
	where := l.sourceOf(agent, rulesPart)
	if where != "" {
		where = " (" + where + ")"
	}
	head := fmt.Sprintf("Your team rules changed%s. Your session goes on; follow the new rules from now.", where)
	if !hadText {
		return head + " They now read:\n\n" + now, string(b), nil
	}
	added, removed := lineChanges(before, now)
	var out []string
	if len(added) > 0 {
		out = append(out, "Added:\n+ "+strings.Join(added, "\n+ "))
	}
	if len(removed) > 0 {
		out = append(out, "Removed:\n- "+strings.Join(removed, "\n- "))
	}
	if len(out) == 0 {
		return "", string(b), nil
	}
	return head + "\n\n" + strings.Join(out, "\n\n"), string(b), nil
}

// KeepRulesTold puts record in place of was as the session's launch record, unless a launch has written another since.
func (l *Launcher) KeepRulesTold(q db.Q, agent, was, record string) error {
	return db.ReplaceLaunchParts(q, agent, was, record)
}

func rulesHash(rules []string) string { return sha256Hex(strings.Join(rules, "\x1e"))[:8] }

// lineChanges is the lines b adds to a and the ones it takes out, in order, by the longest common subsequence of their non-blank lines.
func lineChanges(a, b string) (added, removed []string) {
	la, lb := nonBlank(a), nonBlank(b)
	n, m := len(la), len(lb)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if la[i] == lb[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && la[i] == lb[j]:
			i, j = i+1, j+1
		case j < m && (i == n || lcs[i][j+1] >= lcs[i+1][j]):
			added = append(added, lb[j])
			j++
		default:
			removed = append(removed, la[i])
			i++
		}
	}
	return added, removed
}

func nonBlank(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
