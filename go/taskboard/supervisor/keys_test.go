package supervisor

import (
	"regexp"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/paths"
)

// The rig's tests name agents (alice, bob) and the founder (human); the daemon writes and reads keys (a1, p1). k turns a name into its
// key on the way in, and named turns keys back into names in what a test reads. Writing names out is base-v2 1c's.

var keyWord = regexp.MustCompile(`\b(a[0-9]+|p1)\b`)

// namer is named over home's agents.
func namer(home string) func(string) string {
	return func(s string) string {
		m, _ := paths.AgentKeys(home)
		byKey := map[string]string{"p1": "human"}
		for n, k := range m {
			byKey[k] = n
		}
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
