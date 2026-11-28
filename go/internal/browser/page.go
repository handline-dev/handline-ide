package browser

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Page is the agent's page in its browser, for one command: Asked is each dialog the page opened, and how it was answered.
type Page struct {
	c     *conn
	b     *Browser
	Asked []string
}

// Page connects to the browser's page. A dialog the page opens is answered as the browser says (hl browser dialog), so nothing waits on it.
func (b *Browser) Page() (*Page, error) {
	c, err := b.dialPage()
	if err != nil {
		return nil, err
	}
	p := &Page{c: c, b: b}
	c.on = p.event
	c.within = answerWithin
	if _, err := c.call("Page.enable", nil); err != nil {
		c.close()
		return nil, fmt.Errorf("the page does not answer (%v); a dialog it opened while no command ran may hold it: hl browser tab close, "+
			"or close, then open it again", err)
	}
	c.within = callWithin
	script := dialogScript(b.Accept, b.Reply)
	if _, err := c.call("Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": script}); err != nil {
		c.close()
		return nil, err
	}
	if err := p.Settle(); err != nil {
		c.close()
		return nil, err
	}
	return p, nil
}

// Settle adds to Asked the dialogs the page answered by itself since the last command, or the last Settle.
func (p *Page) Settle() error {
	var earlier []dialogSeen
	if err := p.eval(dialogScript(p.b.Accept, p.b.Reply), &earlier); err != nil {
		return err
	}
	for _, d := range earlier {
		p.asked(d.Type, d.Message, d.Accept)
	}
	return nil
}

func (p *Page) Close() { p.c.close() }

// loadWithin is how long open waits for the page to load.
const loadWithin = 30 * time.Second

// Open goes to url and waits for it to load; it says the page's title and address.
func (p *Page) Open(url string) (string, error) {
	for _, m := range []string{"Page.enable", "Inspector.enable"} {
		if _, err := p.c.call(m, nil); err != nil {
			return "", err
		}
	}
	if _, err := p.c.call("Page.setLifecycleEventsEnabled", map[string]any{"enabled": true}); err != nil {
		return "", err
	}
	raw, err := p.c.call("Page.navigate", map[string]any{"url": url})
	if err != nil {
		return "", err
	}
	var nav struct {
		LoaderID  string `json:"loaderId"`
		ErrorText string `json:"errorText"`
	}
	_ = json.Unmarshal(raw, &nav)
	if nav.ErrorText != "" {
		return "", fmt.Errorf("%s: %s", url, nav.ErrorText)
	}
	if nav.LoaderID != "" { // a new document; a move within the page (#part) loads nothing
		if err := p.c.waitLoad(nav.LoaderID, loadWithin); err != nil {
			return "", fmt.Errorf("%s did not load within %s: %w", url, loadWithin, err)
		}
	}
	var where struct{ Title, URL string }
	if err := p.eval(`({Title: document.title, URL: location.href})`, &where); err != nil {
		return "", err
	}
	return strings.TrimSpace(where.Title + "  " + where.URL), nil
}

// Text is the page's visible text.
func (p *Page) Text() (string, error) {
	var s string
	err := p.eval(`document.body ? document.body.innerText : ""`, &s)
	return s, err
}

// Eval runs script in the page, awaiting a promise, and says its value as JSON (a string as it is).
func (p *Page) Eval(script string) (string, error) {
	var v any
	if err := p.eval(script, &v); err != nil {
		return "", err
	}
	if s, ok := v.(string); ok {
		return s, nil
	}
	b, err := json.MarshalIndent(v, "", "  ")
	return string(b), err
}

func (p *Page) eval(script string, into any) error {
	raw, err := p.c.call("Runtime.evaluate", map[string]any{"expression": script, "returnByValue": true, "awaitPromise": true,
		"userGesture": true})
	if err != nil {
		return err
	}
	var r struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		Exception *struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return err
	}
	if r.Exception != nil {
		if d := r.Exception.Exception.Description; d != "" {
			return errors.New(d)
		}
		return errors.New(r.Exception.Text)
	}
	if len(r.Result.Value) == 0 {
		return nil // undefined
	}
	return json.Unmarshal(r.Result.Value, into)
}

// ref is a node of the tree Tree prints, e<its backend node id>.
var ref = regexp.MustCompile(`^e(\d+)$`)

// object is the remote object of target: a ref from Tree, or a CSS selector. "" when nothing matches.
func (p *Page) object(target string) (string, error) {
	if m := ref.FindStringSubmatch(target); m != nil {
		id, _ := strconv.Atoi(m[1])
		raw, err := p.c.call("DOM.resolveNode", map[string]any{"backendNodeId": id})
		if err != nil {
			return "", fmt.Errorf("%s is not on the page any more; read the tree again (%v)", target, err)
		}
		var r struct {
			Object struct {
				ID string `json:"objectId"`
			} `json:"object"`
		}
		_ = json.Unmarshal(raw, &r)
		return r.Object.ID, nil
	}
	sel, _ := json.Marshal(target)
	raw, err := p.c.call("Runtime.evaluate", map[string]any{"expression": "document.querySelector(" + string(sel) + ")"})
	if err != nil {
		return "", err
	}
	var r struct {
		Result struct {
			ID      string `json:"objectId"`
			Subtype string `json:"subtype"`
		} `json:"result"`
		Exception *struct{} `json:"exceptionDetails"`
	}
	_ = json.Unmarshal(raw, &r)
	if r.Exception != nil {
		return "", fmt.Errorf("%q is not a CSS selector nor a ref (e12)", target)
	}
	return r.Result.ID, nil // a null result has none
}

