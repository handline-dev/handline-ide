package devices

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/handline/handline/go/protocol/rpc"
)

// kinds are the Device.kind values a client may name at device.redeem.
var kinds = map[string]bool{"ios": true, "android": true, "web": true, "desktop": true}

// maxName is the longest name device.rename keeps, in characters.
const maxName = 64

// maxPushToken bounds a push token: an APNs token is 64 hex characters and an FCM one a few hundred, so this is room, not a format.
const maxPushToken = 4096

// Emit puts an event on the devices topic; actor is nil for none. A nil Emit sends none.
type Emit func(typ string, data map[string]any, actor map[string]any, title string)

// Register adds the device methods to a server. m is nil when the daemon has no network listener: there is
// nothing to pair with, and the methods say so.
func Register(s *rpc.Server, m *Manager, emit Emit) {
	if emit == nil {
		emit = func(string, map[string]any, map[string]any, string) {}
	}
	off := func() error { return rpc.Refused("the network listener is off", "network", "") }
	s.Register("device.pair", rpc.Method{Unkept: true, Handler: func(c *rpc.Conn, p map[string]any) (any, error) {
		if m == nil {
			return nil, off()
		}
		ttl := DefaultTTL
		if v, has := p["ttl_s"]; has {
			n, ok := rpc.AsInt(v)
			if !ok || n < 1 || n > int64(MaxTTL/time.Second) {
				return nil, rpc.BadRequestError("ttl_s is a whole number of seconds from 1 to 600", "ttl_s", rpc.InvalidParams)
			}
			ttl = time.Duration(n) * time.Second
		}
		// name is only a hint for the one who scans the code: the device is listed under the name it redeems with.
		var code, print, uri string
		var expires time.Time
		var web any
		host, isText := p["address"].(string)
		if p["address"] != nil && !isText {
			return nil, rpc.BadRequestError("address is a host from connection.addresses", "address", rpc.InvalidParams)
		}
		door, served, ok := m.Doors(host) // "" is the default door
		if !ok {
			e := rpc.Refused("address is not a host the daemon listens on: one of "+strings.Join(served, ", "), "address", "")
			e.Data["addresses"] = served
			return nil, e
		}
		code, expires, print, uri = m.PairAt(ttl, door)
		if door.Web {
			web = WebLink(door, code)
		}
		return map[string]any{"accepted": true, "code": code, "expires_at": expires.UTC().Format("2006-01-02T15:04:05Z"),
			"fingerprint": print, "uri": uri, "web_link": web}, nil
	}})
	s.Register("device.redeem", rpc.Method{Unkept: true, Handler: func(c *rpc.Conn, p map[string]any) (any, error) {
		code, ok := p["code"].(string)
		if !ok {
			return nil, rpc.BadRequestError("code is a string", "code", rpc.InvalidParams)
		}
		name, ok := p["name"].(string)
		if !ok || name == "" {
			return nil, rpc.BadRequestError("name is the device's name, a string", "name", rpc.InvalidParams)
		}
		if m == nil {
			return nil, rpc.Refused("that pairing code is not valid", "pairing_code", "")
		}
		kind, _ := p["kind"].(string)
		if !kinds[kind] {
			kind = "" // a kind this daemon does not know is kept as none, so a newer client still pairs
		}
		token, d, err := m.Redeem(code, name, kind, c.Remote)
		if err != nil {
			return nil, err
		}
		emit("device.paired", m.Row(d), nil, d.Name+" paired")
		return map[string]any{"accepted": true, "token": token, "device": d.ID}, nil
	}})
	s.Register("device.revoke", rpc.Method{Handler: func(c *rpc.Conn, p map[string]any) (any, error) {
		id, ok := p["device"].(string)
		if !ok {
			return nil, rpc.BadRequestError("device is a string", "device", rpc.InvalidParams)
		}
		if m == nil {
			return nil, rpc.NotFoundError("no such device: " + id)
		}
		found, err := m.Revoke(id)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, rpc.NotFoundError("no such device: " + id)
		}
		emit("device.revoked", map[string]any{"device": id}, c.Caller.ActorInfo(), "device "+id+" revoked")
		return map[string]any{"accepted": true}, nil
	}})
	s.Register("device.rename", rpc.Method{Handler: func(c *rpc.Conn, p map[string]any) (any, error) {
		id, ok := p["device"].(string)
		if !ok {
			return nil, rpc.BadRequestError("device is a string", "device", rpc.InvalidParams)
		}
		raw, _ := p["name"].(string)
		name := strings.TrimSpace(raw)
		if name == "" || utf8.RuneCountInString(name) > maxName {
			return nil, rpc.BadRequestError("name is not empty and at most 64 characters", "name", rpc.InvalidParams)
		}
		if m == nil {
			return nil, rpc.NotFoundError("no such device: " + id)
		}
		d, found, err := m.Rename(id, name)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, rpc.NotFoundError("no such device: " + id)
		}
		emit("device.renamed", m.Row(d), c.Caller.ActorInfo(), "device "+id+" renamed "+name)
		return map[string]any{"accepted": true, "said": "Renamed to " + name + "."}, nil
	}})
	s.Register("device.push_token", rpc.Method{Handler: func(c *rpc.Conn, p map[string]any) (any, error) {
		id, _ := c.Caller.Info["device"].(string)
		if id == "" {
			return nil, rpc.Refused("device.push_token is a paired device's own call: this connection is no device", "device", "")
		}
		platform, _ := p["platform"].(string)
		if platform != "apns" && platform != "fcm" {
			return nil, rpc.BadRequestError("platform is apns or fcm", "platform", rpc.InvalidParams)
		}
		token, _ := p["token"].(string)
		if token == "" || len(token) > maxPushToken {
			return nil, rpc.BadRequestError("token is the push token the OS gave, a string of at most 4096 bytes", "token", rpc.InvalidParams)
		}
		sandbox, ok := p["sandbox"].(bool)
		if _, has := p["sandbox"]; has && !ok {
			return nil, rpc.BadRequestError("sandbox is true or false", "sandbox", rpc.InvalidParams)
		}
		if m == nil {
			return nil, off()
		}
		found, err := m.SetPush(id, Push{Platform: platform, Token: token, Sandbox: sandbox})
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, rpc.NotFoundError("no such device: " + id)
		}
		return map[string]any{"accepted": true}, nil
	}})
	s.Register("devices.list", rpc.Method{Handler: func(c *rpc.Conn, p map[string]any) (any, error) {
		list, platforms := []map[string]any{}, []string{}
		if m != nil {
			var err error
			self, _ := c.Caller.Info["device"].(string)
			if list, err = m.List(self); err != nil {
				return nil, err
			}
			if m.PushPlatforms != nil {
				platforms = m.PushPlatforms
			}
		}
		return map[string]any{"devices": list, "push_platforms": platforms}, nil
	}})
}
