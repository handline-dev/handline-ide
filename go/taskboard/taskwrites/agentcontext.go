package taskwrites

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
)

// memoryIndex is the index file of a memory folder: one line per note, linking its file.
const memoryIndex = "MEMORY.md"

// RegisterAgentContext serves what an agent is told and remembers: its role and standing context, which the founder reads and replaces,
// and its memory notes, which the founder reads and removes one of.
func RegisterAgentContext(s *rpc.Server, home string) {
	s.Register("agent.context", rpc.Method{Handler: contextGet, Read: true})
	s.Register("agent.context.set", rpc.Method{Handler: contextSet})
	s.Register("agent.memory", rpc.Method{Handler: memoryGet(home), Read: true})
	s.Register("agent.memory.remove", rpc.Method{Handler: memoryRemove(home)})
}

// knownAgent is the agent a call names, refused as not_found when there is no such agent.
func knownAgent(q db.Q, p map[string]any) (string, error) {
	agent, err := agentParam(p)
	if err != nil {
		return "", err
	}
	key, err := paths.AgentKeyIn(q, agent)
	if err != nil {
		return "", err
	}
	if key == "" {
		e := rpc.NotFoundError("no agent " + agent)
		e.Data = map[string]any{"what": "agent"}
		return "", e
	}
	return agent, nil
}

// contextParts are the parts agent.context reads and agent.context.set writes, by their param name.
var contextParts = []string{db.PartRole, db.PartBase}

func contextGet(c *rpc.Conn, p map[string]any) (any, error) {
	q := c.Server().DB
	agent, err := knownAgent(q, p)
	if err != nil {
		return nil, err
	}
	key, err := paths.KnowsAs(q, agent) // a temp is told the agent's it was hired from
	if err != nil {
		return nil, err
	}
	out := map[string]any{"agent": agent}
	for _, part := range contextParts {
		in, err := db.CurrentInstruction(q, key, part)
		if err != nil {
			return nil, err
		}
		out[part] = nil
		if in != nil {
			out[part] = map[string]any{"text": strings.TrimSpace(in.Text), "by": in.Actor, "at": time.Unix(in.At, 0).UTC().Format(time.RFC3339)}
		}
	}
	return out, nil
}

func contextSet(c *rpc.Conn, p map[string]any) (any, error) {
	agent, err := knownAgent(c.Server().DB, p)
	if err != nil {
		return nil, err
	}
	texts := map[string]string{}
	for _, part := range contextParts {
		v, ok := p[part]
		if !ok || v == nil {
			continue
		}
		s, _ := v.(string)
		if why := db.InstructionRefusal(part, s); why != "" {
			return nil, badParam(why, part)
		}
		texts[part] = s
	}
	if len(texts) == 0 {
		return nil, badParam("name role, base or both, with the whole new text", "role")
	}
	me := c.Caller.Actor()
	return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
		key, err := paths.KnowsAs(tx, agent)
		if err != nil {
			return nil, err
		}
		var changed []string
		now := time.Now().Unix()
		for _, part := range contextParts {
			text, ok := texts[part]
			if !ok {
				continue
			}
			// The founder wrote it, so the founder OK'd it.
			wrote, err := db.SetInstruction(tx, key, part, text, me, me, now)
			if err != nil {
				return nil, err
			}
			if wrote {
				changed = append(changed, part)
			}
		}
		// A temp has no texts of its own: the founder changed the agent's it was hired from, and is told so.
		if len(changed) == 0 {
			return accepted(fmt.Sprintf("%s's text is that already", addr.Ref(key)), nil), nil
		}
		return accepted(fmt.Sprintf("%s: %s replaced; applies when it next starts fresh", addr.Ref(key), strings.Join(changed, " and ")), nil), nil
	})
}

// memoryNote is one note of an agent's memory folder.
type memoryNote struct {
	Name, File, Description, Type, Body string
}

// memoryDir is the folder agent's memory notes are kept in: its own, or a temp's agent's it was hired from.
func memoryDir(q db.Q, home, agent string) (string, error) {
	key, err := paths.KnowsAs(q, agent)
	return filepath.Join(paths.AgentDir(home, key), "memory"), err
}

// parseNote reads a note's frontmatter (name, description, and type at the top or under metadata) and the body after it. A file with
// no frontmatter, or one never closed by a line of ---, is all body, named by its file.
func parseNote(file, text string) memoryNote {
	n := memoryNote{Name: strings.TrimSuffix(file, ".md"), File: file, Body: text}
	lines := strings.Split(strings.ReplaceAll(strings.TrimPrefix(text, "\uFEFF"), "\r\n", "\n"), "\n")
	if len(lines) < 2 || lines[0] != "---" {
		return n
	}
	end := slices.Index(lines[1:], "---") + 1
	if end == 0 {
		return n
	}
	n.Body = strings.TrimLeft(strings.Join(lines[end+1:], "\n"), "\n")
	for _, line := range lines[1:end] {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		top := line == strings.TrimLeft(line, " \t")
		switch {
		case k == "name" && top && v != "":
			n.Name = v
		case k == "description" && (top || n.Description == ""):
			n.Description = v
		case k == "type":
			n.Type = v
		}
	}
	return n
}

