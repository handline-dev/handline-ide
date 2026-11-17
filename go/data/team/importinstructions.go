package team

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/paths"
)

// ImportActor is who the first version of an instruction moved in from its file was written by.
const ImportActor = "imported"

// InstructionParts is the part each name a person knows an instruction by is kept as.
var InstructionParts = map[string]string{"ROLE.md": db.PartRole, "base.md": db.PartBase, "rules": db.PartRules}

// ImportInstructions moves each agent's ROLE.md and base.md and each workspace's base.md into the instructions table as a version by
// ImportActor at the file's time, then removes the file; a seeded workspace with no rules file gets empty rules. A file that cannot be
// read or kept stays, named in errs.
func ImportInstructions(home string, q db.Q) (moved []string, errs []error) {
	type file struct{ owner, part, path string }
	var files []file
	agents, _ := filepath.Glob(filepath.Join(home, "agents", "*"))
	for _, d := range agents {
		if k := filepath.Base(d); actor.KindOf(k) == actor.Agent {
			files = append(files, file{k, db.PartRole, filepath.Join(d, "ROLE.md")}, file{k, db.PartBase, filepath.Join(d, "base.md")})
		}
	}
	wss, _ := filepath.Glob(filepath.Join(home, "workspaces", "*"))
	for _, d := range wss {
		if k := filepath.Base(d); actor.KindOf(k) == actor.Workspace {
			files = append(files, file{k, db.PartRules, filepath.Join(d, "base.md")})
		}
	}
	for _, f := range files {
		ok, err := importOne(q, f.owner, f.part, f.path)
		if err != nil {
			errs = append(errs, err)
		} else if ok {
			moved = append(moved, f.path)
		}
	}
	return moved, errs
}

func importOne(q db.Q, owner, part, path string) (bool, error) {
	st, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		if part == db.PartRules {
			return false, emptiedRules(q, owner, filepath.Dir(path))
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	// a text the version in force already has writes nothing (SetInstruction)
	if _, err := db.SetInstruction(q, owner, part, string(b), ImportActor, "", st.ModTime().Unix()); err != nil {
		return false, err
	}
	return true, os.Remove(path)
}

// emptiedRules keeps empty rules for a workspace seeded once whose rules file is gone: its team removed them.
func emptiedRules(q db.Q, owner, dir string) error {
	st, err := os.Stat(filepath.Join(dir, ".seeded"))
	if err != nil {
		return nil
	}
	was, err := db.CurrentInstruction(q, owner, db.PartRules)
	if err != nil || was != nil {
		return err
	}
	_, err = db.SetInstruction(q, owner, db.PartRules, "", ImportActor, "", st.ModTime().Unix())
	return err
}

// AgentInstruction is the agent's part in force (db.PartRole, db.PartBase), trimmed; who is its key or name. "" for none.
func AgentInstruction(q db.Q, who, part string) (string, error) {
	key, err := paths.AgentKeyIn(q, who)
	if key == "" || err != nil {
		return "", err
	}
	text, err := db.InstructionText(q, key, part)
	return strings.TrimSpace(text), err
}
