package daemon

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/internal/adminwrites"
	"github.com/handline/handline/go/protocol/rpc"
)

// The kinds of address connection.addresses lists.
const (
	kindLAN           = "lan"            // a private IPv4 of a LAN or Wi-Fi interface
	kindTailscale     = "tailscale"      // the Tailscale interface's IPv4
	kindTailscaleName = "tailscale_name" // the MagicDNS name, served on the Tailscale IP
	kindMDNS          = "mdns"           // name.local, served on every LAN IP
)

// netAddress is one host this machine can be reached on, and the IPs a listener binds to serve it.
type netAddress struct {
	Host, Kind, Iface string
	IPs               []net.IP
}

// plain says whether the host is served without TLS: a Tailscale one is (the tailnet encrypts), unless the daemon was given a certificate.
func (a netAddress) plain(cfg Config) bool {
	return cfg.plain() && (a.Kind == kindTailscale || a.Kind == kindTailscaleName)
}

// addressesNow is machineAddresses; a variable so a test can serve hosts it can bind.
var addressesNow = machineAddresses

// machineAddresses is every host this machine can be reached on from another device: no loopback and no public address.
func machineAddresses() []netAddress {
	all, err := interfaceAddrs()
	if err != nil {
		return nil
	}
	var out, lan []netAddress
	var ts *netAddress
	for _, a := range all {
		name := strings.ToLower(a.Name)
		switch {
		case (strings.HasPrefix(name, "utun") || strings.HasPrefix(name, "tailscale")) && tailscaleV4.Contains(a.IP):
			out = append(out, netAddress{Host: a.IP.String(), Kind: kindTailscale, Iface: a.Name, IPs: []net.IP{a.IP}})
			if ts == nil {
				ts = &out[len(out)-1]
			}
		case a.IP.IsPrivate():
			lan = append(lan, netAddress{Host: a.IP.String(), Kind: kindLAN, Iface: a.Name, IPs: []net.IP{a.IP}})
		}
	}
	if ts != nil {
		if n := tailscaleName(); n != "" {
			out = append(out, netAddress{Host: n, Kind: kindTailscaleName, Iface: ts.Iface, IPs: ts.IPs})
		}
	}
	out = append(lan, out...)
	if n := localName(); n != "" && len(lan) > 0 {
		m := netAddress{Host: n, Kind: kindMDNS}
		for _, l := range lan {
			m.IPs = append(m.IPs, l.IPs...)
		}
		out = append(out, m)
	}
	return out
}

// localName is this machine's name.local, which Bonjour answers on its LAN addresses; a variable so a test can name one.
var localName = func() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return ""
	}
	return strings.SplitN(h, ".", 2)[0] + ".local"
}

// tailscaleName is this machine's MagicDNS name out of `tailscale status --json`, or "". It is kept a minute: the address list is read
// every few seconds.
var tailscaleName = func() string {
	tsName.mu.Lock()
	defer tsName.mu.Unlock()
	if time.Since(tsName.at) < time.Minute {
		return tsName.name
	}
	tsName.name, tsName.at = readTailscaleName(), time.Now()
	return tsName.name
}

var tsName struct {
	mu   sync.Mutex
	name string
	at   time.Time
}

func readTailscaleName() string {
	for _, cli := range []string{"tailscale", "/Applications/Tailscale.app/Contents/MacOS/Tailscale"} {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		out, err := exec.CommandContext(ctx, cli, "status", "--json").Output()
		cancel()
		if err != nil {
			continue
		}
		var st struct{ Self struct{ DNSName string } }
		if json.Unmarshal(out, &st) != nil {
			continue
		}
		if n := strings.TrimSuffix(st.Self.DNSName, "."); strings.HasSuffix(n, ".ts.net") {
			return n
		}
	}
	return ""
}

// chosenHosts is the network_addresses setting: nil when unset, the hosts in their order otherwise (an empty list serves none).
func chosenHosts(home string) []string {
	v, set := usage.LoadConfig(home)[usage.NetworkAddressesKey]
	if !set || v == nil {
		return nil
	}
	hosts := []string{}
	if l, ok := v.([]any); ok {
		for _, h := range l {
			if s, ok := h.(string); ok && s != "" {
				hosts = append(hosts, s)
			}
		}
	}
	return hosts
}

// connectionAddresses is connection.addresses: every host this machine can be reached on, read now (interfaces come and go with Wi-Fi),
// whether a listener serves it and how.
func (d *Daemon) connectionAddresses(_ *rpc.Conn, _ map[string]any) (any, error) {
	items := []any{}
	for _, a := range addressesNow() {
		enc := "tls"
		if a.plain(d.cfg) {
			enc = "none"
		}
		listening := false
		if d.net != nil {
			_, listening = d.net.door(a.Host)
		}
		items = append(items, map[string]any{"host": a.Host, "kind": a.Kind, "interface": a.Iface, "listening": listening, "encryption": enc})
	}
	var chosen any
	if h := chosenHosts(d.cfg.Home); h != nil {
		chosen = h
	}
	return map[string]any{"addresses": items, "network_addresses": chosen}, nil
}

// netSetting is adminwrites.Network for this daemon: it reads d.net when called, as the writes are registered before the doors open.
type netSetting struct{ d *Daemon }

func (d *Daemon) networkSetting() adminwrites.Network { return netSetting{d} }

func (s netSetting) Known(hosts []string) error {
	have := map[string]bool{}
	names := []string{}
	for _, a := range addressesNow() {
		have[a.Host] = true
		names = append(names, a.Host)
	}
	for _, h := range hosts {
		if !have[h] {
			e := rpc.Refused(h+" is not a host this machine can be reached on: one of "+strings.Join(names, ", "), "address", "")
			e.Data["addresses"] = names
			return e
		}
	}
	return nil
}

func (s netSetting) Relisten() []string {
	n := s.d.net
	cut := []string{}
	if n == nil {
		return cut
	}
	if err := n.relisten(); err != nil {
		s.d.cfg.Log("network listener: %v", err)
	}
	served := map[string]bool{}
	for _, h := range n.hosts() {
		served[h] = true
	}
	ds, err := s.d.dev.Devices()
	if err != nil {
		return cut
	}
	before := n.pairedBefore()
	for _, dv := range ds {
		host := dv.Address
		if host == "" {
			host = before
		}
		if !served[host] {
			cut = append(cut, dv.Name)
		}
	}
	return cut
}
