// Package reads serves the daemon's read methods from the same sqlite database the Python daemon uses:
// daemon.status, agents.list, agent.get, agent.timeline, tasks.list, task.get and inbox.list. Nothing here writes.
// Each method follows its Python one (handline/daemon/rpc_status.py, rpc_agents.py, rpc_taskview.py, rpc_inbox.py)
// and the rows they read; a test builds a database with the Python migrations' schema and holds the answers to them.
package reads

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/handline/handline/go/accounts/weblogin"
	"github.com/handline/handline/go/kit/paths"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/procload"
	"github.com/handline/handline/go/scheduler/sysres"
)

const (
	defaultLimit = 50
	maxLimit     = 200
)

// Env is what the methods read: the open database, the handline home, when this daemon started.
type Env struct {
	DB      *sql.DB
	Home    string
	Started time.Time
	Sha     string
	Schema  int
	// Context is the model id and the context figures of an agent on an account, as the usage reads give them: the status file's percent,
	// tokens used (nil when unknown or stale) and the limit tokens are judged against (0 when unknown). agent.strip leaves them null then.
	Context func(agent, account string) (model string, pct *int, used *int64, limit int)
	// Login is an account's latest web login, nil for none; nil is no logins.
	Login func(account string) *weblogin.Login
	// Delivery is what the supervisor knows of why a queued line waits; nil leaves out the reasons that need it.
	Delivery Delivery
	// LoginState is whether an account has credentials, known false when this process cannot tell; nil asks the machine
	// (team.Accounts).
	LoginState func(account string) (loggedIn, known bool)
	// PullRequests answers which of the numbers are pull requests of a GitHub repo; nil asks gh.
	PullRequests func(ctx context.Context, repo string, numbers []int) ([]int, error)
	// Issues is the two newest open issues of the GitHub repo of a git folder, as {number, title}; nil asks gh.
	Issues func(ctx context.Context, dir string) ([]any, error)
	// Resources is the latest measure of what the machine and each agent use (sysres.Sampler.Latest); nil serves none.
	Resources func() map[string]any
	// Load is the latest sample of every process and whose it is (procload.Tracker.View); LoadSeries is each agent's minutes since a time.
	// nil serves none.
	Load       func() (procload.View, bool)
	LoadSeries func(from int64) (map[string][]procload.Point, error)
	// Machine is the latest measure of the machine alone (sysres.Sampler.LatestMachine), for whether new starts are held; nil says none.
	Machine func() *sysres.Machine
	// Claude is the claude binary agents are run with, for DaemonStatus.missing_tools; empty when the daemon runs no agent side.
	Claude string
	// HealthClock keeps since when DaemonStatus.health has had its level, shared by every server of the daemon; nil is since the start.
	HealthClock *HealthClock
	// KeepAwake is whether the daemon holds the sleep lock now, and since when (DaemonStatus.keep_awake); nil reads as not held.
	KeepAwake func() (bool, time.Time)
	// AccountWall is when an account's work stops and starts again, for a task's expected start and end: blockedUntil, the reset that
	// lifts the limit it is at now (0 when it is not); runsOutAt, when it reaches a limit at the current burn (0 when it does not or
	// is not known); resetsAt, when that limit lifts. nil reads the usage files.
	AccountWall func(account string, now int64) (blockedUntil, runsOutAt, resetsAt int64)
	// BranchPR answers the pull request of a branch of a GitHub repo as worktrees.list gives it (number, url, verdict, failing), nil for
	// none; nil asks gh.
	BranchPR  func(ctx context.Context, repo, branch string) (map[string]any, error)
	prs       *prCache
	branchPRs *branchPRs
	samples   *usage.SampleCache // the outlook's usage_samples, held between reads
}

