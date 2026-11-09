// Package cmdruns keeps the runs of a message's or an agent reply's command blocks (shell.open with a command and a block): one row of the meta table per block, so
// no migration. A run starts "running"; Settle ends it from what the terminal host says of its window.
package cmdruns

import (
	"database/sql"
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/data/db"
)

const prefix = "run:"

// keep is how long a run row stays after it ends.
const keep = 30 * 24 * 3600

var blockRE = regexp.MustCompile(`^(r?)([0-9]{1,18}):([0-9]{1,9})$`)

// Block is a parsed "<message id>:<n>", or "r<item id>:<n>" for the nth block of an agent's reply item (Reply true, Message the item id).
type Block struct {
	Message, N int64
	Reply      bool
}

// ParseBlock reads "<message id>:<n>" or "r<item id>:<n>"; ok is false for any other text.
func ParseBlock(s string) (Block, bool) {
	m := blockRE.FindStringSubmatch(s)
	if m == nil {
		return Block{}, false
	}
	msg, _ := strconv.ParseInt(m[2], 10, 64)
	n, _ := strconv.ParseInt(m[3], 10, 64)
	return Block{Message: msg, N: n, Reply: m[1] == "r"}, true
}

func (b Block) String() string { return b.owner() + ":" + strconv.FormatInt(b.N, 10) }

// owner is the message id, or r and the reply item's id.
func (b Block) owner() string {
	if b.Reply {
		return "r" + strconv.FormatInt(b.Message, 10)
	}
	return strconv.FormatInt(b.Message, 10)
}

// Run is one run as kept: State is running, done, failed or stopped; Exit is set once it is done or failed. Agent is the agent whose chat
// it ran from, the one told how it ended; a row kept before it was recorded has none.
type Run struct {
	Block    string `json:"block"`
	Terminal string `json:"terminal"`
	Agent    string `json:"agent,omitempty"`
	Command  string `json:"command,omitempty"`
	Started  int64  `json:"started"`
	Ended    int64  `json:"ended,omitempty"`
	State    string `json:"state"`
	Exit     *int   `json:"exit,omitempty"`
}

// Tag is what the window carries to tell it from a later pane with the same id.
func (r Run) Tag() string { return r.Block + "@" + strconv.FormatInt(r.Started, 10) }

// Seconds is how long it ran; -1 while it has not ended or when it ended unread.
func (r Run) Seconds() int64 {
	if r.Exit == nil {
		return -1
	}
	return max(r.Ended-r.Started, 0)
}

// Start records agent's run of command for block in terminal, replacing the one the block had.
func Start(q db.Q, b Block, terminal, agent, command string, started int64) (Run, error) {
	r := Run{Block: b.String(), Terminal: terminal, Agent: agent, Command: command, Started: started, State: "running"}
	enc, _ := json.Marshal(r)
	_, err := q.Exec("INSERT INTO meta(name, value, updated) VALUES(?, ?, ?) ON CONFLICT(name) DO UPDATE SET value=excluded.value, updated=excluded.updated",
		prefix+r.Block, string(enc), started)
	return r, err
}

func decode(name, value string) (Run, bool) {
	var r Run
	if json.Unmarshal([]byte(value), &r) != nil || r.Block != strings.TrimPrefix(name, prefix) {
		return Run{}, false
	}
	return r, true
}

func read(q db.Q, where string, args ...any) ([]Run, error) {
	rows, err := q.Query("SELECT name, COALESCE(value, '') FROM meta WHERE name LIKE ? ORDER BY name", append([]any{where}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			return nil, err
		}
		if r, ok := decode(name, value); ok {
			out = append(out, r)
		}
	}
	return out, rows.Err()
}

// All is every kept run.
func All(q db.Q) ([]Run, error) { return read(q, prefix+"%") }

// Of is the runs of message id, oldest start first.
func Of(q db.Q, id int64) ([]Run, error) { return OfBlocks(q, Block{Message: id}) }

// OfReply is the runs of reply item id, oldest start first.
func OfReply(q db.Q, id int64) ([]Run, error) { return OfBlocks(q, Block{Message: id, Reply: true}) }

// OfBlocks is the runs of every block of b's message or reply item, oldest start first.
func OfBlocks(q db.Q, b Block) ([]Run, error) {
	out, err := read(q, prefix+b.owner()+":%")
	sort.SliceStable(out, func(i, j int) bool { return out[i].Started < out[j].Started })
	return out, err
}

// Replies is the runs of every reply item that has one, by item id, each oldest start first.
func Replies(q db.Q) (map[int64][]Run, error) {
	all, err := read(q, prefix+"r%")
	out := map[int64][]Run{}
	for _, r := range all {
		if b, ok := ParseBlock(r.Block); ok { // the LIKE has kept only reply blocks
			out[b.Message] = append(out[b.Message], r)
		}
	}
	for _, runs := range out {
		sort.SliceStable(runs, func(i, j int) bool { return runs[i].Started < runs[j].Started })
	}
	return out, err
}

// Settle ends every running run the host's answer shows over: its window is dead (exit status and end time are tmux's own), or gone, or is
// another pane now (stopped). panes nil is "could not ask": nothing ends. Rows that ended long ago are dropped. Each run it ends is handed
// to ended in the transaction that ends it, so what ended writes stands or falls with the run's end; a run started again since it was
// read is left alone.
func Settle(d *sql.DB, panes map[string]term.PaneRun, now int64, ended func(q db.Q, r Run) error) error {
	runs, err := All(d)
	if err != nil {
		return err
	}
	for _, r := range runs {
		if r.State != "running" {
			if r.Ended > 0 && now-r.Ended > keep {
				if _, err := d.Exec("DELETE FROM meta WHERE name = ?", prefix+r.Block); err != nil {
					return err
				}
			}
			continue
		}
		if panes == nil {
			continue
		}
		p, here := panes[r.Terminal]
		switch {
		case !here || p.Tag != r.Tag():
			r.State, r.Ended = "stopped", now
		case p.Dead:
			st := p.Status
			r.Exit, r.Ended = &st, p.DeadTime
			if r.Ended == 0 {
				r.Ended = now
			}
			r.State = "done"
			if st != 0 {
				r.State = "failed"
			}
		default:
			continue
		}
		if err := end(d, r, now, ended); err != nil {
			return err
		}
	}
	return nil
}

// end writes r's end, with what ended adds, if its row is still the running run r was read as. ended goes first, so a host it reads
// (a window's last rows) is read before the transaction holds the write lock.
func end(d *sql.DB, r Run, now int64, ended func(db.Q, Run) error) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if ended != nil {
		if err := ended(tx, r); err != nil {
			return err
		}
	}
	enc, _ := json.Marshal(r)
	res, err := tx.Exec("UPDATE meta SET value = ?, updated = ? WHERE name = ? AND json_extract(value, '$.state') = 'running' AND json_extract(value, '$.started') = ?",
		string(enc), now, prefix+r.Block, r.Started)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return err // started again since it was read: that run is not over, and nothing is told
	}
	return tx.Commit()
}

// Now is the clock the daemon stamps a run with.
func Now() int64 { return time.Now().Unix() }
