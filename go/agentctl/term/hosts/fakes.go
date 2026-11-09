//go:build fakes

package hosts

import (
	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/agentctl/term/fake"
)

func init() { term.Register("fake", func(p term.Place) term.Backend { return fake.Fake{Place: p} }) }
