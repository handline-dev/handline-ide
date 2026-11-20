package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/handline/handline/go/kit/protected"
)

// founderRig is a rig whose terminal holds the founder key.
func founderRig(t *testing.T) *rig {
	r := newRig(t)
	t.Setenv("HANDLINE_TEST_ROOT", r.root)
	dir := filepath.Join(protected.RootFor(r.home), "founder-keys")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "key"), []byte("k\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return r
}

// device pair makes a code through the daemon; listing and revoking are the app's, so they are not understood here.
func TestDevicePairGoesThroughTheDaemonAndNothingElseIsADeviceVerb(t *testing.T) {
	r := founderRig(t)
	calls := r.daemon(map[string]string{
		"device.pair": `{"code":"123456","expires_at":"2026-10-17T10:05:00Z","uri":"handline://pair?host=1.2.3.4","fingerprint":null}`,
	})
	out, _, code := r.run(nil, "device", "pair", "--name", "watch")
	if code != 0 || out != "code: 123456   (one time, expires 2026-10-17T10:05:00Z)\nuri: handline://pair?host=1.2.3.4\naddress: 1.2.3.4\nencryption: none (no certificate; plain HTTP)\n" {
		t.Errorf("pair: %q %d", out, code)
	}
	for _, args := range [][]string{{"device", "list"}, {"device", "revoke", "dev_1"}} {
		if _, _, code := r.run(nil, args...); code != 2 {
			t.Errorf("%v: %d", args, code)
		}
	}
	if got := sent(calls()); !reflect.DeepEqual(got, []string{`device.pair {"name":"watch"}`, `connection.addresses {}`}) {
		t.Fatalf("calls %v", got)
	}
}

// A daemon with no listener refuses, and the refusal is shown as it is.
func TestDevicePairShowsTheDaemonsRefusal(t *testing.T) {
	r := founderRig(t)
	r.daemon(map[string]string{"device.pair": "error:the network listener is off"})
	_, errs, code := r.run(nil, "device", "pair")
	if code != 1 || !strings.Contains(errs, "the network listener is off") || strings.Contains(errs, "gateway") {
		t.Fatalf("%q %d", errs, code)
	}
}

// --host names the address; the pairing lists every host a door serves, its kind and encryption, the one named marked.
func TestDevicePairTakesAHostAndListsTheUsableAddresses(t *testing.T) {
	r := founderRig(t)
	calls := r.daemon(map[string]string{
		"device.pair": `{"code":"1","expires_at":"x","uri":"handline://pair?host=192.168.0.5%3A7443","fingerprint":"ab12"}`,
		"connection.addresses": `{"addresses":[{"host":"192.168.0.5","kind":"lan","listening":true,"encryption":"tls"},
			{"host":"100.101.1.2","kind":"tailscale","listening":true,"encryption":"none"},
			{"host":"10.0.0.3","kind":"lan","listening":false,"encryption":"tls"}],"network_addresses":null}`,
	})
	out, _, code := r.run(nil, "device", "pair", "--host", "192.168.0.5")
	want := "address: 192.168.0.5:7443\nfingerprint: ab12\nusable addresses (--host):\n* 192.168.0.5  lan  tls\n  100.101.1.2  tailscale  none\n"
	if code != 0 || !strings.HasSuffix(out, want) {
		t.Fatalf("%q %d", out, code)
	}
	if got := sent(calls()); len(got) == 0 || got[0] != `device.pair {"address":"192.168.0.5"}` {
		t.Fatalf("calls %v", got)
	}
}

// A host no door serves is refused, and the refusal lists the hosts that are.
func TestDevicePairRefusingAHostListsTheUsableOnes(t *testing.T) {
	r := founderRig(t)
	r.daemon(map[string]string{
		"device.pair":          "error[address]:no door serves 8.8.8.8",
		"connection.addresses": `{"addresses":[{"host":"192.168.0.5","kind":"lan","listening":true,"encryption":"tls"}],"network_addresses":null}`,
	})
	_, errs, code := r.run(nil, "device", "pair", "--host", "8.8.8.8")
	if code != 1 || !strings.Contains(errs, "no door serves 8.8.8.8") || !strings.Contains(errs, "  192.168.0.5  lan  tls") {
		t.Fatalf("%q %d", errs, code)
	}
}
