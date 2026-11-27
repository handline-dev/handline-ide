package chatwrites

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/handline/handline/go/agentctl/delivery"
)

// sendOp is one send as the fake session's ledger keeps it.
type sendOp struct {
	ID       string
	UUID     string
	Text     string
	Delivery delivery.Delivery
	State    delivery.State
}

// sendLedger keeps a fake session's sends in the operations table, where the daemon's readers look: the id is
// "send:<agent>:<op>", step is the delivery, message the text and result_json the message uuid.
type sendLedger struct {
	DB    *sql.DB
	Agent string
}

const sendKind = "send"

func (l *sendLedger) key(op string) string { return sendKind + ":" + l.Agent + ":" + op }

func (l *sendLedger) Get(op string) (*sendOp, error) {
	return scanOp(l.DB.QueryRow("SELECT id, state, step, message, result_json FROM operations WHERE id = ? AND kind = ?", l.key(op), sendKind), l.prefix())
}

func (l *sendLedger) prefix() string { return sendKind + ":" + l.Agent + ":" }

func scanOp(r interface{ Scan(...any) error }, prefix string) (*sendOp, error) {
	var id, state, step, msg, res sql.NullString
	if err := r.Scan(&id, &state, &step, &msg, &res); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var r2 struct {
		UUID string `json:"uuid"`
	}
	json.Unmarshal([]byte(res.String), &r2)
	return &sendOp{ID: id.String[len(prefix):], UUID: r2.UUID, Text: msg.String, Delivery: delivery.Delivery(step.String), State: delivery.State(state.String)}, nil
}

func (l *sendLedger) Put(o sendOp) error {
	res, _ := json.Marshal(map[string]string{"uuid": o.UUID})
	now := time.Now().Unix()
	_, err := l.DB.Exec("INSERT INTO operations(id, kind, state, step, message, result_json, agent, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?,?) "+
		"ON CONFLICT(id) DO UPDATE SET state = excluded.state, step = excluded.step, message = excluded.message, result_json = excluded.result_json, updated_at = excluded.updated_at",
		l.key(o.ID), sendKind, string(o.State), string(o.Delivery), o.Text, string(res), l.Agent, now, now)
	return err
}
