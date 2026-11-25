package daemon

import (
	"cmp"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/handline/handline/go/protocol/devices"
	"github.com/handline/handline/go/protocol/rpc"
)

// networkListener is one network door: the same JSON-RPC over a WebSocket on one IP, TLS or plain (a Tailscale address), closed to
// every caller a device token has not opened (see rpc.NetGate).
type networkListener struct {
	ln   net.Listener
	srv  *http.Server
	src  *certSource // the daemon's own certificate; nil on a plain door
	spec doorSpec

	mu    sync.Mutex
	conns map[net.Conn]struct{} // the WebSockets open through this door, which closing it ends
}

// hold keeps c to close with the door; false when the door is already closed.
func (l *networkListener) hold(c net.Conn) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conns == nil {
		return false
	}
	l.conns[c] = struct{}{}
	return true
}

func (l *networkListener) drop(c net.Conn) {
	l.mu.Lock()
	delete(l.conns, c)
	l.mu.Unlock()
}

// close stops the door and ends every connection open through it: a host no longer chosen serves no one.
func (l *networkListener) close() {
	l.srv.Close()
	l.mu.Lock()
	conns := l.conns
	l.conns = nil
	l.mu.Unlock()
	for c := range conns {
		c.Close()
	}
}

// Fingerprint is the sha256 of the certificate the listener presents now, for the pairing QR.
func (n *networkListener) Fingerprint() string {
	if n.src == nil {
		return "" // a plain door: nothing to pin
	}
	c, err := n.src.get()
	if err != nil || len(c.Certificate) == 0 {
		return ""
	}
	sum := sha256.Sum256(c.Certificate[0])
	return hex.EncodeToString(sum[:])
}

// certSource is the certificate a listener presents, read from -cert and -key and read again when either file changes, so
// a renewed certificate (tailscale cert) is served without a restart.
type certSource struct {
	certFile, keyFile string
	mu                sync.Mutex
	cert              *tls.Certificate
	stamp             [2]time.Time
}

func (s *certSource) get() (*tls.Certificate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var stamp [2]time.Time
	for i, f := range []string{s.certFile, s.keyFile} {
		st, err := os.Stat(f)
		if err != nil {
			return s.cert, err
		}
		stamp[i] = st.ModTime()
	}
	if s.cert != nil && stamp == s.stamp {
		return s.cert, nil
	}
	c, err := tls.LoadX509KeyPair(s.certFile, s.keyFile)
	if err != nil {
		if s.cert != nil {
			return s.cert, nil // a half-written renewal: keep serving the last good one
		}
		return nil, err
	}
	s.cert, s.stamp = &c, stamp
	return s.cert, nil
}

// doorSpec is how the door on one IP is served.
type doorSpec struct {
	tls bool        // with the daemon's own certificate (-cert, or the one made for the LAN), its fingerprint pinned by a pairing
	ts  *certSource // Tailscale's own certificate for the MagicDNS name, answered beside plain HTTP (its first arrival reopens the door); nil for none
	web bool        // the web page is served: on loopback and Tailscale, never on Wi-Fi, where a browser could not tell a fake server
}

// netDoors is the network listener: a door on each IP the chosen hosts are served on, all serving one server. Unset, network_addresses
// leaves the one -listen door; set, the doors are made again when the setting or this machine's interfaces change, without a restart.
type netDoors struct {
	d    *Daemon
	cfg  Config
	rs   *rpc.Server
	port string
	ui   http.Handler // the web page every door serves; nil serves none
	src  *certSource  // the certificate a TLS door presents
	ts   *tsCert
	stop chan struct{}

	relistening sync.Mutex // one relisten at a time, so an older want is never applied over a newer one
	mu          sync.Mutex
	open        map[string]*networkListener // by the IP it binds
	served      []netAddress                // the hosts a door is open for, in the setting's order
	magicDNS    map[string]string           // a Tailscale IP's MagicDNS name, by the IP
	chosen      bool                        // network_addresses was set at the last relisten
	picked      string                      // the host a pairing names when it names none; "" for the default door
	closed      bool
}

const loopback = "127.0.0.1"

// relistenEvery is how often the doors are checked against the interfaces, so a Wi-Fi address that comes or goes is served or dropped.
var relistenEvery = 5 * time.Second

