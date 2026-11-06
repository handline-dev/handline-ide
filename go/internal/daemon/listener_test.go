package daemon

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func ephemeralCert() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "handline daemon"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}

func writeCert(t *testing.T, dir string) (crt, key string) {
	t.Helper()
	cert, err := ephemeralCert()
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalECPrivateKey(cert.PrivateKey.(*ecdsa.PrivateKey))
	crt, key = filepath.Join(dir, "net.crt"), filepath.Join(dir, "net.key")
	os.WriteFile(crt, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}), 0o644)
	os.WriteFile(key, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600)
	return
}

// tlsConfig is a daemon in dir whose listener is TLS on loopback, with a certificate made for the test.
func tlsConfig(t *testing.T, dir string) Config {
	t.Helper()
	crt, key := writeCert(t, dir)
	return Config{Home: dir, DBFile: filepath.Join(dir, "db.sqlite"), Listen: "127.0.0.1:0", CertFile: crt, KeyFile: key, Auth: fakeAuth{}}
}

// startListening is a daemon with a TLS listener serving web (none when empty).
func startListening(t *testing.T, web string) (*Daemon, Config) {
	t.Helper()
	cfg := tlsConfig(t, t.TempDir())
	cfg.WebDir = web
	d, err := Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return d, cfg
}

