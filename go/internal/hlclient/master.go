package hlclient

import (
	"fmt"
	"time"

	"github.com/handline/handline/go/protocol/rpc"
)

// MasterSays is who a master says it is in its hello (capabilities.master): its node id and master number.
type MasterSays struct {
	Node   string
	Number int64
}

// MasterIn reads MasterSays from a hello's answer; zero when it names none.
func MasterIn(hello map[string]any) MasterSays {
	caps, _ := hello["capabilities"].(map[string]any)
	m, _ := caps["master"].(map[string]any)
	node, _ := m["node"].(string)
	n, _ := rpc.AsInt(m["number"])
	return MasterSays{Node: node, Number: n}
}

// AskWait bounds a question to another machine's door: it is asked while a daemon decides what it is, so one that does not answer
// is let go soon.
const AskWait = 10 * time.Second

// Ask dials the door at addr (pinned to fp, "" for a plain door), says hello with token as client, and answers who is master there;
// then, when set, runs on the open link before it is closed.
func Ask(addr, fp, token, client string, then func(l *rpc.Link, m MasterSays) error) (MasterSays, error) {
	ws, err := DialMaster(addr, fp)
	if err != nil {
		return MasterSays{}, err
	}
	l := rpc.NewLink(rpc.NewWSTransport(ws), func(method string, _ map[string]any) (any, error) {
		return nil, rpc.Refused(method+" is not served on this link", "link", "")
	})
	ended := make(chan error, 1)
	go func() { ended <- l.Run(AskWait) }()
	defer func() { l.Close(); <-ended }()
	got, err := l.Call("hello", map[string]any{"client": client, "protocol": rpc.Protocol, "token": token}, AskWait)
	if err != nil {
		return MasterSays{}, fmt.Errorf("hello at %s: %w", addr, err)
	}
	m := MasterIn(got)
	if m.Node == "" {
		return m, fmt.Errorf("%s names no master", addr)
	}
	if then != nil {
		return m, then(l, m)
	}
	return m, nil
}

// Above is whether m outranks the master node with number n: a higher number, or at the same number the higher node id, so two
// machines made master while they could not reach each other still agree which one is.
func (m MasterSays) Above(n int64, node string) bool {
	return m.Number > n || m.Number == n && m.Node > node
}

// Door is a machine's door to ask: its node id, address and certificate fingerprint.
type Door struct{ Node, Addr, FP string }

// Highest asks every door at once with token (Ask) and answers the door and word of the highest master among those whose answer names
// the node the door is for; ok false when none does.
func Highest(doors []Door, token, client string) (best Door, says MasterSays, ok bool) {
	type answer struct {
		door Door
		says MasterSays
	}
	got := make(chan *answer, len(doors))
	for _, d := range doors {
		go func(d Door) {
			s, err := Ask(d.Addr, d.FP, token, client, nil)
			if err != nil || s.Node != d.Node {
				got <- nil
				return
			}
			got <- &answer{d, s}
		}(d)
	}
	for range doors {
		if a := <-got; a != nil && (!ok || a.says.Above(says.Number, says.Node)) {
			best, says, ok = a.door, a.says, true
		}
	}
	return best, says, ok
}
