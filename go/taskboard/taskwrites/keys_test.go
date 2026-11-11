package taskwrites

import (
	"database/sql"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/paths"
)

// homes is the home each rig's task database names its agents in, so exec and count write and read them by key.
var homes sync.Map // *sql.DB -> string

// keyed is q with each agent of db's home named in quotes ('w', or "w" in a JSON payload) by its key ('a1'), longest names first,
// and the founder's "human" by p1.
func keyed(db *sql.DB, q string) string {
	h, ok := homes.Load(db)
	if !ok {
		return q
	}
	m, _ := paths.AgentKeys(h.(string))
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	for _, n := range names {
		q = strings.ReplaceAll(q, "'"+n+"'", "'"+m[n]+"'")
		q = strings.ReplaceAll(q, `"`+n+`"`, `"`+m[n]+`"`) // a JSON payload's value
	}
	return strings.ReplaceAll(q, `"human"`, `"p1"`)
}

// as is the caller agent's key in home; "" (the founder) stays "".
func as(t testing.TB, home, agent string) string {
	if agent == "" {
		return ""
	}
	return teamtest.Key(t, home, agent)
}

var keyWord = regexp.MustCompile(`\b(a[0-9]+|p1)\b`)

// named is s with each key of db's home written as the name it stands for (p1 as human), so a test reads replies by name. Writing
// names out is base-v2 1c's; until then the daemon's sentences carry keys.
func named(db *sql.DB, s string) string {
	h, ok := homes.Load(db)
	if !ok {
		return s
	}
	m, _ := paths.AgentKeys(h.(string))
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

// namedAll is named over every string in a decoded reply.
func namedAll(db *sql.DB, v any) any {
	switch x := v.(type) {
	case string:
		return named(db, x)
	case map[string]any:
		for k, e := range x {
			x[k] = namedAll(db, e)
		}
	case []any:
		for i, e := range x {
			x[i] = namedAll(db, e)
		}
	}
	return v
}
