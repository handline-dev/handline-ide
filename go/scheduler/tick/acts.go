package tick

import (
	"slices"
	"sync"
)

// Acts are agents the loop supervises at once between ticks (Loop.Act): work the founder's pause held that it now lets through. Adding
// never blocks; what is added while a tick runs is taken as soon as it ends.
type Acts struct {
	mu     sync.Mutex
	agents []string
	ready  chan struct{}
}

func NewActs() *Acts { return &Acts{ready: make(chan struct{}, 1)} }

// Add asks for the agents to be supervised now.
func (a *Acts) Add(agents ...string) {
	if len(agents) == 0 {
		return
	}
	a.mu.Lock()
	for _, x := range agents {
		if !slices.Contains(a.agents, x) {
			a.agents = append(a.agents, x)
		}
	}
	a.mu.Unlock()
	select {
	case a.ready <- struct{}{}:
	default:
	}
}

// Ready fires once agents were added; nil Acts never fires.
func (a *Acts) Ready() <-chan struct{} {
	if a == nil {
		return nil
	}
	return a.ready
}

// Take is the agents added since the last Take.
func (a *Acts) Take() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := a.agents
	a.agents = nil
	return out
}
