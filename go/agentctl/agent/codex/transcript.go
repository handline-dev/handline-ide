package codex

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SessionFile is the transcript of the newest codex session started in cwd under configDir:
// sessions/YYYY/MM/DD/rollout-<time>-<id>.jsonl, whose first line's session_meta names its cwd. Today's and yesterday's folders are
// read first; the whole tree only when neither has one.
func SessionFile(configDir, cwd string, now time.Time) (string, bool) {
	root := filepath.Join(configDir, "sessions")
	var days []string
	for _, d := range []time.Time{now, now.AddDate(0, 0, -1)} {
		days = append(days, filepath.Join(root, d.Format("2006/01/02"), "rollout-*.jsonl"))
	}
	if p, ok := newestFor(cwd, days...); ok {
		return p, true
	}
	return newestFor(cwd, filepath.Join(root, "*", "*", "*", "rollout-*.jsonl"))
}

func newestFor(cwd string, globs ...string) (string, bool) {
	real, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		real = cwd
	}
	best, at := "", time.Time{}
	for _, g := range globs {
		matches, _ := filepath.Glob(g)
		for _, m := range matches {
			fi, err := os.Stat(m)
			if err != nil || !fi.ModTime().After(at) {
				continue
			}
			if c := sessionCwd(m); c != cwd && c != real {
				continue
			}
			best, at = m, fi.ModTime()
		}
	}
	return best, best != ""
}

// sessionCwd is the cwd a transcript's session_meta line names, "" when it cannot be read.
func sessionCwd(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	r := bufio.NewReader(f)
	line, _ := r.ReadBytes('\n')
	var meta struct {
		Type    string `json:"type"`
		Payload struct {
			Cwd string `json:"cwd"`
		} `json:"payload"`
	}
	if json.Unmarshal(line, &meta) != nil || meta.Type != "session_meta" {
		return ""
	}
	return meta.Payload.Cwd
}

// userText is the text of a transcript line that records a message the person sent, ok false for any other line: the context codex
// files as a user message ("<environment_context>", "<user_instructions>") included.
func userText(line []byte) (string, bool) {
	var rec struct {
		Type    string `json:"type"`
		Payload struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"payload"`
	}
	if json.Unmarshal(line, &rec) != nil || rec.Type != "response_item" || rec.Payload.Type != "message" || rec.Payload.Role != "user" {
		return "", false
	}
	text := ""
	for _, c := range rec.Payload.Content {
		if c.Type == "input_text" {
			text += c.Text
		}
	}
	return text, !strings.HasPrefix(text, "<")
}
