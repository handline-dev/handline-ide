// Package rpctest serves one connection of an rpc.Server over a pipe, for tests.
package rpctest

import (
	"bufio"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/handline/handline/go/protocol/rpc"
)

// Call says hello with key, then sends each call, and is the replies to the calls (not the hello's).
func Call(t testing.TB, srv *rpc.Server, key string, calls ...string) []map[string]any {
	t.Helper()
	cl, sv := net.Pipe()
	done := make(chan struct{})
	go func() { srv.Serve(rpc.NewLineTransport(sv), 0); sv.Close(); close(done) }()
	defer func() { cl.Close(); <-done }()
	rd := bufio.NewReader(cl)
	cl.SetDeadline(time.Now().Add(30 * time.Second))
	hello := `{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"t","protocol":1,"key":"` + key + `"}}`
	var out []map[string]any
	for _, line := range append([]string{hello}, calls...) {
		if _, err := cl.Write([]byte(line + "\n")); err != nil {
			t.Fatal(err)
		}
		b, err := rd.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		json.Unmarshal(b, &m)
		out = append(out, m)
	}
	return out[1:]
}
