package runner

// The holders of the Chat sessions that run here (docs/several-machines.md): the master reads and writes each over the link as if it
// were its own, named by its socket's file name under the home's chat folder.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/handline/handline/go/agentctl/holder"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
)

// chatWrites is how many lines a holder's writes remember: a line written again (its answer was lost with the link) is not written
// twice. Every line the master writes is unique by its uuid or request id.
const chatWrites = 256

type chats struct {
	writing map[string]*sync.Mutex // by socket: one write at a time to a holder, so a repeat never races its first
	mu      sync.Mutex
	readers map[string]*holder.Reader // by stream id
	written map[string][]string       // by socket: the hashes of the last lines written, oldest first
}

// chatSocket is the holder at name under the home's chat folder; "" for a name that is not one there.
func (r *Runner) chatSocket(name string) string {
	if name == "" || name != filepath.Base(name) || !strings.HasSuffix(name, ".sock") {
		return ""
	}
	return filepath.Join(r.cfg.Home, "chat", name)
}

// chatHeadless is whether the session in handle runs under a live Chat holder: no key or screen read reaches it.
func (r *Runner) chatHeadless(handle string) bool {
	r.mu.Lock()
	agent := ""
	for a, s := range r.sessions {
		if s.Handle == handle {
			agent = a
		}
	}
	r.mu.Unlock()
	return agent != "" && holder.Alive(paths.ChatSocket(r.cfg.Home, agent))
}

// chatList is the live holders here, by socket name.
func (r *Runner) chatList() (any, error) {
	dir := filepath.Join(r.cfg.Home, "chat")
	ents, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	names := []string{}
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".sock") && holder.Alive(filepath.Join(dir, e.Name())) {
			names = append(names, e.Name())
		}
	}
	return map[string]any{"holders": names}, nil
}

// chatRead starts a stream of a holder's records for the master: each goes up as a runner.output notice, and the stream's end as one
// whose method is "ended".
func (r *Runner) chatRead(p map[string]any, notify func(string, map[string]any) bool) (any, error) {
	id, _ := p["stream"].(string)
	name, _ := p["holder"].(string)
	from, _ := rpc.AsInt(p["from"])
	sock := r.chatSocket(name)
	if id == "" || sock == "" {
		return nil, rpc.BadRequestError("stream and holder name a holder here", "holder", rpc.InvalidParams)
	}
	rd, err := holder.Open(context.Background(), sock, from)
	if err != nil {
		return nil, rpc.NotFoundError("no holder " + name + " here: " + err.Error())
	}
	r.chat.mu.Lock()
	if r.chat.readers == nil {
		r.chat.readers = map[string]*holder.Reader{}
	}
	r.chat.readers[id] = rd
	r.chat.mu.Unlock()
	go func() {
		defer func() { // a stream of a newer link may have its id now: only this one goes
			r.chat.mu.Lock()
			if r.chat.readers[id] == rd {
				delete(r.chat.readers, id)
			}
			r.chat.mu.Unlock()
			rd.Close()
		}()
		for {
			rec, err := rd.Next()
			if err != nil {
				notify("runner.output", map[string]any{"stream": id, "method": "ended"})
				return
			}
			b, _ := json.Marshal(rec)
			if !notify("runner.output", map[string]any{"stream": id, "method": "record", "record": string(b)}) {
				return
			}
		}
	}()
	b, _ := os.ReadFile(holder.PidFile(sock))
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b))) // which holder this is: a new one at the name starts its records over
	return map[string]any{"accepted": true, "pid": pid}, nil
}

func (r *Runner) chatAck(p map[string]any) (any, error) {
	id, _ := p["stream"].(string)
	seq, _ := rpc.AsInt(p["seq"])
	r.chat.mu.Lock()
	rd := r.chat.readers[id]
	r.chat.mu.Unlock()
	if rd == nil {
		return nil, rpc.NotFoundError("no chat stream " + id)
	}
	if err := rd.Ack(seq); err != nil {
		return nil, err
	}
	return map[string]any{"accepted": true}, nil
}

// chatDetach ends the stream id, if it is a chat one.
func (r *Runner) chatDetach(id string) {
	r.chat.mu.Lock()
	rd := r.chat.readers[id]
	delete(r.chat.readers, id)
	r.chat.mu.Unlock()
	if rd != nil {
		rd.Close()
	}
}

// chatDetachAll ends every chat stream: the link they went up ended.
func (r *Runner) chatDetachAll() {
	r.chat.mu.Lock()
	rds := r.chat.readers
	r.chat.readers = nil
	r.chat.mu.Unlock()
	for _, rd := range rds {
		rd.Close()
	}
}

// chatWrite gives the holder's CLI one line, once.
func (r *Runner) chatWrite(p map[string]any) (any, error) {
	name, _ := p["holder"].(string)
	line, _ := p["line"].(string)
	sock := r.chatSocket(name)
	if sock == "" || line == "" {
		return nil, rpc.BadRequestError("holder names a holder here and line is a line", "holder", rpc.InvalidParams)
	}
	sum := sha256.Sum256([]byte(line))
	h := string(sum[:])
	r.chat.mu.Lock()
	if r.chat.writing == nil {
		r.chat.writing = map[string]*sync.Mutex{}
	}
	one := r.chat.writing[sock]
	if one == nil {
		one = &sync.Mutex{}
		r.chat.writing[sock] = one
	}
	r.chat.mu.Unlock()
	one.Lock()
	defer one.Unlock()
	r.chat.mu.Lock()
	seen := slices.Contains(r.chat.written[sock], h)
	r.chat.mu.Unlock()
	if seen {
		return map[string]any{"accepted": true}, nil
	}
	if err := holder.Write(sock, []byte(line)); err != nil {
		return nil, rpc.Refused("could not write to "+name+": "+err.Error(), "holder", "")
	}
	r.chat.mu.Lock()
	defer r.chat.mu.Unlock()
	if r.chat.written == nil {
		r.chat.written = map[string][]string{}
	}
	w := append(r.chat.written[sock], h)
	if len(w) > chatWrites {
		w = w[len(w)-chatWrites:]
	}
	r.chat.written[sock] = w
	return map[string]any{"accepted": true}, nil
}

func (r *Runner) chatClose(p map[string]any) (any, error) {
	name, _ := p["holder"].(string)
	sock := r.chatSocket(name)
	if sock == "" {
		return nil, rpc.BadRequestError("holder names a holder here", "holder", rpc.InvalidParams)
	}
	if err := holder.CloseInput(sock); err != nil {
		return nil, rpc.Refused("could not end the input of "+name+": "+err.Error(), "holder", "")
	}
	return map[string]any{"accepted": true}, nil
}