// listenNetwork opens the doors and sets d.net.
func listenNetwork(cfg Config, d *Daemon) (*netDoors, error) {
	if err := checkBind(cfg.Listen); err != nil {
		return nil, err
	}
	_, port, _ := net.SplitHostPort(cfg.Listen)
	n := &netDoors{d: d, cfg: cfg, port: port, rs: d.newServer("network"), open: map[string]*networkListener{}, stop: make(chan struct{})}
	n.ts = &tsCert{dir: filepath.Join(cfg.Home, "network"), ready: func() { n.relisten() }}
	if cfg.WebDir != "" {
		var err error
		if n.ui, err = webUI(cfg.WebDir); err != nil {
			return nil, fmt.Errorf("web UI directory: %w", err)
		}
	}
	if !cfg.plain() {
		n.src = &certSource{certFile: cfg.CertFile, keyFile: cfg.KeyFile}
		if _, err := n.src.get(); err != nil {
			return nil, fmt.Errorf("network listener certificate: %w", err)
		}
	}
	d.dev.Doors = n.pairDoor
	d.net = n
	if err := n.relisten(); err != nil {
		n.close()
		d.net = nil
		return nil, err
	}
	go n.watch()
	return n, nil
}

// watch makes the doors again each relistenEvery.
func (n *netDoors) watch() {
	t := time.NewTicker(relistenEvery)
	defer t.Stop()
	for {
		select {
		case <-n.stop:
			return
		case <-t.C:
			n.relisten()
		}
	}
}

// want is the hosts to serve and how the door on each IP is served: the -listen door while network_addresses is unset, else each chosen
// host this machine has an address for now; and Handline's pick for a pairing (pickHost) when no door is named for it, unless -advertise
// names the address. names is this machine's MagicDNS name by its Tailscale IP. There is always a plain loopback door for the desktop app.
func (n *netDoors) want() (hosts []netAddress, specs map[string]doorSpec, names map[string]string, picked string, chosen bool) {
	hosts, specs, names, picked, _, chosen = n.wantAll()
	return hosts, specs, names, picked, chosen
}

// wantAll is want, saying also whether the last of hosts is there only as the pick (OwnDoor leaves it out: runners reach a host chosen).
func (n *netDoors) wantAll() (hosts []netAddress, specs map[string]doorSpec, names map[string]string, picked string, onlyPicked, chosen bool) {
	specs = map[string]doorSpec{loopback: {web: true}} // the desktop app's door
	all := addressesNow()
	// the master's door for runners (master.go) is the plain one; Tailscale is asked only while its address is up
	https := n.ts != nil && n.cfg.plain() && slices.ContainsFunc(all, func(a netAddress) bool { return a.Kind == kindTailscale }) && tailnetHTTPS()
	have := map[string]netAddress{}
	names = map[string]string{}
	for _, a := range all {
		have[a.Host] = a
		if a.Kind == kindTailscaleName {
			for _, ip := range a.IPs {
				names[ip.String()] = a.Host
			}
		}
	}
	list := chosenHosts(n.cfg.addressesHome())
	chosen = list != nil
	if !chosen {
		host, _, _ := net.SplitHostPort(n.cfg.Listen)
		ip := net.ParseIP(host) // checkBind made it an IP literal
		a := have[ip.String()]
		a.Host, a.IPs = ip.String(), []net.IP{ip}
		if a.Kind == "" && tailscaleV4.Contains(ip) {
			a.Kind = kindTailscale
		}
		hosts = append(hosts, a)
		specs[a.Host] = doorSpec{tls: !n.cfg.plain(), web: ip.IsLoopback() || a.Kind == kindTailscale || !n.cfg.plain()}
		if n.cfg.Advertise == "" {
			picked = pickHost(all)
		}
	} else if len(list) > 0 { // an empty choice serves nothing, not even a pick
		picked = pickHost(all)
	}
	for _, h := range list {
		if a, ok := have[h]; ok {
			hosts = append(hosts, a)
		}
	}
	if a, ok := have[picked]; ok && !slices.ContainsFunc(hosts, func(h netAddress) bool { return h.Host == picked }) {
		hosts, onlyPicked = append(hosts, a), true
	}
	wifi := map[string]bool{}
	for i, a := range hosts {
		for _, ip := range a.IPs {
			k := ip.String()
			spec := specs[k]
			if chosen || i > 0 {
				spec.tls = spec.tls || !a.plain(n.cfg)
				// A self-made certificate on Wi-Fi serves no page: a browser's warning for it cannot tell a fake server from this Mac.
				wifi[k] = wifi[k] || n.cfg.plain() && (a.Kind == kindLAN || a.Kind == kindMDNS)
				spec.web = !wifi[k]
			}
			if name := names[k]; https && !spec.tls && name != "" && (a.Kind == kindTailscale || a.Kind == kindTailscaleName) {
				spec.ts = n.ts.source(name, n.cfg.Log)
			}
			specs[k] = spec
		}
	}
	return hosts, specs, names, picked, onlyPicked, chosen
}

