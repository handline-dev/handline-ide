package adminwrites

import (
	"errors"
	"fmt"
	"strings"

	"github.com/handline/handline/go/internal/worktreeprune"
	"github.com/handline/handline/go/kit/protected"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/worktreesetup"
)

// repo.set: a repo's worktree setup, what every worktree the daemon makes for it gets. An omitted field is kept, null clears it.
func (e *Env) repoSet(c *rpc.Conn, p map[string]any) (any, error) {
	path, _ := p["path"].(string)
	known := false
	for _, r := range worktreeprune.Repos(e.Home) {
		known = known || r == path
	}
	if !known {
		return nil, notFound("repo", path, fmt.Sprintf("%q is no repo an agent works in (repos.list)", path))
	}
	st := protected.New(e.Home)
	s := worktreesetup.Get(st, path)
	var said []string
	if v, in := p["script"]; in {
		script, ok := v.(string)
		if v != nil && !ok {
			return nil, badParam("script is text, or null", "script")
		}
		s.Script = strings.TrimSpace(script)
		said = append(said, "script "+map[bool]string{true: "cleared", false: "set"}[s.Script == ""])
	}
	if v, in := p["copy"]; in {
		l, ok := v.([]any)
		if v != nil && !ok {
			return nil, badParam("copy is a list of paths, or null", "copy")
		}
		s.Copy = []string{}
		for _, x := range l {
			entry, ok := x.(string)
			if !ok {
				return nil, badParam("copy is a list of paths, or null", "copy")
			}
			s.Copy = append(s.Copy, strings.TrimSpace(entry))
		}
		said = append(said, fmt.Sprintf("copy %d path(s)", len(s.Copy)))
	}
	if len(said) == 0 {
		return nil, badParam("name script or copy", "script")
	}
	if err := worktreesetup.Put(st, path, s); err != nil {
		var fe *worktreesetup.FieldError
		if errors.As(err, &fe) {
			return nil, badParam(fe.Msg, fe.Field)
		}
		return nil, refused(err.Error(), "state")
	}
	return wrote(c, strings.Join(said, ", ")+" for "+path, nil)
}
