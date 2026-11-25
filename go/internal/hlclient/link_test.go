package hlclient

import (
	"strings"
	"testing"
)

// hl device join takes the browser link device.pair gives and a handline:// one, and trusts a link as the apps do: a certificate
// fingerprint, a Tailscale name or loopback; any other is refused with the apps' sentence.
func TestAPairingLinkIsTrustedAsTheAppsTrustOne(t *testing.T) {
	fp := strings.Repeat("ab", 32)
	for _, c := range []struct{ link, addr, code, fp string }{
		{"https://mac.tail1.ts.net:7443/#pair=AB-CD", "mac.tail1.ts.net:7443", "AB-CD", ""},
		{"http://mac.tail1.ts.net:7443/#pair=AB-CD", "mac.tail1.ts.net:7443", "AB-CD", ""},
		{"https://192.168.0.5:7443/#pair=AB-CD&fp=" + strings.ToUpper(fp), "192.168.0.5:7443", "AB-CD", fp},
		{"http://127.0.0.1:7443/#pair=X", "127.0.0.1:7443", "X", ""},
		{" handline://pair?host=192.168.0.5%3A7443&code=X&fp=" + fp + "\n", "192.168.0.5:7443", "X", fp},
		{"handline://pair?host=box.tail1.ts.net.%3A7443&code=X", "box.tail1.ts.net.:7443", "X", ""},
	} {
		addr, code, gotFP, err := ParseLink(c.link)
		if err != nil || addr != c.addr || code != c.code || gotFP != c.fp {
			t.Errorf("%s: %s %s %s %v", c.link, addr, code, gotFP, err)
		}
	}
	for link, says := range map[string]string{
		"https://192.168.0.5:7443/#pair=X":                   "is not a Tailscale name (*.ts.net), so it cannot be trusted",
		"http://100.64.0.1:7443/#pair=X":                     "is not a Tailscale name (*.ts.net), so it cannot be trusted",
		"handline://pair?host=192.168.0.5%3A7443&code=X":     "is not a Tailscale name (*.ts.net), so it cannot be trusted",
		"https://mac.tail1.ts.net:7443/":                     "names no host or no code",
		"https://192.168.0.5:7443/#pair=X&fp=12":             "not a sha256 fingerprint",
		"ftp://mac.tail1.ts.net/#pair=X":                     "not a pairing link",
		"https://mac.tail1.ts.net.evil.example:7443/#pair=X": "cannot be trusted",
	} {
		if _, _, _, err := ParseLink(link); err == nil || !strings.Contains(err.Error(), says) {
			t.Errorf("%s: %v", link, err)
		}
	}
}