// Register adds the read methods to the server's table. Each is a Read, so a slow one does not hold up a connection's next calls.
func Register(s *rpc.Server, env Env) {
	e := &env
	e.prs = newPRCache()
	e.samples = &usage.SampleCache{}
	e.branchPRs = newBranchPRs()
	s.Register("load.get", rpc.Method{Handler: e.loadGet, Read: true})
	s.Register("daemon.status", rpc.Method{Handler: e.status, Read: true})
	s.Register("agents.list", rpc.Method{Handler: e.agentsList, Read: true})
	s.Register("agent.get", rpc.Method{Handler: e.agentGet, Read: true})
	s.Register("subagents.counts", rpc.Method{Handler: e.subagentCounts, Read: true})
	s.Register("pr.exists", rpc.Method{Handler: e.prExists, Read: true})
	s.Register("agent.strip", rpc.Method{Handler: e.agentStrip, Read: true})
	s.Register("agent.timeline", rpc.Method{Handler: e.agentTimeline, Read: true})
	s.Register("tasks.list", rpc.Method{Handler: e.tasksList, Read: true})
	s.Register("requests.list", rpc.Method{Handler: e.requestsList, Read: true})
	s.Register("for_you.list", rpc.Method{Handler: e.forYouList, Read: true})
	s.Register("requests.pending", rpc.Method{Handler: e.requestsPending, Read: true})
	s.Register("task.get", rpc.Method{Handler: e.taskGet, Read: true})
	s.Register("task.board", rpc.Method{Handler: e.taskBoard, Read: true})
	s.Register("memory.status", rpc.Method{Handler: e.memoryStatus, Read: true})
	s.Register("task.view.list", rpc.Method{Handler: e.taskViews, Read: true})
	s.Register("task.history", rpc.Method{Handler: e.taskHistory, Read: true})
	s.Register("task.time", rpc.Method{Handler: e.taskTime, Read: true})
	s.Register("task.comment.list", rpc.Method{Handler: e.taskMemoList, Read: true})
	s.Register("task.memo.list", rpc.Method{Handler: e.taskMemoList, Read: true}) // the old name
	s.Register("events.feed", rpc.Method{Handler: e.eventsFeed, Read: true})
	s.Register("prompt.list", rpc.Method{Handler: e.promptList, Read: true})
	s.Register("settings.get", rpc.Method{Verbatim: true, Handler: e.settingsGet, Read: true})
	s.Register("answers.list", rpc.Method{Handler: e.answersList, Read: true})
	s.Register("inbox.get", rpc.Method{Handler: e.inboxGet, Waits: true})
	s.Register("request.read", rpc.Method{Handler: e.requestRead, Waits: true})
	s.Register("chat.messages", rpc.Method{Handler: e.chatMessages, Read: true})
	s.Register("chat.requests", rpc.Method{Handler: e.chatRequests, Read: true})
	s.Register("chat.transcript", rpc.Method{Verbatim: true, Handler: e.chatTranscript, Read: true})
	s.Register("chat.locate", rpc.Method{Handler: e.chatLocate, Read: true})
	s.Register("chat.refs", rpc.Method{Handler: e.chatRefs, Read: true})
	s.Register("chat.search", rpc.Method{Handler: e.chatSearch, Read: true})
	s.Register("proposal.list", rpc.Method{Handler: e.proposalList, Read: true})
	s.Register("repos.list", rpc.Method{Handler: e.reposList, Read: true})
	s.Register("repos.recent", rpc.Method{Handler: e.reposRecent, Read: true})
	s.Register("repo.facts", rpc.Method{Handler: e.repoFacts, Read: true})
	s.Register("worktrees.list", rpc.Method{Handler: e.worktreesList, Read: true})
	s.Register("worktree.diff", rpc.Method{Verbatim: true, Handler: e.worktreeDiff, Read: true})
	s.Register("agent.options", rpc.Method{Handler: e.agentOptions, Read: true})
	s.Register("operation.get", rpc.Method{Handler: e.operationGet, Read: true})
	s.Register("accounts.list", rpc.Method{Handler: e.accountsList, Read: true})
	s.Register("status.pick", rpc.Method{Handler: e.statusPick, Read: true})
	s.Register("stats.get", rpc.Method{Handler: e.statsGet, Read: true})
	s.Register("stats.outlook", rpc.Method{Handler: e.statsOutlook, Read: true})
	s.Register("stats.cost", rpc.Method{Handler: e.statsCost, Read: true})
	s.Register("stats.retro", rpc.Method{Handler: e.statsRetro, Read: true})
	s.Register("team.get", rpc.Method{Handler: e.teamGet, Read: true})
	s.Register("people.list", rpc.Method{Handler: e.peopleList, Read: true})
	s.Register("workspaces.list", rpc.Method{Handler: e.workspacesList, Read: true})
	s.Register("workspace.rules", rpc.Method{Handler: e.workspaceRules, Read: true})
	s.Register("agent.triggers", rpc.Method{Handler: e.agentTriggers, Read: true})
	s.Register("schedules.list", rpc.Method{Handler: e.schedulesList, Read: true})
	s.Register("inbox.list", rpc.Method{Handler: e.inboxList, Waits: true})
	s.Register("inbox.queue", rpc.Method{Handler: e.inboxQueue, Waits: true})
}

// row is one result row by column name, the way sqlite3.Row reads in the Python daemon.
type row map[string]any

