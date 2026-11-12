// Package vendors links every agent CLI the daemon knows, so each registers its account facts with package agent: a binary and a test
// that look an account's vendor up import it for that.
package vendors

import (
	_ "github.com/handline/handline/go/agentctl/agent/claudecode"
	_ "github.com/handline/handline/go/agentctl/agent/codex"
	_ "github.com/handline/handline/go/agentctl/agent/opencode"
)
