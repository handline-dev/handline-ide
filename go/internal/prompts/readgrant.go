package prompts

import (
	"path/filepath"
	"strings"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/scheduler/access"
)

// AlwaysKey is the choice that allows a read of another agent's memory or transcripts and keeps it allowed: it stores ReadGrant as
// the agent's grants, which the next launch lifts the hide for.
const AlwaysKey = "always"

// ReadGrant is what an "Allow always" stores when the tool call reads another agent's memory or a transcript: the agents folder,
// which holds every memory folder, and every Claude account's transcripts. Nil for any other call. The founder allowed reading all of
// them at once, so one answer covers them all rather than one question per folder.
func (e *Env) ReadGrant(tool string, input map[string]any) []string {
	if tool != "Read" && tool != "Grep" && tool != "Glob" {
		return nil
	}
	p, _ := input["file_path"].(string)
	for _, k := range []string{"path", "pattern"} {
		if p == "" {
			p, _ = input[k].(string)
		}
	}
	if !filepath.IsAbs(p) {
		return nil
	}
	agents := paths.AgentsDir(e.Home)
	grant := []string{agents}
	for _, acct := range paths.ListAccounts(e.Home) {
		if team.AccountVendor(e.Home, acct) != team.DefaultVendor {
			continue
		}
		if cfg, err := team.AccountConfigDir(e.Home, acct); err == nil {
			grant = append(grant, filepath.Join(cfg, "projects"))
		}
	}
	rel, err := filepath.Rel(agents, filepath.Clean(p))
	parts := strings.Split(rel, string(filepath.Separator))
	memory := err == nil && parts[0] != ".." && len(parts) >= 2 && parts[1] == "memory"
	if memory || len(access.Outside(grant[1:], []string{p})) == 0 {
		return grant
	}
	return nil
}