// indexLinks are the files MEMORY.md links, in its order.
func indexLinks(index string) []string {
	var out []string
	for _, line := range strings.Split(index, "\n") {
		if f := indexLink(line); f != "" && !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}

// indexLink is the file a MEMORY.md line links (`[title](file.md)`), "" for none.
func indexLink(line string) string {
	_, after, ok := strings.Cut(line, "](")
	if !ok {
		return ""
	}
	f, _, ok := strings.Cut(after, ")")
	f = strings.TrimPrefix(f, "./")
	if !ok || !strings.HasSuffix(f, ".md") || strings.ContainsAny(f, "/\\") {
		return "" // a link out of the folder is not one of its notes
	}
	return f
}

// memoryNotes are the notes in dir, in MEMORY.md's order, then the rest by file name; none for a folder not made yet.
func memoryNotes(dir string) ([]memoryNote, error) {
	ents, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range ents {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".md") && e.Name() != memoryIndex && !strings.HasPrefix(e.Name(), ".") {
			files = append(files, e.Name())
		}
	}
	index, err := os.ReadFile(filepath.Join(dir, memoryIndex))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	order := indexLinks(string(index))
	rank := func(f string) int {
		if i := slices.Index(order, f); i >= 0 {
			return i
		}
		return len(order)
	}
	slices.SortStableFunc(files, func(a, b string) int {
		if d := rank(a) - rank(b); d != 0 {
			return d
		}
		return strings.Compare(a, b)
	})
	notes := make([]memoryNote, 0, len(files))
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			return nil, err
		}
		notes = append(notes, parseNote(f, string(b)))
	}
	return notes, nil
}

func memoryGet(home string) func(*rpc.Conn, map[string]any) (any, error) {
	return func(c *rpc.Conn, p map[string]any) (any, error) {
		agent, err := knownAgent(c.Server().DB, p)
		if err != nil {
			return nil, err
		}
		dir, err := memoryDir(c.Server().DB, home, agent)
		if err != nil {
			return nil, err
		}
		notes, err := memoryNotes(dir)
		if err != nil {
			return nil, err
		}
		out := make([]any, 0, len(notes))
		for _, n := range notes {
			out = append(out, map[string]any{"name": n.Name, "file": n.File, "description": n.Description, "type": n.Type, "body": n.Body})
		}
		return map[string]any{"agent": agent, "notes": out}, nil
	}
}

func memoryRemove(home string) func(*rpc.Conn, map[string]any) (any, error) {
	return func(c *rpc.Conn, p map[string]any) (any, error) {
		agent, err := knownAgent(c.Server().DB, p)
		if err != nil {
			return nil, err
		}
		name, _ := p["name"].(string)
		if name == "" {
			return nil, badParam("name is a note's name as agent.memory gives it", "name")
		}
		return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			dir, err := memoryDir(tx, home, agent)
			if err != nil {
				return nil, err
			}
			notes, err := memoryNotes(dir)
			if err != nil {
				return nil, err
			}
			// A name two notes share names neither; each one's file name still names it alone.
			var hit []memoryNote
			for _, n := range notes {
				if n.Name == name || n.File == name {
					hit = append(hit, n)
				}
			}
			switch {
			case len(hit) == 0:
				e := rpc.NotFoundError(fmt.Sprintf("%s has no memory note %q", addr.Ref(agent), name))
				e.Data = map[string]any{"what": "memory"}
				return nil, e
			case len(hit) > 1 && !slices.ContainsFunc(hit, func(n memoryNote) bool { return n.File == name }):
				return nil, badParam(fmt.Sprintf("%d notes are named %q: name one by its file", len(hit), name), "name")
			case len(hit) > 1:
				hit = slices.DeleteFunc(hit, func(n memoryNote) bool { return n.File != name })
			}
			// The file goes first: a removal cut short leaves at most an index line linking nothing, never a note the index hides.
			if err := os.Remove(filepath.Join(dir, hit[0].File)); err != nil {
				return nil, err
			}
			if err := dropIndexLine(dir, hit[0].File); err != nil {
				return nil, err
			}
			return accepted(fmt.Sprintf("%s's memory note %s removed", addr.Ref(agent), hit[0].Name), nil), nil
		})
	}
}

// dropIndexLine takes the lines linking file out of dir's MEMORY.md, every other byte left as it was.
func dropIndexLine(dir, file string) error {
	path := filepath.Join(dir, memoryIndex)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	lines := strings.SplitAfter(string(b), "\n")
	all := len(lines)
	kept := slices.DeleteFunc(lines, func(l string) bool { return indexLink(l) == file })
	if len(kept) == all {
		return nil
	}
	// Written whole beside it and renamed over it, so nobody reads a half-written index.
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".MEMORY-*.md")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(strings.Join(kept, "")); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(st.Mode().Perm()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
