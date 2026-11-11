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
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/protocol/rpc"
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

var tokenWord = regexp.MustCompile(`<@([^>]+)>`)

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
	s = tokenWord.ReplaceAllString(s, "$1") // <@key>, as a sentence names a who
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

// toldTo is who the requests whose body, read by name, starts with prefix went to: their names, sorted, space-separated.
func toldTo(t *testing.T, db *sql.DB, prefix string) string {
	t.Helper()
	rows, err := db.Query("SELECT target, body FROM requests")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var to []string
	for rows.Next() {
		var target, body string
		rows.Scan(&target, &body)
		if strings.HasPrefix(named(db, body), prefix) {
			to = append(to, named(db, target))
		}
	}
	sort.Strings(to)
	return strings.Join(to, " ")
}

// rigServer serves the task methods over d to agent (the founder for "") as the daemon would: who params resolved to keys, the
// caller by its key, and a fake teller's records read by name. d must be registered in homes.
func rigServer(t testing.TB, d *sql.DB, home, agent string, tell Teller) *rpc.Server {
	s := rpc.NewServer(auth{as(t, home, agent)}, "x", store.LatestVersion(), "local", func(string, ...any) {})
	s.DB = d
	s.Names = teamtest.DB(t, home)
	if f, ok := tell.(*fakeTeller); ok {
		f.db = d
	}
	Register(s, home, tell, nil)
	return s
}
