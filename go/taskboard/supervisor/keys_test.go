package supervisor

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/paths"
)

// The rig's tests name agents (alice, bob) and the founder (human); the daemon writes and reads keys (a1, p1). k turns a name into its
// key on the way in, and named turns keys back into names in what a test reads. Writing names out is base-v2 1c's.

var keyWord = regexp.MustCompile(`\b(a[0-9]+|p1)\b`)

var tokenWord = regexp.MustCompile(`<@([^>]+)>`)

// namer is named over home's agents.
func namer(home string) func(string) string {
	return func(s string) string {
		m, _ := paths.AgentKeys(home)
		byKey := map[string]string{"p1": "human"}
		for n, k := range m {
			byKey[k] = n
		}
		s = tokenWord.ReplaceAllString(s, "$1") // <@key>, as a sentence names a who
		return keyWord.ReplaceAllStringFunc(s, func(k string) string {
			if n, ok := byKey[k]; ok {
				return n
			}
			return k
		})
	}
}

func (r *rig) k(name string) string                { return keyOf(r.t, r.home, name) }
func (r *rig) named(s string) string               { return namer(r.home)(s) }
func keyOf(t testing.TB, home, name string) string { return teamtest.Key(t, home, name) }

// namedQ is QueueDelivered read by name.
func (r *rig) namedQ(tid int64, by string, queued bool) string {
	return r.named(r.QueueDelivered(tid, by, queued))
}

// sql is q with each agent named in quotes ('bob', or "bob" in a JSON payload) by its key, longest names first, and 'human' by p1.
func (r *rig) sql(q string) string {
	m, _ := paths.AgentKeys(r.home)
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	for _, n := range names {
		q = strings.ReplaceAll(q, "'"+n+"'", "'"+m[n]+"'")
		q = strings.ReplaceAll(q, `"`+n+`"`, `"`+m[n]+`"`)
	}
	return strings.ReplaceAll(strings.ReplaceAll(q, "'human'", "'p1'"), `"human"`, `"p1"`)
}
