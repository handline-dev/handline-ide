package browser

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/handline/handline/go/internal/screenpipe"
)

// conn is one DevTools connection: calls are answered in turn, and the loads seen meanwhile are kept for waitLoad.
type conn struct {
	ws     *websocket.Conn
	next   int
	within time.Duration // how long one call may take
	loaded map[string]bool
	on     func(message) // sees each event read
}

type message struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// callWithin is how long one DevTools call may take unless the connection says otherwise.
const callWithin = 30 * time.Second

func dial(url string) (*conn, error) {
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(256 << 20) // a full-page screenshot comes back in one message
	return &conn{ws: ws, within: callWithin, loaded: map[string]bool{}}, nil
}

func (c *conn) close() { c.ws.Close() }

// call sends method and waits for its answer.
func (c *conn) call(method string, params any) (json.RawMessage, error) {
	c.next++
	id := c.next
	if params == nil {
		params = struct{}{}
	}
	deadline := time.Now().Add(c.within)
	_ = c.ws.SetWriteDeadline(deadline)
	if err := c.ws.WriteJSON(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	for {
		m, err := c.read(deadline)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", method, err)
		}
		if m.ID != id {
			continue
		}
		if m.Error != nil {
			return nil, fmt.Errorf("%s: %s", method, m.Error.Message)
		}
		return m.Result, nil
	}
}

// waitLoad waits for the load of the document loader started, one already seen counting.
func (c *conn) waitLoad(loader string, within time.Duration) error {
	deadline := time.Now().Add(within)
	for !c.loaded[loader] {
		if _, err := c.read(deadline); err != nil {
			return err
		}
	}
	return nil
}

// read is the next message; it notes a document's load, and a page that crashed or went away is an error.
func (c *conn) read(deadline time.Time) (message, error) {
	_ = c.ws.SetReadDeadline(deadline)
	var m message
	if err := c.ws.ReadJSON(&m); err != nil {
		return m, err
	}
	if m.Method != "" && c.on != nil {
		c.on(m)
	}
	switch m.Method {
	case "Page.lifecycleEvent":
		var e struct{ Name, LoaderID string }
		if json.Unmarshal(m.Params, &e) == nil && e.Name == "load" {
			c.loaded[e.LoaderID] = true
		}
	case "Inspector.detached", "Inspector.targetCrashed":
		return m, errors.New("the page closed or crashed (" + m.Method + ")")
	}
	return m, nil
}

type target struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
	URL   string `json:"url"`
	WS    string `json:"webSocketDebuggerUrl"`
}

func (b *Browser) endpoint(path, method string) (*http.Response, error) {
	req, err := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d%s", b.Port, path), nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err == nil && resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: %s", path, resp.Status)
	}
	return resp, err
}

// dialBrowser is a connection to the browser itself.
func (b *Browser) dialBrowser() (*conn, error) {
	resp, err := b.endpoint("/json/version", http.MethodGet)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var v struct {
		WS string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}
	return dial(v.WS)
}

// dialPage is a connection to the agent's page: the one it worked in, else the first, else a new one. A page a click opened beside it
// does not take its place.
func (b *Browser) dialPage() (*conn, error) { return b.dialPageOr(true) }

// dialPageOr is dialPage; with open false, a browser with no page is not open, and none is opened for it.
func (b *Browser) dialPageOr(open bool) (*conn, error) {
	resp, err := b.endpoint("/json/list", http.MethodGet)
	if err != nil {
		return nil, err
	}
	var list []target
	err = json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	var pick *target
	for i, t := range list {
		if t.Type != "page" || strings.HasPrefix(t.URL, "devtools://") || t.WS == "" {
			continue
		}
		if t.ID == b.PageID {
			pick = &list[i]
			break
		}
		if pick == nil {
			pick = &list[i]
		}
	}
	if pick == nil && !open {
		return nil, screenpipe.ErrNotOpen
	}
	if pick == nil {
		resp, err := b.endpoint("/json/new?about:blank", http.MethodPut)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		pick = &target{}
		if err := json.NewDecoder(resp.Body).Decode(pick); err != nil {
			return nil, err
		}
		if pick.WS == "" {
			return nil, errors.New("the browser opened no page")
		}
	}
	if pick.ID != b.PageID {
		id := pick.ID
		_ = b.update(func(b *Browser) { b.PageID = id })
	}
	return dial(pick.WS)
}
