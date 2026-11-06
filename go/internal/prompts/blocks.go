// Package prompts is the one prompt an agent's screen shows, kept as a row the bridge reads and answers
// (handline/actions/prompt_blocks.py, dialogs.py, confirmations.py, ask_question.py, daemon/prompts.py, daemon/rpc_prompts.py).
//
// The screen watch judges each agent's screen once and keeps a prompt_blocks row for what it shows: a permission prompt, an
// AskUserQuestion, or a plain dialog, never two at once, in that priority. Answering presses keys only for what a fresh read of
// the screen shows now: a prompt whose panel text, or (for a permission prompt) hook wait id, no longer matches the row is refused.
package prompts

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/internal/reads"
	"github.com/handline/handline/go/protocol/rpc"
)

// StaleS is how old a row's last_seen may be before it no longer counts as a prompt on screen: a watch that stopped, or a screen
// it cannot read, must not leave a prompt showing for good.
const StaleS = 10

// Env is what the prompt code works on.
type Env struct {
	DB    *sql.DB
	Home  string
	Now   func() time.Time
	Trust *team.Trust
	// Reads draws the Prompt a stale answer carries.
	Reads *reads.Env
	// Refresh is how often a waiting hook call marks its question seen and looks for its agent having moved on; zero is StaleS/3 seconds.
	Refresh time.Duration

	hooks hookWaits
}

func (e *Env) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// Block is a prompt_blocks row, panel and detail decoded.
type Block struct {
	ID                  int64
	Agent, Kind         string
	Fingerprint, Title  string
	Panel               []string
	Detail              map[string]any
	WaitID              sql.NullInt64
	Answerable          bool
	FirstSeen, LastSeen int64
	GoneAt              sql.NullInt64
}

const blockCols = "id, agent, kind, fingerprint, title, panel, detail, wait_id, answerable, first_seen, last_seen, gone_at"

func scanBlock(r interface{ Scan(...any) error }) (*Block, error) {
	var b Block
	var panel, detail string
	var answerable int64
	if err := r.Scan(&b.ID, &b.Agent, &b.Kind, &b.Fingerprint, &b.Title, &panel, &detail, &b.WaitID, &answerable,
		&b.FirstSeen, &b.LastSeen, &b.GoneAt); err != nil {
		return nil, err
	}
	b.Answerable = answerable != 0
	json.Unmarshal([]byte(panel), &b.Panel)
	json.Unmarshal([]byte(detail), &b.Detail)
	if b.Panel == nil {
		b.Panel = []string{}
	}
	if b.Detail == nil {
		b.Detail = map[string]any{}
	}
	return &b, nil
}

// OpenOf is agent's open row (db.open_prompt), refreshed or not; nil when there is none.
func (e *Env) OpenOf(agent string) (*Block, error) {
	b, err := scanBlock(e.DB.QueryRow("SELECT "+blockCols+" FROM prompt_blocks WHERE agent = ? AND gone_at IS NULL", agent))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return b, err
}

// OpenBlockOf is agent's open block, when the watch has refreshed it within StaleS.
func (e *Env) OpenBlockOf(agent string) (*Block, error) {
	b, err := e.OpenOf(agent)
	if err != nil || b == nil || e.now().Unix()-b.LastSeen > StaleS {
		return nil, err
	}
	return b, nil
}

func (e *Env) get(id int64) (*Block, error) {
	b, err := scanBlock(e.DB.QueryRow("SELECT "+blockCols+" FROM prompt_blocks WHERE id = ?", id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return b, err
}

// Fingerprint is the identity of what a screen block says: its kind and panel text.
func Fingerprint(kind string, panel []string) string {
	sum := sha1.Sum([]byte(strings.Join(append([]string{kind}, panel...), "\n")))
	return hex.EncodeToString(sum[:])
}

// vendorOf is the vendor an agent runs under; an unreadable agent file is not a dialog, and reads as the default.
func (e *Env) vendorOf(agent string) string {
	cfg, err := team.NewRoster(e.Home).Read(agent)
	if err != nil {
		return team.DefaultVendor
	}
	if v, _ := cfg["vendor"].(string); v != "" {
		return v
	}
	return team.DefaultVendor
}

// StaleError is the refusal for an answer to a prompt that is not the one open now; current is that one, or nil.
func StaleError(current any) *rpc.Error {
	return &rpc.Error{RPCCode: -32003, Code: "stale_prompt", Message: "the prompt on screen is not the one answered; nothing was sent",
		Data: map[string]any{"current": current}}
}

// New is the prompt code of one daemon.
func New(db *sql.DB, home string, rd *reads.Env) *Env {
	return &Env{DB: db, Home: home, Trust: team.NewTrust(home), Reads: rd}
}