// relisten opens the doors wanted and closes the rest. The loopback door, and with network_addresses unset the -listen door, that cannot
// be bound is the error: without it the desktop app loads forever. Any other that cannot be bound is logged and tried again on the next round.
func (n *netDoors) relisten() error {
	n.relistening.Lock()
	defer n.relistening.Unlock()
	hosts, want, names, picked, chosen := n.want()
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return nil
	}
	for ip, l := range n.open {
		if spec, ok := want[ip]; !ok || spec != l.spec {
			l.close()
			delete(n.open, ip)
			n.cfg.Log("network listener on %s closed", l.ln.Addr())
		}
	}
	must := map[string]bool{loopback: true} // the desktop app's door, and while network_addresses is unset the -listen one
	if !chosen {
		must[hosts[0].Host] = true
	}
	var failed error
	for ip, spec := range want {
		if n.open[ip] != nil {
			continue
		}
		l, err := n.listen(ip, spec)
		if err != nil {
			n.cfg.Log("network listener on %s: %v", ip, err)
			if must[ip] {
				failed = err
			}
			continue
		}
		n.open[ip] = l
	}
	n.served = n.served[:0]
	for _, a := range hosts {
		if n.servesLocked(a) {
			n.served = append(n.served, a)
		}
	}
	n.chosen, n.magicDNS, n.picked = chosen, names, picked
	return failed
}

// servesLocked says whether every IP of a has an open door. Called with n.mu held.
func (n *netDoors) servesLocked(a netAddress) bool {
	for _, ip := range a.IPs {
		if n.open[ip.String()] == nil {
			return false
		}
	}
	return len(a.IPs) > 0
}

func (n *netDoors) listen(ip string, spec doorSpec) (*networkListener, error) {
	addr := net.JoinHostPort(ip, n.port)
	if err := checkBind(addr); err != nil {
		return nil, err
	}
	l := &networkListener{conns: map[net.Conn]struct{}{}, spec: spec}
	var err error
	if spec.tls {
		if l.src, err = n.tlsSource(); err != nil {
			return nil, err
		}
		src := l.src
		l.ln, err = tls.Listen("tcp", addr, &tls.Config{GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return src.get() },
			MinVersion: tls.VersionTLS13})
	} else {
		// A plain door is never served anywhere the tailnet (or loopback, for a local proxy) is not the only way in.
		if err := checkPlainBind(addr); err != nil {
			return nil, err
		}
		l.ln, err = net.Listen("tcp", addr)
		if err == nil && spec.ts != nil {
			src := spec.ts
			l.ln = plainOrTLS(l.ln, &tls.Config{GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return src.get() },
				MinVersion: tls.VersionTLS12})
		}
	}
	if err != nil {
		return nil, err
	}
	up := websocket.Upgrader{CheckOrigin: sameOrigin} // a browser page may only be this door's own; the token decides the rest
	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", func(w http.ResponseWriter, r *http.Request) {
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		c := ws.UnderlyingConn()
		if !n.d.trackWS(c) {
			ws.Close()
			return
		}
		defer n.d.untrack(c)
		if !l.hold(c) {
			ws.Close()
			return
		}
		defer l.drop(c)
		n.rs.ServeNet(rpc.NewWSTransport(ws), remoteHost(r.RemoteAddr), func() { ws.Close() })
	})
	if n.ui != nil && spec.web {
		mux.Handle("/", n.ui)
	}
	l.srv = &http.Server{Handler: mux, ReadHeaderTimeout: rpc.RecvTimeout}
	go l.srv.Serve(l.ln)
	switch {
	case spec.tls:
		n.cfg.Log("network listener on %s, certificate %s", l.ln.Addr(), l.Fingerprint())
	case spec.ts != nil:
		n.cfg.Log("network listener on %s, plain HTTP and Tailscale's certificate", l.ln.Addr())
	default:
		n.cfg.Log("network listener on %s, plain HTTP (no certificate)", l.ln.Addr())
	}
	return l, nil
}

