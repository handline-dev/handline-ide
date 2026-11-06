package daemon

import (
	"net"
	"strings"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/internal/reads"
	"github.com/handline/handline/go/protocol/devices"
	"github.com/handline/handline/go/protocol/rpc"
)

// connectionGet is connection.get: how this daemon is reached, read only. listener is the door a pairing names by default, listeners
// every host a door is open for. Without -listen it has no listener.
func (d *Daemon) connectionGet(_ *rpc.Conn, _ map[string]any) (any, error) {
	var listener any
	listeners := []any{}
	if d.net != nil {
		if door, ok := d.net.door(""); ok {
			listener = doorRow(door)
		}
		for _, h := range d.net.hosts() {
			if door, ok := d.net.door(h); ok {
				listeners = append(listeners, doorRow(door))
			}
		}
	}
	return map[string]any{"listener": listener, "listeners": listeners,
		"dev_mode": reads.DevMode(usage.LoadConfig(d.cfg.Home))}, nil
}

func doorRow(door devices.Door) map[string]any {
	l := map[string]any{"address": door.Addr, "encryption": "tls", "fingerprint": door.Fingerprint, "tailscale_host": nil, "web": door.Web}
	if !door.TLS {
		l["encryption"], l["fingerprint"] = "none", nil
	}
	if host, _, err := net.SplitHostPort(door.Addr); err == nil && strings.HasSuffix(host, ".ts.net") {
		l["tailscale_host"] = host
	}
	return l
}
