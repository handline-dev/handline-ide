package daemon

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestTheAddressListKeepsLANAndTailscaleAndLeavesLoopbackAndPublicOut(t *testing.T) {
	was, wasTS, wasLocal := interfaceAddrs, tailscaleName, localName
	t.Cleanup(func() { interfaceAddrs, tailscaleName, localName = was, wasTS, wasLocal })
	interfaceAddrs = func() ([]ifaceAddr, error) {
		return []ifaceAddr{{"lo0", net.ParseIP("127.0.0.1")}, {"en0", net.ParseIP("192.168.0.5")}, {"en0", net.ParseIP("8.8.8.8")},
			{"en5", net.ParseIP("100.70.0.1")}, {"utun3", net.ParseIP("100.101.1.2")}, {"en7", net.ParseIP("10.0.0.3")},
			{"en8", net.ParseIP("172.20.1.1")}, {"en9", net.ParseIP("172.32.0.1")}}, nil
	}
	tailscaleName = func() string { return "mac.tail1.ts.net" }
	localName = func() string { return "mac.local" }
	var got []string
	for _, a := range machineAddresses() {
		got = append(got, fmt.Sprintf("%s %s %s %v", a.Host, a.Kind, a.Iface, a.IPs))
	}
	want := []string{"192.168.0.5 lan en0 [192.168.0.5]", "10.0.0.3 lan en7 [10.0.0.3]", "172.20.1.1 lan en8 [172.20.1.1]",
		"100.101.1.2 tailscale utun3 [100.101.1.2]", "mac.tail1.ts.net tailscale_name utun3 [100.101.1.2]",
		"mac.local mdns  [192.168.0.5 10.0.0.3 172.20.1.1]"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	tailscaleName, localName = func() string { return "" }, func() string { return "" }
	interfaceAddrs = func() ([]ifaceAddr, error) { return []ifaceAddr{{"lo0", net.ParseIP("127.0.0.1")}}, nil }
	if a := machineAddresses(); len(a) != 0 {
		t.Fatalf("loopback alone listed %v", a)
	}
}

// serveHosts makes the address list these hosts, all on loopback so the test can bind them.
func serveHosts(t *testing.T, hosts ...netAddress) {
	t.Helper()
	was := addressesNow
	t.Cleanup(func() { addressesNow = was })
	addressesNow = func() []netAddress { return hosts }
}

var loop = []net.IP{net.ParseIP("127.0.0.1")}

// startDoors is a daemon on loopback (plain, as with no -cert) with the founder's local socket and, when web is set, the web page.
func startDoors(t *testing.T, web bool) (*Daemon, *client) {
	t.Helper()
	dir, err := os.MkdirTemp("", "hl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	cfg := Config{Home: dir, DBFile: filepath.Join(dir, "db.sqlite"), SocketFile: filepath.Join(dir, "d.sock"), Listen: "127.0.0.1:0",
		Auth: fakeAuth{key: "k"}}
	if web {
		cfg.WebDir = filepath.Join(dir, "web")
		os.MkdirAll(cfg.WebDir, 0o755)
		os.WriteFile(filepath.Join(cfg.WebDir, "index.html"), []byte("<html>handline</html>"), 0o644)
	}
	d, err := Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	c := dial(t, d)
	c.raw(t, `{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"t","protocol":1,"key":"k"}}`)
	return d, c
}

var reqN int

func call(t *testing.T, c *client, method, params string) map[string]any {
	t.Helper()
	reqN++
	params = strings.Replace(params, "{", fmt.Sprintf(`{"request_id":"r%d",`, reqN), 1)
	params = strings.Replace(params, ",}", "}", 1)
	return c.raw(t, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"%s","params":%s}`, reqN, method, params))
}

func setAddresses(t *testing.T, c *client, list string) map[string]any {
	t.Helper()
	r := call(t, c, "settings.set", `{"changes":{"network_addresses":`+list+`}}`)
	res, ok := r["result"].(map[string]any)
	if !ok {
		t.Fatalf("settings.set %s: %v", list, r)
	}
	return res
}

func listeners(t *testing.T, c *client) []any {
	t.Helper()
	res := call(t, c, "connection.get", `{}`)["result"].(map[string]any)
	return res["listeners"].([]any)
}

// fetch reads the page at the door: over TLS (returning the certificate's fingerprint) or plain.
func fetch(addr string, isTLS bool) (body, fp string, err error) {
	if !isTLS {
		r, err := http.Get("http://" + addr + "/")
		if err != nil {
			return "", "", err
		}
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		return string(b), "", nil
	}
	conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return "", "", err
	}
	defer conn.Close()
	sum := sha256.Sum256(conn.ConnectionState().PeerCertificates[0].Raw)
	fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", addr)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	b, _ := io.ReadAll(conn)
	return string(b), hex.EncodeToString(sum[:]), nil
}

func boundAddr(t *testing.T, d *Daemon, ip string) string {
	t.Helper()
	d.net.mu.Lock()
	defer d.net.mu.Unlock()
	l := d.net.open[ip]
	if l == nil {
		return ""
	}
	return l.ln.Addr().String()
}

func TestChosenHostsAreServedLiveTLSOnTheLANPlainOnTailscaleAndDroppedWhenGone(t *testing.T) {
	d, c := startDoors(t, true)
	before := boundAddr(t, d, "127.0.0.1")
	if before == "" || len(listeners(t, c)) != 1 {
		t.Fatalf("unset network_addresses is not the -listen door: %v", listeners(t, c))
	}
	serveHosts(t, netAddress{Host: "127.0.0.1", Kind: kindLAN, Iface: "en0", IPs: loop})
	if r := setAddresses(t, c, `["127.0.0.1"]`); !reflect.DeepEqual(r["unreachable_devices"], []any{}) {
		t.Fatalf("%v", r)
	}
	read := call(t, c, "connection.addresses", `{}`)["result"].(map[string]any)
	want := map[string]any{"host": "127.0.0.1", "kind": "lan", "interface": "en0", "listening": true, "encryption": "tls"}
	if !reflect.DeepEqual(read["addresses"], []any{want}) || !reflect.DeepEqual(read["network_addresses"], []any{"127.0.0.1"}) {
		t.Fatalf("connection.addresses: %v", read)
	}
	ls := listeners(t, c)
	l, _ := ls[0].(map[string]any)
	addr := boundAddr(t, d, "127.0.0.1")
	if len(ls) != 1 || l["encryption"] != "tls" || l["web"] != true || l["address"] != addr {
		t.Fatalf("a LAN host is not served over TLS: %v", ls)
	}
	body, fp, err := fetch(addr, true)
	if err != nil || !strings.Contains(body, "<html>handline</html>") || fp != l["fingerprint"] {
		t.Fatalf("web page over TLS: %v %q fp %s", err, body, fp)
	}
	crt, _ := os.ReadFile(filepath.Join(d.cfg.Home, "network", "lan.crt"))
	if b, _ := pem.Decode(crt); b == nil || hex.EncodeToString(sha256Of(b.Bytes)) != fp {
		t.Fatalf("the LAN door does not present the certificate made in the home")
	}
	if _, _, err := fetch(before, false); err == nil && before != addr {
		t.Fatalf("the -listen door is still served after the choice")
	}
	dialer := websocket.Dialer{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	ws, _, err := dialer.Dial("wss://"+addr+"/rpc", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	// The address goes away (Wi-Fi off): the LAN door closes with the connection open through it, and only the desktop app's
	// loopback door is left, plain and listed nowhere.
	serveHosts(t)
	d.net.relisten()
	ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := ws.ReadMessage(); err == nil || isTimeout(err) {
		t.Fatalf("a connection through a dropped door stays open: %v", err)
	}
	if _, _, err := fetch(addr, true); err == nil || len(listeners(t, c)) != 0 {
		t.Fatalf("a host the machine no longer has is still served: %v", listeners(t, c))
	}
	if _, _, err := fetch(boundAddr(t, d, loopback), false); err != nil {
		t.Fatalf("no loopback door for the app: %v", err)
	}

	// It comes back as Tailscale's (the address list is read again on each round): served plain.
	serveHosts(t, netAddress{Host: "127.0.0.1", Kind: kindTailscale, Iface: "utun3", IPs: loop})
	d.net.relisten()
	if l := listeners(t, c); len(l) != 1 || l[0].(map[string]any)["encryption"] != "none" || l[0].(map[string]any)["fingerprint"] != nil {
		t.Fatalf("a Tailscale host is not served plain: %v", l)
	}

	setAddresses(t, c, `[]`)
	if len(listeners(t, c)) != 0 {
		t.Fatalf("an empty choice still serves %v", listeners(t, c))
	}
	setAddresses(t, c, `null`)
	if l := listeners(t, c); len(l) != 1 || l[0].(map[string]any)["address"] != boundAddr(t, d, loopback) {
		t.Fatalf("clearing the choice does not bring back the -listen door: %v", l)
	}
}

func isTimeout(err error) bool { ne, ok := err.(net.Error); return ok && ne.Timeout() }

func sha256Of(b []byte) []byte { s := sha256.Sum256(b); return s[:] }

func TestASettingNamingAHostThisMachineDoesNotHaveIsRefused(t *testing.T) {
	_, c := startDoors(t, false)
	serveHosts(t, netAddress{Host: "192.168.0.5", Kind: kindLAN, IPs: []net.IP{net.ParseIP("192.168.0.5")}})
	r := call(t, c, "settings.set", `{"changes":{"network_addresses":["10.9.9.9"]}}`)
	if gate(r) != "address" || !strings.Contains(fmt.Sprint(r), "192.168.0.5") {
		t.Fatalf("%v", r)
	}
	for _, bad := range []string{`"10.9.9.9"`, `["a","a"]`, `[""]`, `[1]`} {
		if r := call(t, c, "settings.set", `{"changes":{"network_addresses":`+bad+`}}`); r["error"] == nil {
			t.Fatalf("%s was taken: %v", bad, r)
		}
	}
}

func TestDevicePairNamesTheChosenHostWithAnAppURIAndAWebLinkAndRefusesAnyOther(t *testing.T) {
	d, c := startDoors(t, true)
	serveHosts(t, netAddress{Host: "127.0.0.1", Kind: kindLAN, IPs: loop}, netAddress{Host: "mac.local", Kind: kindMDNS, IPs: loop})
	setAddresses(t, c, `["mac.local"]`)
	_, port, _ := net.SplitHostPort(boundAddr(t, d, "127.0.0.1"))
	res := call(t, c, "device.pair", `{"address":"mac.local"}`)["result"].(map[string]any)
	u, _ := url.Parse(res["uri"].(string))
	q := u.Query()
	if q.Get("host") != "mac.local:"+port || q.Get("fp") == "" || q.Get("fp") != res["fingerprint"] {
		t.Fatalf("%v", res)
	}
	if res["web_link"] != "https://mac.local:"+port+"/#pair="+res["code"].(string) {
		t.Fatalf("web link %v", res["web_link"])
	}
	if _, dev, err := d.dev.Redeem(res["code"].(string), "phone", "", "10.0.0.9"); err != nil || dev.Address != "mac.local" {
		t.Fatalf("%v %v", dev, err)
	}
	if res := call(t, c, "device.pair", `{}`)["result"].(map[string]any); !strings.Contains(res["uri"].(string), "mac.local") {
		t.Fatalf("with no address the pairing names the first chosen host: %v", res)
	}
	r := call(t, c, "device.pair", `{"address":"127.0.0.1"}`)
	data, _ := r["error"].(map[string]any)["data"].(map[string]any)
	if gate(r) != "address" || !reflect.DeepEqual(data["addresses"], []any{"mac.local"}) {
		t.Fatalf("a host not chosen was paired: %v", r)
	}

	// Choosing another host answers with the device that was paired on the one dropped.
	if r := setAddresses(t, c, `["127.0.0.1"]`); !reflect.DeepEqual(r["unreachable_devices"], []any{"phone"}) {
		t.Fatalf("%v", r)
	}
}

func TestWithoutTheWebPageThePairingHasNoWebLink(t *testing.T) {
	_, c := startDoors(t, false)
	res := call(t, c, "device.pair", `{}`)["result"].(map[string]any)
	if res["web_link"] != nil || !strings.HasPrefix(res["uri"].(string), "handline://pair?") {
		t.Fatalf("%v", res)
	}
	_, c = startDoors(t, true)
	res = call(t, c, "device.pair", `{}`)["result"].(map[string]any)
	if l, _ := res["web_link"].(string); !strings.HasPrefix(l, "http://127.0.0.1:") {
		t.Fatalf("%v", res)
	}
}

func TestUnsetAddressesKeepADoorForTheDesktopAppThatIsListedNowhere(t *testing.T) {
	n := &netDoors{cfg: Config{Home: t.TempDir(), Listen: "100.64.0.1:7443"}}
	hosts, doors, chosen := n.want()
	if chosen || len(hosts) != 1 || hosts[0].Host != "100.64.0.1" {
		t.Fatalf("only the -listen host is listed: %v %v", hosts, chosen)
	}
	if isTLS, ok := doors[loopback]; !ok || isTLS {
		t.Fatalf("no plain loopback door: %v", doors)
	}
}

func TestWithChosenAddressesALoopbackDoorThatCannotBeBoundFailsTheStartAndAnotherHostIsOnlyLogged(t *testing.T) {
	held, err := net.Listen("tcp", "127.0.0.1:0") // another program on the daemon's port
	if err != nil {
		t.Fatal(err)
	}
	port := held.Addr().(*net.TCPAddr).Port
	// 10.9.9.9 is listed as this machine's, so its door is wanted, but no interface has it: its bind fails
	serveHosts(t, netAddress{Host: "10.9.9.9", Kind: kindLAN, Iface: "en0", IPs: []net.IP{net.ParseIP("10.9.9.9")}})
	var mu sync.Mutex
	var logged []string
	start := func() (*Daemon, error) {
		dir, err := os.MkdirTemp("", "hl") // short: the socket path has a length limit
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
		os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("network_addresses:\n  - 10.9.9.9\n"), 0o644)
		return Start(Config{Home: dir, DBFile: filepath.Join(dir, "db.sqlite"), SocketFile: filepath.Join(dir, "d.sock"),
			Listen: fmt.Sprintf("127.0.0.1:%d", port), Auth: fakeAuth{key: "k"}, Log: func(f string, a ...any) {
				mu.Lock()
				logged = append(logged, fmt.Sprintf(f, a...))
				mu.Unlock()
			}})
	}
	if d, err := start(); err == nil || !strings.Contains(err.Error(), "127.0.0.1") {
		if d != nil {
			d.Close()
		}
		t.Fatalf("the start went on without the desktop app's door: %v", err)
	}
	held.Close()
	d, err := start()
	if err != nil {
		t.Fatalf("with the port free: %v", err)
	}
	defer d.Close()
	if boundAddr(t, d, loopback) == "" || boundAddr(t, d, "10.9.9.9") != "" {
		t.Fatalf("want the loopback door alone: loopback %q", boundAddr(t, d, loopback))
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(strings.Join(logged, "\n"), "network listener on 10.9.9.9") {
		t.Fatalf("a chosen host that cannot be bound is logged and the start goes on: %q", logged)
	}
}
