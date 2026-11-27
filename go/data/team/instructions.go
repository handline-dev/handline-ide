package team

import (
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/paths"
)

// InstructionParts is the part each name a person knows an instruction by is kept as.
var InstructionParts = map[string]string{"ROLE.md": db.PartRole, "base.md": db.PartBase, "rules": db.PartRules}

// AgentInstruction is the agent's part in force (db.PartRole, db.PartBase), trimmed; who is its key or name. "" for none.
func AgentInstruction(q db.Q, who, part string) (string, error) {
	key, err := paths.AgentKeyIn(q, who)
	if key == "" || err != nil {
		return "", err
	}
	if key, err = paths.KnowsAs(q, key); err != nil { // a temp's role is the agent's it was hired from
		return "", err
	}
	text, err := db.InstructionText(q, key, part)
	return strings.TrimSpace(text), err
}
