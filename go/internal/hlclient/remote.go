package hlclient

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/handline/handline/go/kit/joined"
	"github.com/handline/handline/go/kit/protected"
)

// Join redeems a pairing link's code at the master it names, as this machine: name, kind (mac or server) and node id. The connection
// is pinned to the link's fingerprint. What it answers is kept in home's protected store.
func Join(home, link, name, kind, node string) (joined.Master, error) {
	if protected.InAgentSession() { // checked before the code is spent: its token could not be kept
		return joined.Master{}, protected.ErrNotTheFounder
	}
	addr, code, fp, err := ParseLink(link)
	if err != nil {
		return joined.Master{}, err
	}
	ws, err := DialMaster(addr, fp)
	if err != nil {
		return joined.Master{}, err
	}
	defer ws.Close()
	req := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "device.redeem", "params": map[string]any{
		"code": code, "name": name, "kind": kind, "node": node, "request_id": "join-" + node + "-" + fmt.Sprint(time.Now().UnixNano())}}
	ws.SetWriteDeadline(time.Now().Add(Quick))
	if err := ws.WriteJSON(req); err != nil {
		return joined.Master{}, err
	}
	ws.SetReadDeadline(time.Now().Add(Quick))
	var reply struct {
		Result struct {
			Token  string `json:"token"`
			Device string `json:"device"`
		} `json:"result"`
		Error *struct {
			Message string         `json:"message"`
			Data    map[string]any `json:"data"`
		} `json:"error"`
	}
	// From here the master may have made the device: a token not kept is lost, and the machine joins again only once it is revoked.
	lost := func(err error) (joined.Master, error) {
		return joined.Master{}, fmt.Errorf("%w: if %s shows in the master's Devices, revoke it there and join again", err, name)
	}
	if err := ws.ReadJSON(&reply); err != nil {
		return lost(err)
	}
	if reply.Error != nil {
		reason, _ := reply.Error.Data["reason"].(string)
		if reason == "" {
			reason = reply.Error.Message
		}
		return joined.Master{}, errors.New(reason)
	}
	if reply.Result.Token == "" || reply.Result.Device == "" {
		return lost(errors.New("the master answered no token"))
	}
	m := joined.Master{Addr: addr, Fingerprint: fp, Token: reply.Result.Token, Device: reply.Result.Device, At: time.Now().Unix()}
	if err := joined.Keep(home, m); err != nil {
		return lost(err)
	}
	return m, nil
}

// ParseLink reads a pairing link: the browser link device.pair gives (http(s)://host:port/#pair=code, with &fp= on a self-made
// certificate) or a handline://pair?host=&code=&fp= one. It is trusted as the apps trust one: with a certificate fingerprint to pin, or
// for a Tailscale name (*.ts.net), which resolves only on the tailnet, or for loopback.
func ParseLink(link string) (addr, code, fp string, err error) {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil {
		return "", "", "", errors.New("not a pairing link")
	}
	switch {
	case u.Scheme == "handline" && u.Host == "pair":
		q := u.Query()
		addr, code, fp = q.Get("host"), q.Get("code"), q.Get("fp")
	case u.Scheme == "http" || u.Scheme == "https":
		q, _ := url.ParseQuery(u.Fragment)
		addr, code, fp = u.Host, q.Get("pair"), q.Get("fp")
	default:
		return "", "", "", errors.New("not a pairing link: it is the link Connect a device gives, https://… or http://…")
	}
	fp = strings.ToLower(fp)
	if addr == "" || code == "" {
		return "", "", "", errors.New("the pairing link names no host or no code")
	}
	if b, err := hex.DecodeString(fp); err != nil || fp != "" && len(b) != sha256.Size {
		return "", "", "", errors.New("the pairing link's fp is not a sha256 fingerprint")
	}
	host := strings.ToLower(strings.TrimSuffix(hostOnly(addr), "."))
	if ip := net.ParseIP(host); fp == "" && !strings.HasSuffix(host, ".ts.net") && (ip == nil || !ip.IsLoopback()) && host != "localhost" {
		return "", "", "", fmt.Errorf("this pairing link has no certificate fingerprint, and %s is not a Tailscale name (*.ts.net), so it cannot be trusted", addr)
	}
	return addr, code, fp, nil
}

func hostOnly(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil {
		return h
	}
	return addr
}

// DialMaster opens the master's /rpc at addr over a websocket, pinned to the certificate fingerprint fp ("" for a plain door).
func DialMaster(addr, fp string) (*websocket.Conn, error) {
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	scheme := "ws"
	if fp != "" {
		scheme = "wss"
		dialer.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true, // the master's certificate is its own: the fingerprint the link names is what is trusted
			VerifyConnection: func(cs tls.ConnectionState) error {
				if len(cs.PeerCertificates) == 0 {
					return errors.New("the master showed no certificate")
				}
				sum := sha256.Sum256(cs.PeerCertificates[0].Raw)
				if hex.EncodeToString(sum[:]) != fp {
					return errors.New("the master's certificate is not the one the pairing link names")
				}
				return nil
			},
		}
	}
	ws, _, err := dialer.Dial(scheme+"://"+addr+"/rpc", nil)
	if err != nil {
		return nil, fmt.Errorf("cannot reach the master at %s: %w", addr, err)
	}
	return ws, nil
}
