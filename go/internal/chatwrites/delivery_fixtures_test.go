package chatwrites

import (
	"bufio"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/handline/handline/go/agentctl/delivery"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/internal/fixtureplay"
	"github.com/handline/handline/go/internal/reads"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/taskboard/supervisor"
)

// The fixtures of contract/fixtures/delivery.json, which need structured agents the daemon binary cannot host yet, played here against the
// chat writes and reads on the same server: bob is idle and echoes during a fixture's sleep, carol has a turn running, dave lost an operation, alice is a terminal agent.

func TestTheDeliveryFixturesPassAgainstTheChatWritesAndReads(t *testing.T) {
	t.Parallel()
	fixtures, err := fixtureplay.Load(filepath.Join("..", "..", "..", "contract", "fixtures", "delivery.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, fx := range fixtures {
		t.Run(fx.Name, func(t *testing.T) {
			r := newRig(t)
			teamtest.Seed(t, r.home, "carol", "dave")
			f := &fakeSessions{r: r, busyFor: map[string]bool{r.k("carol"): true}}
			r.env.Agents = f
			srv := r.server("")
			reads.Register(srv, reads.Env{DB: r.db, Home: r.home, Started: time.Now()})
			// dave's operation was written, and the session ended before it was echoed
			r.one(srv, "chat.send", `{"agent":"dave","body":"in flight when it died","request_id":"crashed-op"}`)
			f.setState(r.k("dave"), supervisor.RequestOp(1), delivery.Unknown)
			vars := map[string]any{"founder_key": "founder", "transcript_agent": r.k("bob"), "working_agent": r.k("carol"), "crashed_agent": r.k("dave"), "terminal_agent": r.k("alice")}
			saved := map[string]any{}
			cl, sv := net.Pipe()
			done := make(chan struct{})
			go func() { srv.Serve(rpc.NewLineTransport(sv), 0); sv.Close(); close(done) }()
			defer func() { cl.Close(); <-done }()
			rd := bufio.NewReader(cl)
			cl.SetDeadline(time.Now().Add(20 * time.Second))
			for n, step := range fx.Steps {
				if s, ok := step["sleep_s"].(float64); ok {
					f.confirmAll() // the CLI echoes while the client waits
					time.Sleep(time.Duration(s * float64(time.Second)))
				}
				line, _ := json.Marshal(fixtureplay.Fill(step["send"], mergeVars(vars, saved)))
				cl.Write(append(line, '\n'))
				b, err := rd.ReadBytes('\n')
				if err != nil {
					t.Fatal(err)
				}
				var reply map[string]any
				json.Unmarshal(b, &reply)
				var want any
				got := reply["result"]
				if want = step["result"]; want == nil {
					want, got = step["error"], reply["error"]
				}
				if err := fixtureplay.Match(fixtureplay.Fill(want, mergeVars(vars, saved)), got, saved, "$"); err != nil {
					t.Fatalf("step %d: %v\nreply %s", n+1, err, b)
				}
			}
		})
	}
}

func mergeVars(a, b map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
