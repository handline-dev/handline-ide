package devices

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/protocol/rpc"
)

// kinds are the Device.kind values a client may name at device.redeem.
var kinds = map[string]bool{"ios": true, "android": true, "web": true, "desktop": true, "mac": true, "server": true}

// nodeID is what a node id may be: the 16 hex digits NodeID makes, with room.
var nodeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// lends are the kinds that lend their machine: they redeem with its node id and have Run, Pause and Off.
var lends = map[string]bool{"mac": true, "server": true}

// personOf is the person a caller is: a person's own key, or a device's person; "" for an agent or no one.
func personOf(c *rpc.Conn) string {
	switch c.Caller.Kind {
	case "person":
		return c.Caller.Key
	case "device":
		return c.Caller.Person
	}
	return ""
}

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
		person := personOf(c)
		if v, has := p["person"]; has { // the owner makes the link of a person they invited
			k, _ := v.(string)
			if actor.KindOf(k) != actor.Person {
				return nil, rpc.BadRequestError("person is a person's key (p<n>)", "person", rpc.InvalidParams)
			}
			if k != person && c.Caller.Role != "owner" {
				return nil, rpc.Refused("a member pairs only their own devices", "person", "")
			}
			person = k
		}
		code, expires, print, uri = m.PairAt(ttl, door, person)
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
		node, _ := p["node"].(string)
		if lends[kind] != (node != "") || !nodeID.MatchString(node) && node != "" {
			return nil, rpc.BadRequestError("node is the machine's node id, given by a mac or a server and by nothing else", "node", rpc.InvalidParams)
		}
		token, d, err := m.Redeem(code, name, kind, node, c.Remote)
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
		if c.Caller.Role != "owner" && !m.Owns(id, personOf(c)) {
			return nil, rpc.Refused("a member revokes only their own devices", "person", "")
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
	s.Register("device.set", rpc.Method{Handler: func(c *rpc.Conn, p map[string]any) (any, error) {
		id, ok := p["device"].(string)
		if !ok {
			return nil, rpc.BadRequestError("device is a string", "device", rpc.InvalidParams)
		}
		var n NodeSet
		if v, has := p["lending"]; has {
			if n.Lending, _ = v.(string); !Lendings[n.Lending] {
				return nil, rpc.BadRequestError("lending is run, pause or off", "lending", rpc.InvalidParams)
			}
		}
		if v, has := p["memory_max_gb"]; has {
			var gb int64 // null: back to agents_memory_max_gb
			if v != nil {
				num, _ := v.(json.Number)
				var err error
				if gb, err = num.Int64(); err != nil || gb < 1 || gb > MemoryMaxGB {
					return nil, rpc.BadRequestError(fmt.Sprintf("memory_max_gb is a whole number of GB from 1 to %d, or null for the master's agents_memory_max_gb", MemoryMaxGB), "memory_max_gb", rpc.InvalidParams)
				}
			}
			n.MemoryMaxGB = &gb
		}
		if n.Lending == "" && n.MemoryMaxGB == nil {
			return nil, rpc.BadRequestError("device.set takes lending, memory_max_gb or both", "device", rpc.InvalidParams)
		}
		if m == nil {
			return nil, rpc.NotFoundError("no such device: " + id)
		}
		d, err := m.SetNode(id, personOf(c), n)
		if err != nil {
			return nil, err
		}
		var what []string
		if n.Lending != "" {
			what = append(what, "set to "+n.Lending)
		}
		if n.MemoryMaxGB != nil {
			what = append(what, "memory cap changed")
		}
		emit("device.changed", m.Row(d), c.Caller.ActorInfo(), d.Name+" "+strings.Join(what, ", "))
		return map[string]any{"accepted": true}, nil
	}})
	s.Register("device.update", rpc.Method{Handler: func(c *rpc.Conn, p map[string]any) (any, error) {
		id, ok := p["device"].(string)
		if !ok {
			return nil, rpc.BadRequestError("device is a string", "device", rpc.InvalidParams)
		}
		if m == nil || m.Update == nil {
			return nil, rpc.Refused("this daemon hands no build to its machines", "build", "")
		}
		if !m.Owns(id, personOf(c)) {
			return nil, rpc.Refused("only the person whose device it is updates it", "person", "")
		}
		if err := m.Update(id); err != nil {
			return nil, err
		}
		return map[string]any{"accepted": true}, nil
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
			mine := "" // the owner sees everyone's, a member their own
			if c.Caller.Role != "owner" {
				mine = personOf(c)
			}
			if list, err = m.List(self, mine); err != nil {
				return nil, err
			}
			if m.PushPlatforms != nil {
				platforms = m.PushPlatforms
			}
		}
		return map[string]any{"devices": list, "push_platforms": platforms}, nil
	}})
}
