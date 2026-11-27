package daemon

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// tsCert is Tailscale's own certificate for this machine's MagicDNS name, which a browser trusts as it trusts any site: `tailscale cert`
// writes it into <home>/network, and it is fetched again a month before it expires (certSource serves the new files without a restart).
type tsCert struct {
	dir   string
	ready func()                            // told when a fetch has written a certificate, so the doors are made again
	get   func(name, crt, key string) error // fetches the certificate

	mu       sync.Mutex
	name     string // the name the files are for
	src      *certSource
	fetching bool
	tried    time.Time // the last fetch that left no certificate good for renewBefore: the next waits an hour
}

// renewBefore is how long before its expiry the certificate is fetched again; Tailscale's are good for 90 days.
const renewBefore = 30 * 24 * time.Hour

// tailscaleCert runs `tailscale cert` for name and writes what it prints into crt and key: the App Store build of Tailscale is
// sandboxed and cannot write a file outside its own folder.
func tailscaleCert(name, crt, key string) error {
	var last error
	for _, cli := range tailscaleCLIs {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		out, err := exec.CommandContext(ctx, cli, "cert", "--cert-file", "-", "--key-file", "-",
			"--min-validity", renewBefore.String(), name).Output()
		cancel()
		if err != nil {
			last = fmt.Errorf("%s cert: %w", cli, err)
			continue
		}
		var certs, keys []byte
		for rest := out; ; {
			var b *pem.Block
			if b, rest = pem.Decode(rest); b == nil {
				break
			}
			if b.Type == "CERTIFICATE" {
				certs = append(certs, pem.EncodeToMemory(b)...)
			} else {
				keys = append(keys, pem.EncodeToMemory(b)...)
			}
		}
		if len(certs) == 0 || len(keys) == 0 {
			return fmt.Errorf("%s cert printed no certificate and key", cli)
		}
		// The key first: a certificate file is what says the pair is there.
		if err := writeWhole(key, keys, 0o600); err != nil {
			return err
		}
		return writeWhole(crt, certs, 0o644)
	}
	return last
}

// source is the certificate for name when one is there and not expired, else nil; either way a fetch starts when none is there for
// name or it expires within renewBefore.
func (t *tsCert) source(name string, log func(string, ...any)) *certSource {
	t.mu.Lock()
	defer t.mu.Unlock()
	crt, key := filepath.Join(t.dir, "tailscale.crt"), filepath.Join(t.dir, "tailscale.key")
	if t.name != name {
		t.name, t.src, t.tried = name, nil, time.Time{}
	}
	until, forName := certUntil(crt, name)
	switch valid := forName && time.Now().Before(until); {
	case !valid:
		t.src = nil
	case t.src == nil:
		t.src = &certSource{certFile: crt, keyFile: key}
	}
	if (!forName || time.Until(until) < renewBefore) && !t.fetching && time.Since(t.tried) >= time.Hour {
		t.fetching = true
		go t.fetch(name, crt, key, log)
	}
	return t.src
}

func (t *tsCert) fetch(name, crt, key string, log func(string, ...any)) {
	err := os.MkdirAll(t.dir, 0o700)
	if err == nil {
		err = t.get(name, crt, key)
	}
	t.mu.Lock()
	t.fetching = false
	if until, ok := certUntil(crt, name); err != nil || !ok || time.Until(until) < renewBefore {
		t.tried = time.Now() // failed, or Tailscale gave the same one back: ask again in an hour, not on every round
	}
	t.mu.Unlock()
	if err != nil {
		log("tailscale certificate for %s: %v", name, err)
		return
	}
	t.ready()
}

// certUntil is when the certificate in file expires, and whether it is one for name.
func certUntil(file, name string) (time.Time, bool) {
	b, err := os.ReadFile(file)
	if err != nil {
		return time.Time{}, false
	}
	blk, _ := pem.Decode(b)
	if blk == nil {
		return time.Time{}, false
	}
	c, err := x509.ParseCertificate(blk.Bytes)
	if err != nil || c.VerifyHostname(name) != nil {
		return time.Time{}, false
	}
	return c.NotAfter, true
}
