//go:build !windows

package hosts

import (
	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/agentctl/term/tmux"
)

// Default is the host on macOS and Linux.
const Default = "tmux"

func init() { term.Register("tmux", func(p term.Place) term.Backend { return tmux.Tmux{Place: p} }) }
