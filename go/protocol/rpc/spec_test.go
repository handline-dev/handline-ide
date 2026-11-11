package rpc

import (
	"strings"
	"testing"
)

// A method of the contract takes its role and write flag from the spec; a registration that says otherwise panics.
func TestRegisterTakesRoleAndWriteFromTheSpec(t *testing.T) {
	if sp, ok := Spec("task.done"); !ok || !sp.Write || sp.Role != "agent" {
		t.Fatalf("task.done in the spec: %+v %v", sp, ok)
	}
	s := NewServer(nil, "", 1, "local", t.Logf)
	s.Register("task.done", Method{})
	if m := s.methods["task.done"]; m.Role != "agent" || !m.Write {
		t.Fatalf("registered %+v; want role agent, write", m)
	}
	panics := func(name string, m Method, want string) {
		t.Helper()
		defer func() {
			if r := recover(); r == nil || !strings.Contains(r.(string), want) {
				t.Errorf("%s %+v: recovered %v; want a panic with %q", name, m, r, want)
			}
		}()
		s.Register(name, m)
	}
	panics("task.done", Method{Role: "owner"}, "contract/spec says")
	panics("task.get", Method{Write: true}, "contract/spec says")
	panics("task.done", Method{Read: true}, "contract/spec says")
	panics("test.open", Method{}, "gives no Role")
	s.Register("test.only", Method{Role: "owner"}) // a name the spec lacks keeps its own role
	if s.methods["test.only"].Role != "owner" {
		t.Fatalf("test.only: %+v", s.methods["test.only"])
	}
}
