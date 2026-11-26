package access

import (
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/protected"
)

// Rule is a Bash permission the founder always allows an agent, in Claude Code's rule form: Bash(content). Each fresh session of the
// agent, and of its temps, starts with it in its permissions' allow list.
type Rule struct {
	Rule string `json:"rule"`
	At   int64  `json:"at"`
}

// rulesNS is where the founder's always-allowed rules are kept: the protected store, which no agent can write, so a rule there is one
// the founder pressed Always allow for.
const rulesNS = "always-allow"

// RuleOwner is whose rules agent's sessions start with: a temp's are those of the agent it was hired from.
func RuleOwner(q db.Q, agent string) (string, error) {
	kind, id, ok := actor.Parse(agent)
	if !ok || kind != actor.Agent {
		return agent, nil
	}
	var from sql.NullString
	err := q.QueryRow("SELECT hired_from FROM agents WHERE id = ?", id).Scan(&from)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && from.String == "") {
		return agent, nil
	}
	return from.String, err
}

// Rules is the agent's always-allowed rules, newest first.
func Rules(home, agent string) ([]Rule, error) {
	v, ok := protected.New(home).Get(rulesNS, agent)
	if !ok || v == "" {
		return []Rule{}, nil
	}
	var rs []Rule
	return rs, json.Unmarshal([]byte(v), &rs)
}

func saveRules(home, agent string, rs []Rule) error {
	st := protected.New(home)
	if len(rs) == 0 {
		return st.Delete(rulesNS, agent)
	}
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].At > rs[j].At })
	b, err := json.Marshal(rs)
	if err != nil {
		return err
	}
	return st.Put(rulesNS, agent, string(b))
}

// rulesMu orders the read and write of a rules entry: an agent and its temps share one, and two answers at once must both stay.
var rulesMu sync.Mutex

// AddRule keeps rule as the agent's, once. Only the founder's process may: the store refuses an agent's session.
func AddRule(home, agent, rule string, now time.Time) error {
	rulesMu.Lock()
	defer rulesMu.Unlock()
	rs, err := Rules(home, agent)
	if err != nil {
		return err
	}
	rs = slices.DeleteFunc(rs, func(r Rule) bool { return r.Rule == rule })
	return saveRules(home, agent, append(rs, Rule{Rule: rule, At: now.Unix()}))
}

// RemoveRule takes rule back, and says whether the agent had it. A session that started with it keeps it until its next fresh start.
func RemoveRule(home, agent, rule string) (bool, error) {
	rulesMu.Lock()
	defer rulesMu.Unlock()
	rs, err := Rules(home, agent)
	if err != nil {
		return false, err
	}
	kept := slices.DeleteFunc(slices.Clone(rs), func(r Rule) bool { return r.Rule == rule })
	if len(kept) == len(rs) {
		return false, nil
	}
	return true, saveRules(home, agent, kept)
}

// RulesForLaunch is the rules a session of agent starts with: its own, or for a temp those of the agent it was hired from. Each is
// checked again (SafeRule), so a rule kept before a check tightened is not carried.
func RulesForLaunch(q db.Q, home, agent string) ([]string, error) {
	owner, err := RuleOwner(q, agent)
	if err != nil {
		return nil, err
	}
	rs, err := Rules(home, owner)
	if err != nil {
		return nil, nil // an entry that cannot be read starts the session with no rules, never stops it
	}
	var out []string
	for _, r := range rs {
		if SafeRule(home, r.Rule) {
			out = append(out, r.Rule)
		}
	}
	sort.Strings(out) // the launch line stays the same for the same rules
	return out, nil
}

// neverWords are what an always-allowed command may not name, as a word or within one: typing into another terminal, a credential,
// the user's secret folders, the daemon's records, and a push, which asks each time unless it is the lander's exact push. A best
// effort beside the founder's own reading of the exact text, which is the check that counts.
var neverWords = []string{"send-keys", "tmux", "osascript", "security", "credential", "keychain", "token", "secret", "password",
	".ssh", "handline", "push", "sqlite3"}

// shellChars are what may make a rule mean other than its text: a wildcard, quoting and escapes, substitution, a home or variable
// path, and the operators Claude Code splits a command at or matches around (; & | < > and parentheses).
const shellChars = "*\\()$`'\"~;&|<>\n\r\t{}[]!#"

// SafeRule is whether Always allow may keep rule: a Bash rule for one plain exact command, so the rule covers that text and nothing
// more. One with a shell character, words not single-spaced, over 300 characters, or naming a never word, the handline home or the
// protected store, in any letter case, is never kept.
func SafeRule(home, rule string) bool {
	content, ok := strings.CutPrefix(rule, "Bash(")
	if !ok || !strings.HasSuffix(content, ")") {
		return false
	}
	content = strings.TrimSuffix(content, ")")
	if content == "" || len(content) > 300 || strings.ContainsAny(content, shellChars) || strings.Join(strings.Fields(content), " ") != content {
		return false
	}
	low := strings.ToLower(content)
	for _, bad := range append([]string{strings.ToLower(home), strings.ToLower(protected.RootFor(home))}, neverWords...) {
		if bad != "" && strings.Contains(low, bad) {
			return false
		}
	}
	return true
}
