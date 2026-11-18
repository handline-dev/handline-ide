package compose

import (
	"testing"

	"github.com/handline/handline/go/internal/rpctest"
	"github.com/handline/handline/go/protocol/rpc"
)

type founder struct{}

func (founder) AgentOf(int) string         { return "" }
func (founder) FounderKeyOK(k string) bool { return k == "key" }

// call serves one connection and returns the replies to the calls given (after the hello).
func call(t *testing.T, srv *rpc.Server, calls ...string) []map[string]any {
	return rpctest.Call(t, srv, "key", calls...)
}

func (founder) SessionsReadable() error { return nil }
