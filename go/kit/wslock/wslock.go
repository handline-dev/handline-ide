// Package wslock keeps a workspace's delete apart from the calls that add to it or start in it, within the daemon: any number of
// those share a workspace, and a delete has it alone. Neither waits: whoever cannot have it is refused.
package wslock

import "sync"

var held sync.Map // home and workspace key -> *sync.RWMutex

func of(home, ws string) *sync.RWMutex {
	m, _ := held.LoadOrStore(home+"\x00"+ws, &sync.RWMutex{})
	return m.(*sync.RWMutex)
}

// Use is a hold on home's workspace ws for a call that adds an agent to it or starts one in it; ok is false while it is being deleted.
func Use(home, ws string) (release func(), ok bool) {
	m := of(home, ws)
	if !m.TryRLock() {
		return nil, false
	}
	return m.RUnlock, true
}

// Delete is ws for its delete alone; ok is false while a call holds it by Use.
func Delete(home, ws string) (release func(), ok bool) {
	m := of(home, ws)
	if !m.TryLock() {
		return nil, false
	}
	return m.Unlock, true
}