// tlsSource is -cert and -key when given, else the certificate the daemon made for its LAN doors (made the first time one is needed).
func (n *netDoors) tlsSource() (*certSource, error) {
	if n.src != nil {
		return n.src, nil
	}
	crt, key, err := ensureLANCert(n.cfg.Home)
	if err != nil {
		return nil, fmt.Errorf("network listener certificate: %w", err)
	}
	n.src = &certSource{certFile: crt, keyFile: key}
	if _, err := n.src.get(); err != nil {
		n.src = nil
		return nil, fmt.Errorf("network listener certificate: %w", err)
	}
	return n.src, nil
}

// door is the door that serves host. "" is the door a pairing names by default: -advertise (or the -listen door) while
// network_addresses is unset, else the first LAN host served, else the first chosen host served.
func (n *netDoors) door(host string) (devices.Door, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if host == "" && n.chosen {
		host = n.firstLAN()
	}
	for i, a := range n.served {
		l := n.open[a.IPs[0].String()]
		if l == nil {
			continue
		}
		name := n.nameLocked(a, l)
		bound := l.ln.Addr().String()
		_, port, _ := net.SplitHostPort(bound)
		addr := net.JoinHostPort(name, port)
		if !n.chosen && i == 0 && n.cfg.Advertise != "" {
			advHost, _, _ := net.SplitHostPort(n.cfg.Advertise)
			if host == "" || host == advHost {
				addr = n.cfg.Advertise
			}
			if host == advHost {
				host = ""
			}
		}
		if host == "" || host == a.Host || host == name {
			return devices.Door{Addr: addr, Fingerprint: l.Fingerprint(), TLS: l.src != nil || l.spec.ts != nil, Web: n.ui != nil && l.spec.web}, true
		}
	}
	return devices.Door{}, false
}

// nameLocked is the host a pairing for a names: its MagicDNS name when a is a Tailscale IP served plain or on Tailscale's certificate (an
// app trusts a link with no fingerprint only for *.ts.net, and the certificate is the name's), else a.Host. The caller holds n.mu.
func (n *netDoors) nameLocked(a netAddress, l *networkListener) string {
	if name := n.magicDNS[a.Host]; name != "" && a.Kind == kindTailscale && l.src == nil {
		return name
	}
	return a.Host
}

// firstLAN is the first served LAN host, "" when none is. The caller holds n.mu.
func (n *netDoors) firstLAN() string {
	for _, a := range n.served {
		if a.Kind == kindLAN {
			return a.Host
		}
	}
	return ""
}

// hosts is every host a door is open for, the one a pairing names by default first.
func (n *netDoors) hosts() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := []string{}
	if !n.chosen && len(n.served) > 0 {
		if adv, _, err := net.SplitHostPort(n.cfg.Advertise); err == nil && adv != n.served[0].Host {
			out = append(out, adv)
		}
	}
	lan := ""
	if n.chosen {
		if lan = n.firstLAN(); lan != "" {
			out = append(out, lan)
		}
	}
	seen := map[string]bool{lan: true}
	for _, a := range n.served {
		for _, h := range []string{a.Host, n.nameLocked(a, n.open[a.IPs[0].String()])} {
			if !seen[h] {
				seen[h] = true
				out = append(out, h)
			}
		}
	}
	return out
}

// pairedBefore is the host a device paired before addresses were kept was given: -advertise, else the -listen host.
func (n *netDoors) pairedBefore() string {
	addr := n.cfg.Advertise
	if addr == "" {
		addr = n.cfg.Listen
	}
	host, _, _ := net.SplitHostPort(addr)
	return host
}

// pairDoor is devices.Manager.Doors: the door of Handline's pick, and the default door when there is none or its door is not open.
func (n *netDoors) pairDoor() (devices.Door, bool) {
	n.mu.Lock()
	host := n.picked
	n.mu.Unlock()
	if d, ok := n.door(host); ok {
		return d, true
	}
	return n.door("")
}

// pickHost is the host a pairing names: this machine's Tailscale name while Tailscale is up, its Tailscale IP when the name was not read,
// else its first Wi-Fi (LAN) address; "" with none.
func pickHost(all []netAddress) string {
	ip, lan := "", ""
	for _, a := range all {
		switch {
		case a.Kind == kindTailscaleName:
			return a.Host
		case a.Kind == kindTailscale && ip == "":
			ip = a.Host
		case a.Kind == kindLAN && lan == "":
			lan = a.Host
		}
	}
	return cmp.Or(ip, lan)
}

func (n *netDoors) close() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.closed {
		n.closed = true
		close(n.stop)
	}
	for ip, l := range n.open {
		l.close()
		delete(n.open, ip)
	}
	n.served = nil
}

