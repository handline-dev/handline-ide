package daemon

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// writeNameCert writes a certificate for name that expires at until, signed by itself, and answers the pool that trusts it.
func writeNameCert(t *testing.T, name, crt, key string, until time.Time) *x509.CertPool {
	t.Helper()
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), DNSNames: []string{name}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: until, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &k.PublicKey, k)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := x509.MarshalECPrivateKey(k)
	os.MkdirAll(filepath.Dir(crt), 0o700)
	if err := writeWhole(key, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeWhole(crt, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(c)
	return pool
}

// With tailnet HTTPS on, the Tailscale door answers TLS on Tailscale's own certificate for the name beside plain HTTP, the pairing link is
// https on the name with no fingerprint, and a certificate a month from its end is fetched again and served without a restart.
func TestWithTailnetHTTPSTheTailscaleDoorServesTailscalesCertificateAndRenewsIt(t *testing.T) {
	const name = "mac.tail1.ts.net"
	d, c := startDoors(t, true)
	dir := filepath.Join(d.cfg.Home, "network")
	crt, key := filepath.Join(dir, "tailscale.crt"), filepath.Join(dir, "tailscale.key")
	writeNameCert(t, name, crt, key, time.Now().Add(10*24*time.Hour)) // last fetched long ago: due for renewal
	var pool atomic.Pointer[x509.CertPool]
	var fetched atomic.Int32
	d.cfg.machine.setHTTPS(func() bool { return true })
	d.cfg.machine.setFetch(func(n, crtFile, keyFile string) error {
		if n != name || crtFile != crt || keyFile != key {
			t.Errorf("tailscale cert %s %s %s", n, crtFile, keyFile)
		}
		fetched.Add(1)
		pool.Store(writeNameCert(t, name, crtFile, keyFile, time.Now().Add(90*24*time.Hour)))
		return nil
	})
	renewed := make(chan struct{})
	d.net.ts.ready = func() { d.net.relisten(); close(renewed) }
	serveOn(d, netAddress{Host: "127.0.0.1", Kind: kindTailscale, Iface: "utun3", IPs: loop},
		netAddress{Host: name, Kind: kindTailscaleName, Iface: "utun3", IPs: loop})
	setAddresses(t, c, `["127.0.0.1"]`)

	res := call(t, c, "device.pair", `{}`)["result"].(map[string]any)
	addr := boundAddr(t, d, "127.0.0.1")
	_, port, _ := strings.Cut(addr, ":")
	if res["fingerprint"] != "" || res["web_link"] != "https://"+name+":"+port+"/#pair="+res["code"].(string) {
		t.Fatalf("the link is not https on the name with no fingerprint: %v", res)
	}
	select {
	case <-renewed:
	case <-time.After(10 * time.Second):
		t.Fatal("a certificate near its end is not fetched again")
	}
	if fetched.Load() != 1 {
		t.Fatalf("fetched %d times", fetched.Load())
	}
	d.net.relisten()
	if fetched.Load() != 1 {
		t.Fatal("a renewed certificate is fetched again on the next round")
	}

	// Verified the normal way, with the renewed certificate; the page is served on the Tailscale door.
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: name, RootCAs: pool.Load()})
	if err != nil {
		t.Fatalf("TLS on the Tailscale door: %v", err)
	}
	conn.Close()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{ServerName: name, RootCAs: pool.Load()}}}
	r, err := client.Get("https://" + addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if !strings.Contains(string(b), "<html>handline</html>") {
		t.Fatalf("no page over TLS on the Tailscale door: %q", b)
	}
	// An app paired on ws:// keeps reaching the same door plain.
	if body, _, err := fetch(addr, false); err != nil || !strings.Contains(body, "<html>handline</html>") {
		t.Fatalf("plain HTTP on the Tailscale door: %v %q", err, body)
	}

	// HTTPS turned off: the door is plain alone again, and the link http.
	d.cfg.machine.setHTTPS(func() bool { return false })
	d.net.relisten()
	res = call(t, c, "device.pair", `{}`)["result"].(map[string]any)
	if l, _ := res["web_link"].(string); !strings.HasPrefix(l, "http://"+name+":") {
		t.Fatalf("%v", res)
	}
	if _, err := tls.Dial("tcp", boundAddr(t, d, "127.0.0.1"), &tls.Config{ServerName: name, RootCAs: pool.Load()}); err == nil {
		t.Fatal("TLS is still answered with tailnet HTTPS off")
	}
}

// A certificate that cannot be fetched leaves the door plain, as it was, and is asked for again only an hour later.
func TestATailscaleCertificateThatCannotBeFetchedLeavesTheDoorPlain(t *testing.T) {
	d, c := startDoors(t, false)
	var fetched atomic.Int32
	failed := make(chan struct{}, 4)
	d.cfg.machine.setHTTPS(func() bool { return true })
	d.cfg.machine.setFetch(func(string, string, string) error {
		fetched.Add(1)
		defer func() { failed <- struct{}{} }()
		return os.ErrPermission
	})
	serveOn(d, netAddress{Host: "127.0.0.1", Kind: kindTailscale, Iface: "utun3", IPs: loop},
		netAddress{Host: "mac.tail1.ts.net", Kind: kindTailscaleName, Iface: "utun3", IPs: loop})
	setAddresses(t, c, `["127.0.0.1"]`)
	<-failed
	d.net.relisten()
	res := call(t, c, "device.pair", `{}`)["result"].(map[string]any)
	if l, _ := res["web_link"].(string); !strings.HasPrefix(l, "http://mac.tail1.ts.net:") || fetched.Load() != 1 {
		t.Fatalf("%v, fetched %d", res, fetched.Load())
	}
}
