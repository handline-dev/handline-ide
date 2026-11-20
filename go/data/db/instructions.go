package db

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/handline/handline/go/kit/actor"
)

// The instructions a launch gives an agent that people write: its role and standing context, and its workspace's rules. Each is kept as
// versions in the instructions table, newest in force, with who wrote it and who OK'd it; nothing edits them on disk.
const (
	PartRole  = "role"  // an agent's role (was ROLE.md)
	PartBase  = "base"  // an agent's standing context (was base.md)
	PartRules = "rules" // a workspace's rules (was its base.md)
)

// Instruction is one version of an instruction.
type Instruction struct {
	ID                                 int64
	Owner, Part, Text, Actor, Approver string
	At                                 int64
}

const instructionCols = "id, owner, part, text, actor, approver, at"

func scanInstruction(s interface{ Scan(...any) error }) (Instruction, error) {
	var in Instruction
	err := s.Scan(&in.ID, &in.Owner, &in.Part, &in.Text, &in.Actor, &in.Approver, &in.At)
	return in, err
}

// CurrentInstruction is the version of owner's part in force, nil when it has none.
func CurrentInstruction(q Q, owner, part string) (*Instruction, error) {
	in, err := scanInstruction(q.QueryRow("SELECT "+instructionCols+" FROM instructions WHERE owner = ? AND part = ? ORDER BY id DESC LIMIT 1",
		owner, part))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &in, nil
}

// InstructionText is the text of owner's part in force, "" for none.
func InstructionText(q Q, owner, part string) (string, error) {
	in, err := CurrentInstruction(q, owner, part)
	if in == nil || err != nil {
		return "", err
	}
	return in.Text, nil
}

// SetInstruction keeps text as owner's part in force, written by actor and OK'd by approver at at. Text the same as the version in force
// (ignoring the space around it) writes nothing, and says so.
func SetInstruction(q Q, owner, part, text, actor, approver string, at int64) (bool, error) {
	was, err := CurrentInstruction(q, owner, part)
	if err != nil {
		return false, err
	}
	text = strings.TrimSpace(text)
	if was != nil && strings.TrimSpace(was.Text) == text {
		return false, nil
	}
	_, err = q.Exec("INSERT INTO instructions(owner, part, text, actor, approver, at) VALUES (?, ?, ?, ?, ?, ?)",
		owner, part, text, actor, approver, at)
	return err == nil, err
}

// InstructionsAfter is owner's part's versions after the one with id after, oldest first: with the newest id a session launched with, the
// changes it has not seen; with 0, the whole history.
func InstructionsAfter(q Q, owner, part string, after int64) ([]Instruction, error) {
	rows, err := q.Query("SELECT "+instructionCols+" FROM instructions WHERE owner = ? AND part = ? AND id > ? ORDER BY id", owner, part, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Instruction
	for rows.Next() {
		in, err := scanInstruction(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

// LastInstructionID is the newest version's id of any instruction written before at (all of them for at 0), 0 for none: what a launch
// then had seen.
func LastInstructionID(q Q, at int64) (int64, error) {
	var id sql.NullInt64
	query, args := "SELECT MAX(id) FROM instructions", []any{}
	if at > 0 {
		query, args = query+" WHERE at < ?", []any{at}
	}
	err := q.QueryRow(query, args...).Scan(&id)
	return id.Int64, err
}

// InstructionOwner is the key that keeps agent's part: agent itself, or for rules its workspace.
func InstructionOwner(q Q, agent, part string) (string, error) {
	if part != PartRules {
		return agent, nil
	}
	_, id, _ := actor.Parse(agent)
	var ws int64
	if err := q.QueryRow("SELECT workspace FROM agents WHERE id = ?", id).Scan(&ws); err != nil {
		return "", err
	}
	return actor.Key(actor.Workspace, ws), nil
}