// plain is plain mode: a listener given no certificate serves plain HTTP/ws, and only on a Tailscale or loopback address.
func (c Config) plain() bool { return c.CertFile == "" && c.KeyFile == "" }

func (c Config) addressesHome() string {
	if c.AddressesHome != "" {
		return c.AddressesHome
	}
	return c.Home
}

// ifaceAddr is one IPv4 address of one network interface.
type ifaceAddr struct {
	Name string
	IP   net.IP
}

// interfaceAddrs lists the machine's interface addresses; a variable so a test can name some.
var interfaceAddrs = func() ([]ifaceAddr, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []ifaceAddr
	for _, i := range ifs {
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
				out = append(out, ifaceAddr{i.Name, n.IP})
			}
		}
	}
	return out, nil
}

// tailscaleIPs are the addresses held by the Tailscale interface itself (utun* on macOS, tailscale* elsewhere) in
// 100.64.0.0/10. The range alone is not enough: carrier-grade NAT hands it out on ordinary LAN and Wi-Fi interfaces.
func tailscaleIPs() ([]net.IP, error) {
	all, err := interfaceAddrs()
	if err != nil {
		return nil, err
	}
	var ips []net.IP
	for _, a := range all {
		name := strings.ToLower(a.Name)
		if (strings.HasPrefix(name, "utun") || strings.HasPrefix(name, "tailscale")) && tailscaleV4.Contains(a.IP) {
			ips = append(ips, a.IP)
		}
	}
	if len(ips) == 0 {
		return nil, errors.New("no VPN interface (utun*, tailscale*) has an address in 100.64.0.0/10; plain HTTP without -cert and -key serves nowhere else")
	}
	return ips, nil
}

// plainAddr completes ":PORT" with the Tailscale address; a given host is left for checkPlainBind to judge.
func plainAddr(listen string) (string, error) {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", fmt.Errorf("network listener address %q: %w (want :PORT or ADDR:PORT)", listen, err)
	}
	if host != "" {
		return listen, nil
	}
	ips, err := tailscaleIPs()
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(ips[0].String(), port), nil
}

// checkPlainBind admits only an address of the Tailscale interface, or loopback for a local proxy such as `tailscale serve`.
func checkPlainBind(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() {
			return nil
		}
		ips, _ := tailscaleIPs()
		for _, t := range ips {
			if t.Equal(ip) {
				return nil
			}
		}
	}
	return fmt.Errorf("network listener address %q: without -cert and -key the listener serves plain HTTP, which only a VPN interface address (100.64.0.0/10) or loopback may; any other address needs -cert and -key", addr)
}

// sameOrigin lets a call without an Origin through (a native client) and a browser's only from the page this
// listener served.
func sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	return err == nil && u.Host == r.Host
}

func remoteHost(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

func (d *Daemon) trackWS(c net.Conn) bool { return d.track(c) }

// NetworkAddr is where the door a pairing names by default is bound; "" with no door open.
func (d *Daemon) NetworkAddr() string {
	if l := d.defaultDoor(); l != nil {
		return l.ln.Addr().String()
	}
	return ""
}

func (d *Daemon) NetworkFingerprint() string {
	if l := d.defaultDoor(); l != nil {
		return l.Fingerprint()
	}
	return ""
}

func (d *Daemon) defaultDoor() *networkListener {
	if d.net == nil {
		return nil
	}
	d.net.mu.Lock()
	defer d.net.mu.Unlock()
	if len(d.net.served) == 0 {
		return nil
	}
	return d.net.open[d.net.served[0].IPs[0].String()]
}

// checkBind refuses an address the listener must never take: only an IP literal of this machine's own LAN,
// Tailscale or loopback (never a wildcard, and never a public address), because a phone's door is not the internet's.
func checkBind(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("network listener address %q: %w", addr, err)
	}
	ip := net.ParseIP(host)
	switch {
	case ip == nil:
		return fmt.Errorf("network listener address %q: the host must be an IP address of this machine's LAN or VPN", addr)
	case ip.IsUnspecified():
		return fmt.Errorf("network listener address %q binds every interface; name the LAN or VPN address", addr)
	case ip.IsLoopback(), ip.IsPrivate(), ip.IsLinkLocalUnicast(), tailscaleV4.Contains(ip):
		return nil
	}
	return fmt.Errorf("network listener address %q is a public address; only loopback, LAN and VPN (100.64.0.0/10) addresses are served", addr)
}

var _, tailscaleV4, _ = net.ParseCIDR("100.64.0.0/10")
