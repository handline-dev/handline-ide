package rulesfold

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// RunTimeout is the longest one step's model answer may take.
const RunTimeout = 10 * time.Minute

// FoldModel is the model a step asks.
const FoldModel = "sonnet"

// Claude is a Model that runs the CLI at path once, headless, on the account whose config folder is configDir: no tools, no session
// kept, in an empty folder with only project settings, so the account's hooks (which report to the daemon as an agent) do not run.
// The child sees only PATH, HOME, USER, LOGNAME, TMPDIR and LANG of the daemon's environment, and env, the pairs the daemon sets on
// every claude it runs.
func Claude(path, configDir string, env []string) Model {
	return func(ctx context.Context, prompt, schema string) ([]byte, error) {
		cwd, err := os.MkdirTemp("", "rules-fold-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(cwd)
		ctx, cancel := context.WithTimeout(ctx, RunTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, path, "--print", "--model", FoldModel, "--output-format", "json", "--json-schema", schema,
			"--no-session-persistence", "--tools", "", "--setting-sources", "project")
		cmd.Dir, cmd.Stdin = cwd, bytes.NewBufferString(prompt)
		for _, k := range []string{"PATH", "HOME", "USER", "LOGNAME", "TMPDIR", "LANG"} {
			if v, ok := os.LookupEnv(k); ok {
				cmd.Env = append(cmd.Env, k+"="+v)
			}
		}
		cmd.Env = append(append(cmd.Env, "CLAUDE_CONFIG_DIR="+configDir), env...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("claude: %w: %.200s", err, stderr.String())
		}
		return structured(out)
	}
}

// structured is the answer's structured output; the rest of the CLI's report (usage, cost) is not kept.
func structured(out []byte) ([]byte, error) {
	var r struct {
		IsError    bool            `json:"is_error"`
		Structured json.RawMessage `json:"structured_output"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("claude's report: %w", err)
	}
	if r.IsError || len(r.Structured) == 0 {
		return nil, errors.New("claude gave no structured answer")
	}
	return r.Structured, nil
}
