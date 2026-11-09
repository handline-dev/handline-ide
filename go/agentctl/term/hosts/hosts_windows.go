package hosts

import (
	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/agentctl/term/windows"
)

// Default is the host on Windows, which drives no terminal yet.
const Default = "windows"

func init() { term.Register(Default, func(term.Place) term.Backend { return windows.Host{} }) }
