package agentview

import (
	"path/filepath"
	"strings"
)

// AskLine is a permission prompt's tool and command as the one line a row shows. A leading cd into dir, the agent's own folder, is
// dropped with its ; or &&: the line is cut short, and the folder would fill it in place of the command asked about.
func AskLine(tool, command, dir string) string {
	return strings.TrimSpace(tool + ": " + dropOwnCd(command, dir))
}

func dropOwnCd(cmd, dir string) string {
	s, ok := strings.CutPrefix(strings.TrimLeft(cmd, " "), "cd ")
	if dir == "" || !ok {
		return cmd
	}
	s = strings.TrimLeft(s, " ")
	var path string
	if q := s[:min(len(s), 1)]; q == `"` || q == "'" {
		end := strings.Index(s[1:], q)
		if end < 0 {
			return cmd
		}
		path, s = s[1:1+end], s[2+end:]
	} else {
		end := strings.IndexAny(s, " ;&")
		if end < 0 {
			return cmd
		}
		path, s = s[:end], s[end:]
	}
	if filepath.Clean(path) != filepath.Clean(dir) {
		return cmd
	}
	s = strings.TrimLeft(s, " ")
	rest, ok := strings.CutPrefix(s, "&&")
	if !ok {
		if rest, ok = strings.CutPrefix(s, ";"); !ok {
			return cmd
		}
	}
	if rest = strings.TrimSpace(rest); rest == "" {
		return cmd
	}
	return rest
}