func query(db db.Q, q string, args ...any) ([]row, error) {
	rs, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	cols, err := rs.Columns()
	if err != nil {
		return nil, err
	}
	var out []row
	for rs.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return nil, err
		}
		r := row{}
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				vals[i] = string(b)
			}
			r[c] = vals[i]
		}
		out = append(out, r)
	}
	return out, rs.Err()
}

func queryOne(db *sql.DB, q string, args ...any) (row, error) {
	rs, err := query(db, q, args...)
	if err != nil || len(rs) == 0 {
		return nil, err
	}
	return rs[0], nil
}

func (r row) set(k string) bool { return r != nil && r[k] != nil }

func (r row) str(k string) string {
	if v, ok := r[k].(string); ok {
		return v
	}
	return ""
}

// strOrNil is the column as JSON has it: a string, or null.
func (r row) strOrNil(k string) any {
	if v, ok := r[k].(string); ok {
		return v
	}
	return nil
}

func (r row) i(k string) int64 { return num(r[k]) }

// num is a sqlite number as a whole one; 0 for null.
func num(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	}
	return 0
}

func (r row) intOrNil(k string) any {
	if r[k] == nil {
		return nil
	}
	return r.i(k)
}

// ts is the spec's Timestamp: whole seconds, UTC; null for a column that is null.
func ts(v any) any {
	switch n := v.(type) {
	case int64:
		return time.Unix(n, 0).UTC().Format("2006-01-02T15:04:05Z")
	case float64:
		return time.Unix(int64(n), 0).UTC().Format("2006-01-02T15:04:05Z")
	}
	return nil
}

// Param readers: a wrong type is a bad_request naming the field, as the Python methods' helpers do.

func badParam(msg, field string) error { return rpc.BadRequestError(msg, field, rpc.InvalidParams) }

func optInt(params map[string]any, key string) (*int64, error) {
	v, has := params[key]
	if !has || v == nil {
		return nil, nil
	}
	n, ok := rpc.AsInt(v)
	if !ok {
		return nil, badParam(key+" is an integer", key)
	}
	return &n, nil
}

func optStr(params map[string]any, key string) (*string, error) {
	v, has := params[key]
	if !has || v == nil {
		return nil, nil
	}
	s, ok := v.(string)
	if !ok {
		return nil, badParam(key+" is a string", key)
	}
	return &s, nil
}

// optTags is a list of tags under key, lowercased as a task's are; nil when absent.
func optTags(params map[string]any, key string) ([]string, error) {
	v, has := params[key]
	if !has || v == nil {
		return nil, nil
	}
	items, ok := v.([]any)
	if !ok {
		return nil, badParam(key+" is a list of tags", key)
	}
	out := make([]string, len(items))
	for i, it := range items {
		s, ok := it.(string)
		if !ok {
			return nil, badParam(key+" is a list of tags", key)
		}
		out[i] = strings.ToLower(strings.TrimSpace(s))
	}
	return out, nil
}

func optBool(params map[string]any, key string) (bool, error) {
	v, has := params[key]
	if !has {
		return false, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, badParam(key+" is true or false", key)
	}
	return b, nil
}

// pageLimit is limit from 1 to maxLimit, defaultLimit when absent.
func pageLimit(params map[string]any) (int, error) {
	n, err := optInt(params, "limit")
	if err != nil {
		return 0, badParam(fmt.Sprintf("limit is an integer from 1 to %d", maxLimit), "limit")
	}
	if n == nil {
		return defaultLimit, nil
	}
	if *n < 1 || *n > maxLimit {
		return 0, badParam(fmt.Sprintf("limit is an integer from 1 to %d", maxLimit), "limit")
	}
	return int(*n), nil
}

// listAgents is the registered agents.
func (e *Env) listAgents() []string { return paths.ListAgents(e.Home) }

// oneline is a value as one line of printable text.
func oneline(s string) string {
	s = strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\v' || r == '\f' || r == 0x85 || r == 0x2028 || r == 0x2029
	}), " ")
	return strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return ' '
	}, s)
}

// head is s cut to n characters.
func head(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func decodeObject(s string) map[string]any {
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	var m map[string]any
	if d.Decode(&m) != nil || m == nil {
		return map[string]any{}
	}
	return m
}

// loginState is an account's login as the daemon's login check answers it.
func (e *Env) loginState(account string) (loggedIn, known bool) {
	if e.LoginState != nil {
		return e.LoginState(account)
	}
	return team.Accounts{Home: e.Home}.LoginState(account)
}
