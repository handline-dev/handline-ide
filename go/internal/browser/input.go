package browser

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func (p *Page) mouse(kind string, x, y float64, button string) error {
	ev := map[string]any{"type": kind, "x": x, "y": y}
	if button != "" {
		ev["button"], ev["clickCount"] = button, 1
		if kind == "mouseMoved" {
			ev["buttons"] = 1
		}
	}
	_, err := p.c.call("Input.dispatchMouseEvent", ev)
	return err
}

// HoverAt moves the mouse to the middle of box.
func (p *Page) HoverAt(b Box) error {
	x, y := b.centre()
	return p.mouse("mouseMoved", x, y, "")
}

// DragTo is target's box once to's has been scrolled into view, and to's: a drag needs both on the screen.
func (p *Page) DragTo(target, to string) (from, onto Box, err error) {
	if onto, err = p.Box(to); err != nil {
		return
	}
	from, err = p.box(target, false)
	return
}

// dragSteps is how many moves a drag makes on its way, so a page that follows the pointer sees it travel.
const dragSteps = 8

// DragAt presses the mouse in from, moves it to onto and lets go there: a page that follows the pointer sees a drag, and an HTML drag the
// browser starts is caught and dropped on onto.
func (p *Page) DragAt(from, onto Box) error {
	var data json.RawMessage
	prev := p.c.on
	p.c.on = func(m message) {
		if m.Method == "Input.dragIntercepted" {
			var e struct {
				Data json.RawMessage `json:"data"`
			}
			_ = json.Unmarshal(m.Params, &e)
			data = e.Data
		}
		prev(m)
	}
	defer func() { p.c.on = prev }()
	if _, err := p.c.call("Input.setInterceptDrags", map[string]any{"enabled": true}); err != nil {
		return err
	}
	defer p.c.call("Input.setInterceptDrags", map[string]any{"enabled": false})
	x0, y0 := from.centre()
	x1, y1 := onto.centre()
	if err := p.mouse("mouseMoved", x0, y0, ""); err != nil {
		return err
	}
	if err := p.mouse("mousePressed", x0, y0, "left"); err != nil {
		return err
	}
	for i := 1; i <= dragSteps; i++ {
		f := float64(i) / dragSteps
		if err := p.mouse("mouseMoved", x0+(x1-x0)*f, y0+(y1-y0)*f, "left"); err != nil {
			return err
		}
		if data != nil {
			break
		}
	}
	if data != nil {
		for _, kind := range []string{"dragEnter", "dragOver", "drop"} {
			if _, err := p.c.call("Input.dispatchDragEvent", map[string]any{"type": kind, "x": x1, "y": y1, "data": data}); err != nil {
				return err
			}
		}
	}
	return p.mouse("mouseReleased", x1, y1, "left")
}

// Viewport is the window's visible part of the page.
func (p *Page) Viewport() (Box, error) {
	var v []float64
	if err := p.eval(`[innerWidth, innerHeight]`, &v); err != nil {
		return Box{}, err
	}
	if len(v) != 2 {
		return Box{}, errors.New("the page has no window")
	}
	return Box{0, 0, v[0], v[1]}, nil
}

// ScrollAt turns the wheel over the middle of at by dx, dy pixels and says where the page, or the element scrolled, is then.
func (p *Page) ScrollAt(at Box, dx, dy float64) (string, error) {
	x, y := at.centre()
	if _, err := p.c.call("Input.dispatchMouseEvent", map[string]any{"type": "mouseWheel", "x": x, "y": y, "deltaX": dx,
		"deltaY": dy}); err != nil {
		return "", err
	}
	var where []float64
	err := p.eval(fmt.Sprintf(`new Promise(done => requestAnimationFrame(() => requestAnimationFrame(() => {
		let el = document.elementFromPoint(%g, %g);
		while (el && el !== document.scrollingElement && !(el.scrollHeight > el.clientHeight && /auto|scroll/.test(getComputedStyle(el).overflowY))
			&& !(el.scrollWidth > el.clientWidth && /auto|scroll/.test(getComputedStyle(el).overflowX))) el = el.parentElement;
		el ||= document.scrollingElement;
		done([el.scrollLeft, el.scrollTop, el.scrollWidth - el.clientWidth, el.scrollHeight - el.clientHeight]);
	})))`, x, y), &where)
	if err != nil {
		return "", err
	}
	if len(where) != 4 {
		return "", errors.New("the page did not say where it is")
	}
	return fmt.Sprintf("at %g,%g of %g,%g", where[0], where[1], where[2], where[3]), nil
}

// Select picks the option of target, a select, whose value or else whose label is choice, and says its label.
func (p *Page) Select(target, choice string) (string, error) {
	id, err := p.found(target)
	if err != nil {
		return "", err
	}
	var label string
	err = p.callOn(id, `function(choice) {
		if (!(this instanceof HTMLSelectElement)) throw new Error("not a select");
		const o = [...this.options].find(o => o.value === choice) || [...this.options].find(o => o.label.trim() === choice.trim());
		if (!o) throw new Error("no option has the value or label " + JSON.stringify(choice) + "; it has " +
			[...this.options].map(o => JSON.stringify(o.label.trim())).join(", "));
		this.value = o.value;
		this.dispatchEvent(new Event("input", {bubbles: true}));
		this.dispatchEvent(new Event("change", {bubbles: true}));
		return o.label.trim();
	}`, &label, choice)
	if err != nil {
		return "", fmt.Errorf("%s: %w", target, err)
	}
	return label, nil
}

// Upload puts files, whole paths, in target, a file input.
func (p *Page) Upload(target string, files []string) error {
	id, err := p.found(target)
	if err != nil {
		return err
	}
	var ok bool
	if err := p.callOn(id, `function() { return this instanceof HTMLInputElement && this.type === "file"; }`, &ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s is not a file input", target)
	}
	_, err = p.c.call("DOM.setFileInputFiles", map[string]any{"objectId": id, "files": files})
	return err
}

// callOn runs fn with the object as this and args, and reads its value into into; a throw is the error.
func (p *Page) callOn(id, fn string, into any, args ...any) error {
	as := []any{}
	for _, a := range args {
		as = append(as, map[string]any{"value": a})
	}
	raw, err := p.c.call("Runtime.callFunctionOn", map[string]any{"objectId": id, "returnByValue": true, "functionDeclaration": fn,
		"arguments": as})
	if err != nil {
		return err
	}
	var r struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		Exception *struct {
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return err
	}
	if r.Exception != nil {
		d, _, _ := strings.Cut(r.Exception.Exception.Description, "\n")
		return errors.New(strings.TrimPrefix(d, "Error: "))
	}
	if len(r.Result.Value) == 0 {
		return nil
	}
	return json.Unmarshal(r.Result.Value, into)
}