func (c *wsClient) read() map[string]any {
	c.t.Helper()
	c.ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, b, err := c.ws.ReadMessage()
	if err != nil {
		c.t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(b, &m)
	return m
}

func (c *wsClient) send(msg string) { c.ws.WriteMessage(websocket.TextMessage, []byte(msg)) }

func gate(m map[string]any) any {
	if e, ok := m["error"].(map[string]any); ok {
		return e["data"].(map[string]any)["gate"]
	}
	return nil
}

func helloAs(t *testing.T, d *Daemon, token string) *wsClient {
	t.Helper()
	c := dialNet(t, d)
	if r := c.ask(`{"jsonrpc":"2.0","id":"h","method":"hello","params":{"client":"t","protocol":1,"token":"` + token + `"}}`); r["result"] == nil {
		t.Fatalf("%v", r)
	}
	return c
}

func mustPair(t *testing.T, d *Daemon) string { token, _ := pair(t, d, "phone"); return token }

func TestARenewedCertificateIsServedWithoutARestart(t *testing.T) {
	d, cfg := startListening(t, "")
	fp := d.NetworkFingerprint()
	crt, key := writeCert(t, t.TempDir())
	newCrt, _ := os.ReadFile(crt)
	newKey, _ := os.ReadFile(key)
	os.WriteFile(cfg.CertFile, newCrt, 0o644)
	os.WriteFile(cfg.KeyFile, newKey, 0o600)
	future := time.Now().Add(time.Minute)
	os.Chtimes(cfg.CertFile, future, future)
	os.Chtimes(cfg.KeyFile, future, future)
	if got := d.NetworkFingerprint(); got == fp || got == "" {
		t.Fatalf("%s -> %s", fp, got)
	}
	conn, err := tls.Dial("tcp", d.NetworkAddr(), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	sum := sha256.Sum256(conn.ConnectionState().PeerCertificates[0].Raw)
	if hex.EncodeToString(sum[:]) != d.NetworkFingerprint() {
		t.Fatal("the listener still presents the old certificate")
	}
	os.WriteFile(cfg.CertFile, []byte("half"), 0o644) // a renewal caught half written
	os.Chtimes(cfg.CertFile, future.Add(time.Minute), future.Add(time.Minute))
	if d.NetworkFingerprint() == "" {
		t.Fatal("a half-written certificate took the listener down")
	}
}

func TestThePairingCodeNamesTheAdvertisedAddressAndTheFingerprint(t *testing.T) {
	cfg := tlsConfig(t, t.TempDir())
	cfg.Advertise = "mac.tail1234.ts.net:7443"
	d, err := Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	code, _, fp, uri := d.dev.Pair(time.Minute)
	u, _ := url.Parse(uri)
	q := u.Query()
	if q.Get("host") != "mac.tail1234.ts.net:7443" || q.Get("fp") != d.NetworkFingerprint() || fp != q.Get("fp") || q.Get("code") != code {
		t.Fatal(uri)
	}
}

func get(t *testing.T, d *Daemon, method, path string) *http.Response {
	t.Helper()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	req, _ := http.NewRequest(method, "https://"+d.NetworkAddr()+"/", nil)
	req.URL.Opaque = "//" + d.NetworkAddr() + path // sent as it is, so ".." and %2e survive
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestTheWebUIIsServedOnTheListenersOriginAndOnlyWhatIsInsideIt(t *testing.T) {
	web := filepath.Join(t.TempDir(), "web")
	os.MkdirAll(filepath.Join(web, "assets", "sub"), 0o755)
	os.WriteFile(filepath.Join(web, "index.html"), []byte("<html>app</html>"), 0o644)
	os.WriteFile(filepath.Join(web, "assets", "app.js"), []byte("1"), 0o644)
	os.WriteFile(filepath.Join(web, "assets", "app.css"), []byte("1"), 0o644)
	os.WriteFile(filepath.Join(web, "assets", "x.wasm"), []byte("1"), 0o644)
	os.WriteFile(filepath.Join(web, "assets", "sub", "index.html"), []byte("sub"), 0o644)
	os.WriteFile(filepath.Join(web, ".env"), []byte("secret"), 0o644)
	os.MkdirAll(filepath.Join(web, "empty"), 0o755)
	secret := filepath.Join(filepath.Dir(web), "secret.txt")
	os.WriteFile(secret, []byte("outside"), 0o644)
	os.Symlink(secret, filepath.Join(web, "link.txt"))
	d, _ := startListening(t, web)
	body := func(r *http.Response) string { b, _ := io.ReadAll(r.Body); return string(b) }

	r := get(t, d, "GET", "/")
	if r.StatusCode != 200 || body(r) != "<html>app</html>" || r.Header.Get("Cache-Control") != "no-store" || r.Header.Get("Last-Modified") != "" ||
		!strings.HasPrefix(r.Header.Get("Content-Type"), "text/html") || r.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("index: %v", r.Header)
	}
	if r := get(t, d, "GET", "/index.html"); r.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("index.html is cached")
	}
	for path, ct := range map[string]string{"/assets/app.js": "text/javascript", "/assets/app.css": "text/css", "/assets/x.wasm": "application/wasm"} {
		if r := get(t, d, "GET", path); r.StatusCode != 200 || !strings.HasPrefix(r.Header.Get("Content-Type"), ct) {
			t.Errorf("%s: %d %s", path, r.StatusCode, r.Header.Get("Content-Type"))
		}
	}
	if r := get(t, d, "GET", "/assets/sub/"); body(r) != "sub" {
		t.Fatal("a directory with an index serves it")
	}
	for _, path := range []string{"/missing.js", "/empty/", "/assets/", "/.env", "/assets/../.env", "/../secret.txt", "/%2e%2e/secret.txt",
		"/assets/%2e%2e/%2e%2e/secret.txt", "/link.txt", "/%2e%2e%2fsecret.txt", "/assets/..%2f..%2fsecret.txt", "//etc/passwd"} {
		r := get(t, d, "GET", path)
		if r.StatusCode == 200 && strings.Contains(body(r), "outside") || r.StatusCode == 200 && strings.Contains(body(r), "secret") {
			t.Errorf("%s leaked", path)
		}
		if r.StatusCode == 200 {
			t.Errorf("%s: %d", path, r.StatusCode)
		}
	}
	if r := get(t, d, "GET", "/assets/"); r.StatusCode != 404 {
		t.Fatalf("a listing: %d", r.StatusCode)
	}
	if r := get(t, d, "POST", "/"); r.StatusCode != 405 {
		t.Fatal(r.StatusCode)
	}
	if r := get(t, d, "GET", "/?token=abc"); r.StatusCode != 400 {
		t.Fatal("a token in the URL was accepted")
	}
}

func TestAWebBuildThatReplacesTheDirectoryIsServedWithoutARestart(t *testing.T) {
	web := filepath.Join(t.TempDir(), "web")
	os.MkdirAll(web, 0o755)
	os.WriteFile(filepath.Join(web, "index.html"), []byte("old"), 0o644)
	d, _ := startListening(t, web)
	os.RemoveAll(web) // a build empties dir and makes it again: a new directory under the same name
	os.MkdirAll(web, 0o755)
	os.WriteFile(filepath.Join(web, "index.html"), []byte("new"), 0o644)
	r := get(t, d, "GET", "/")
	if b, _ := io.ReadAll(r.Body); r.StatusCode != 200 || string(b) != "new" {
		t.Fatalf("after a rebuild: %d %q", r.StatusCode, b)
	}
}

func TestAWebDirectoryThatCannotBeOpenedStopsTheStart(t *testing.T) {
	if _, err := webUI(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("a missing web directory was taken")
	}
}

func TestNothingIsServedWithoutAWebDirectoryAndASocketFromAnotherPageIsRefused(t *testing.T) {
	d, _ := startListening(t, "")
	if r := get(t, d, "GET", "/"); r.StatusCode != 404 {
		t.Fatal(r.StatusCode)
	}
	dialer := websocket.Dialer{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	for origin, ok := range map[string]bool{"https://" + d.NetworkAddr(): true, "https://evil.example": false, "null": false} {
		ws, _, err := dialer.Dial("wss://"+d.NetworkAddr()+"/rpc", http.Header{"Origin": {origin}})
		if (err == nil) != ok {
			t.Errorf("origin %s: %v", origin, err)
		}
		if ws != nil {
			ws.Close()
		}
	}
}

// startPlain is a daemon with no certificate (Tailscale mode) on loopback, where a test can reach it.
func startPlain(t *testing.T, web string) *Daemon {
	t.Helper()
	dir := t.TempDir()
	d, err := Start(Config{Home: dir, DBFile: filepath.Join(dir, "db.sqlite"), Listen: "127.0.0.1:0", WebDir: web, Auth: fakeAuth{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return d
}

func TestWithoutACertificateTheListenerServesPlainHTTPAndWebSocketWithNothingToPin(t *testing.T) {
	web := t.TempDir()
	os.WriteFile(filepath.Join(web, "index.html"), []byte("<html>pair</html>"), 0o644)
	d := startPlain(t, web)
	if d.NetworkFingerprint() != "" {
		t.Fatal("a plain listener has no certificate to fingerprint")
	}
	if resp, err := http.Get("http://" + d.NetworkAddr() + "/"); err != nil || resp.StatusCode != 200 {
		t.Fatalf("%v %v", resp, err)
	}
	if _, err := (&http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}).Get("https://" + d.NetworkAddr() + "/"); err == nil {
		t.Fatal("TLS answered on a plain listener")
	}
	for origin, ok := range map[string]bool{"http://" + d.NetworkAddr(): true, "http://evil.example": false, "https://" + d.NetworkAddr(): true} {
		ws, _, err := websocket.DefaultDialer.Dial("ws://"+d.NetworkAddr()+"/rpc", http.Header{"Origin": {origin}})
		if (err == nil) != ok {
			t.Errorf("origin %s: %v", origin, err)
		}
		if ws != nil {
			ws.Close()
		}
	}
	ws, _, err := websocket.DefaultDialer.Dial("ws://"+d.NetworkAddr()+"/rpc", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	c := &wsClient{t, ws}
	if r := c.ask(`{"jsonrpc":"2.0","id":1,"method":"agents.list"}`); gate(r) != "unpaired" {
		t.Fatalf("%v", r)
	}
	_, _, _, uri := d.dev.Pair(time.Minute)
	if u, _ := url.Parse(uri); u.Query().Has("fp") || u.Query().Get("host") != d.NetworkAddr() {
		t.Fatal(uri)
	}
}

func fakeInterfaces(t *testing.T, as ...ifaceAddr) {
	was := interfaceAddrs
	t.Cleanup(func() { interfaceAddrs = was })
	interfaceAddrs = func() ([]ifaceAddr, error) { return as, nil }
}

func ia(name, ip string) ifaceAddr { return ifaceAddr{name, net.ParseIP(ip)} }

func TestThePlainListenerRefusesEveryAddressButTheTailscaleInterfaceAndLoopback(t *testing.T) {
	// 100.64.9.9 on en0 is carrier-grade NAT on an ordinary interface: in the range, not Tailscale.
	fakeInterfaces(t, ia("utun4", "100.64.0.1"), ia("tailscale0", "100.127.255.254"), ia("en0", "100.64.9.9"), ia("en0", "192.168.1.5"))
	for addr, ok := range map[string]bool{"100.64.0.1:1": true, "100.127.255.254:1": true, "127.0.0.1:1": true, "[::1]:1": true,
		"100.64.9.9:1": false, "192.168.1.5:1": false, "10.0.0.2:1": false, "172.16.0.1:1": false, "169.254.1.1:1": false,
		"100.128.0.1:1": false, "100.63.255.255:1": false, "100.64.0.2:1": false, "0.0.0.0:1": false, "8.8.8.8:1": false, "host.ts.net:1": false} {
		if err := checkPlainBind(addr); (err == nil) != ok {
			t.Errorf("%s: %v", addr, err)
		}
	}
	// and Start refuses before it binds anything
	for _, addr := range []string{"192.168.1.5:0", "10.0.0.2:0", "0.0.0.0:0"} {
		dir := t.TempDir()
		d, err := Start(Config{Home: dir, DBFile: filepath.Join(dir, "db.sqlite"), Listen: addr, Auth: fakeAuth{}}) // the address may not be this machine's, so the refusal must be the plain-HTTP rule, not a failed bind
		if err == nil {
			d.Close()
			t.Errorf("%s: plain listener started", addr)
		} else if !strings.Contains(err.Error(), "plain HTTP") && !strings.Contains(err.Error(), "binds every interface") {
			t.Errorf("%s: %v", addr, err)
		}
	}
}

func TestTailscaleModeBindsTheTailscaleInterfaceAndNeverFallsBack(t *testing.T) {
	fakeInterfaces(t, ia("en0", "100.64.9.9"), ia("en1", "192.168.1.5"), ia("utun3", "100.101.102.103"))
	if a, err := plainAddr(":7443"); err != nil || a != "100.101.102.103:7443" {
		t.Fatalf("%q %v", a, err)
	}
	if a, err := plainAddr("127.0.0.1:7443"); err != nil || a != "127.0.0.1:7443" {
		t.Fatalf("%q %v", a, err)
	}
	// the only 100.64/10 address is on a LAN interface: no Tailscale, no fallback to it
	fakeInterfaces(t, ia("en0", "100.64.9.9"), ia("en1", "192.168.1.5"))
	if a, err := plainAddr(":7443"); err == nil {
		t.Fatalf("fell back to %q", a)
	}
	dir := t.TempDir()
	if d, err := Start(Config{Home: dir, DBFile: filepath.Join(dir, "db.sqlite"), Listen: ":0", Auth: fakeAuth{}}); err == nil {
		d.Close()
		t.Fatal("started with no Tailscale address")
	}
}
