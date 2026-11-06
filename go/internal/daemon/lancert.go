package daemon

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// ensureLANCert is the self-signed certificate the LAN doors present, made in <home>/network the first time one is needed and kept, so
// its fingerprint (which a paired phone pins) stays the same. A browser warns once about it: nothing a LAN has can vouch for it.
func ensureLANCert(home string) (crt, key string, err error) {
	dir := filepath.Join(home, "network")
	crt, key = filepath.Join(dir, "lan.crt"), filepath.Join(dir, "lan.key")
	if _, err := os.Stat(crt); err == nil {
		return crt, key, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "handline daemon"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(20, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, a := range addressesNow() {
		if ip := net.ParseIP(a.Host); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, a.Host)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &k.PublicKey, k)
	if err != nil {
		return "", "", err
	}
	kb, err := x509.MarshalECPrivateKey(k)
	if err != nil {
		return "", "", err
	}
	// The key first: a certificate file is what says the pair is there.
	if err := writeWhole(key, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600); err != nil {
		return "", "", err
	}
	if err := writeWhole(crt, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return "", "", err
	}
	return crt, key, nil
}

// writeWhole writes b to path through a temporary file and a rename, so a crash leaves the old file or none, never half of one.
func writeWhole(path string, b []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
