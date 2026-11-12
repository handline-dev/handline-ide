package leases

import (
	"errors"
	"net"
	"regexp"
	"strconv"
	"syscall"

	"github.com/handline/handline/go/accounts/usage"
)

// PortRangeKey is the config.yaml key of the ports a port lease is picked from, "4000-4999".
const PortRangeKey = "port_range"

// PortRange is the range cfg sets; ok is false when it is unset or not lo-hi with 1024 <= lo <= hi <= 65535.
func PortRange(cfg usage.Config) (lo, hi int, ok bool) {
	s, _ := cfg[PortRangeKey].(string)
	return ParsePortRange(s)
}

var portRangeRe = regexp.MustCompile(`^([0-9]{1,5})-([0-9]{1,5})$`)

// ParsePortRange reads "lo-hi", digits only, as the contract's pattern says.
func ParsePortRange(s string) (lo, hi int, ok bool) {
	g := portRangeRe.FindStringSubmatch(s)
	if g == nil {
		return 0, 0, false
	}
	lo, _ = strconv.Atoi(g[1])
	hi, _ = strconv.Atoi(g[2])
	if lo < 1024 || hi > 65535 || lo > hi {
		return 0, 0, false
	}
	return lo, hi, true
}

func (m *Manager) portRange() (lo, hi int, ok bool) {
	if m.Ports == nil {
		return 0, 0, false
	}
	return m.Ports()
}

// probeAddrs are where a server may listen; a port is free only when every one binds. SO_REUSEADDR lets 127.0.0.1 bind beside a
// listener on the wildcard, so each is tried.
var probeAddrs = []struct{ network, host string }{{"tcp4", "127.0.0.1"}, {"tcp4", "0.0.0.0"}, {"tcp6", "::1"}, {"tcp6", "::"}}

// portFree is whether port binds on this machine now.
func (m *Manager) portFree(port int) bool {
	if m.PortFree != nil {
		return m.PortFree(port)
	}
	for _, a := range probeAddrs {
		ln, err := net.Listen(a.network, net.JoinHostPort(a.host, strconv.Itoa(port)))
		if err != nil {
			if a.network == "tcp6" && !errors.Is(err, syscall.EADDRINUSE) {
				continue // no IPv6 here
			}
			return false
		}
		ln.Close()
	}
	return true
}

func (m *Manager) heldPorts() map[int]bool {
	held := map[int]bool{}
	for _, l := range m.held {
		if l.Kind == Port {
			held[l.Port] = true
		}
	}
	return held
}

// pickPort is a port of the range that no lease holds and that binds now, 0 for none; with no range, one the system gives. The caller
// holds mu.
func (m *Manager) pickPort() int {
	held := m.heldPorts()
	lo, hi, ok := m.portRange()
	if !ok {
		return m.systemPort(held)
	}
	if len(held) > hi-lo { // every port is leased: no need to probe
		return 0
	}
	for p := lo; p <= hi; p++ {
		if !held[p] && m.portFree(p) {
			return p
		}
	}
	return 0
}

// systemPort is a free port the system gives that no lease holds, 0 when it gives none.
func (m *Manager) systemPort(held map[int]bool) int {
	for range 8 {
		ln, err := net.Listen("tcp", ":0")
		if err != nil {
			m.logf("lease: no free port: %v", err)
			return 0
		}
		p := ln.Addr().(*net.TCPAddr).Port
		ln.Close()
		if !held[p] {
			return p
		}
	}
	return 0
}