// Box is a target's place on the page, in the page's CSS pixels: its top left corner and its size.
type Box struct{ X, Y, W, H float64 }

func (b Box) centre() (x, y float64) { return b.X + b.W/2, b.Y + b.H/2 }

// Box scrolls target into view and says its box.
func (p *Page) Box(target string) (Box, error) { return p.box(target, true) }

// found is target's remote object; an error when nothing matches.
func (p *Page) found(target string) (string, error) {
	id, err := p.object(target)
	if err == nil && id == "" {
		err = fmt.Errorf("nothing on the page matches %s", target)
	}
	return id, err
}

func (p *Page) box(target string, scroll bool) (Box, error) {
	id, err := p.found(target)
	if err != nil {
		return Box{}, err
	}
	raw, err := p.c.call("Runtime.callFunctionOn", map[string]any{"objectId": id, "returnByValue": true, "arguments": []any{map[string]any{
		"value": scroll}}, "functionDeclaration": `function(scroll) {
		const el = this.nodeType === 1 ? this : this.parentElement;
		if (scroll) el.scrollIntoView({block: "center", inline: "center"});
		const r = el.getBoundingClientRect();
		return [r.x, r.y, r.width, r.height];
	}`})
	if err != nil {
		return Box{}, err
	}
	var r struct {
		Result struct {
			Value []float64 `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &r); err != nil || len(r.Result.Value) != 4 {
		return Box{}, fmt.Errorf("%s has no box", target)
	}
	v := r.Result.Value
	if v[2] == 0 && v[3] == 0 {
		return Box{}, fmt.Errorf("%s is not shown", target)
	}
	return Box{v[0], v[1], v[2], v[3]}, nil
}

// Click clicks the middle of target.
func (p *Page) Click(target string) error {
	b, err := p.Box(target)
	if err != nil {
		return err
	}
	return p.ClickAt(b)
}

// ClickAt clicks the middle of box.
func (p *Page) ClickAt(b Box) error {
	x, y := b.centre()
	for _, kind := range []string{"mouseMoved", "mousePressed", "mouseReleased"} {
		ev := map[string]any{"type": kind, "x": x, "y": y}
		if kind != "mouseMoved" {
			ev["button"], ev["clickCount"] = "left", 1
		}
		if _, err := p.c.call("Input.dispatchMouseEvent", ev); err != nil {
			return err
		}
	}
	return nil
}

// Type clicks target and types text into it, then presses Enter when enter.
func (p *Page) Type(target, text string, enter bool) error {
	b, err := p.Box(target)
	if err != nil {
		return err
	}
	return p.TypeAt(b, text, enter)
}

// TypeAt clicks the middle of box and types text there, then presses Enter when enter.
func (p *Page) TypeAt(b Box, text string, enter bool) error {
	if err := p.ClickAt(b); err != nil {
		return err
	}
	if _, err := p.c.call("Input.insertText", map[string]any{"text": text}); err != nil {
		return err
	}
	if !enter {
		return nil
	}
	return p.Press("Enter")
}

// Wait waits until target is on the page, or, when text is set, until the page's text holds it: the page watches its own changes.
func (p *Page) Wait(target, text string, within time.Duration) error {
	what, test := target, ""
	switch {
	case text != "":
		q, _ := json.Marshal(text)
		what, test = strconv.Quote(text), `(document.body ? document.body.innerText : "").includes(`+string(q)+`)`
	case ref.MatchString(target): // a ref is a node already read; it is there or gone
		id, err := p.object(target)
		if err != nil || id == "" {
			return fmt.Errorf("%s is not on the page any more; read the tree again", target)
		}
		return nil
	default:
		q, _ := json.Marshal(target)
		test = `!!document.querySelector(` + string(q) + `)`
	}
	script := fmt.Sprintf(`new Promise(done => {
		const ok = () => %s;
		if (ok()) return done(true);
		const o = new MutationObserver(() => { if (ok()) { o.disconnect(); done(true); } });
		o.observe(document, {subtree: true, childList: true, characterData: true, attributes: true});
		setTimeout(() => { o.disconnect(); done(ok()); }, %d);
	})`, test, within.Milliseconds())
	p.c.within = within + callWithin
	var found bool
	if err := p.eval(script, &found); err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%s did not appear within %s", what, within)
	}
	return nil
}

// Shot saves a PNG of the page to file: what the window shows, or the whole page when full.
func (p *Page) Shot(file string, full bool) error {
	params := map[string]any{"format": "png"}
	if full {
		raw, err := p.c.call("Page.getLayoutMetrics", nil)
		if err != nil {
			return err
		}
		var m struct {
			Size struct{ Width, Height float64 } `json:"cssContentSize"`
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			return err
		}
		params["captureBeyondViewport"] = true
		params["clip"] = map[string]any{"x": 0, "y": 0, "width": m.Size.Width, "height": m.Size.Height, "scale": 1}
	}
	raw, err := p.c.call("Page.captureScreenshot", params)
	if err != nil {
		return err
	}
	var r struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return err
	}
	png, err := base64.StdEncoding.DecodeString(r.Data)
	if err != nil {
		return err
	}
	return os.WriteFile(file, png, 0o644)
}
