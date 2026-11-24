package sim

import (
	"fmt"
	"io"
	"strings"
)

// Node is one element of what is on a screen, the same for iOS (its accessibility tree) and Android (its view hierarchy): frames are in
// the points taps are given in.
type Node struct {
	Type        string  `json:"type"`
	Label       string  `json:"label,omitempty"`
	ID          string  `json:"id,omitempty"`
	Value       string  `json:"value,omitempty"`
	Placeholder string  `json:"placeholder,omitempty"`
	Frame       Frame   `json:"frame"`
	Enabled     *bool   `json:"enabled,omitempty"` // false when not; absent when enabled
	Selected    bool    `json:"selected,omitempty"`
	Children    []*Node `json:"children,omitempty"`
}

// Frame is x, y, width and height.
type Frame [4]float64

// Center is the point a tap on the element goes to.
func (f Frame) Center() (float64, float64) { return f[0] + f[2]/2, f[1] + f[3]/2 }

func (n *Node) disabled() bool { return n.Enabled != nil && !*n.Enabled }

// interactive are the types listed even with no text of their own: an agent may need to see a button that has no label to know it is
// there.
var interactive = map[string]bool{"button": true, "textField": true, "secureTextField": true, "switch": true, "slider": true, "searchField": true,
	"textView": true, "tab": true, "cell": true, "stepper": true, "link": true, "key": true, "EditText": true, "Button": true, "CheckBox": true,
	"Switch": true, "ImageButton": true}

func (n *Node) shown() bool {
	return n.Label != "" || n.ID != "" || n.Value != "" || n.Placeholder != "" || interactive[n.Type]
}

// Render writes the elements that say something, one per line, indented by how many shown elements they sit in.
func Render(w io.Writer, root *Node) {
	var walk func(n *Node, depth int)
	walk = func(n *Node, depth int) {
		if n.shown() {
			fmt.Fprintf(w, "%s%s\n", strings.Repeat("  ", depth), n.line())
			depth++
		}
		for _, c := range n.Children {
			walk(c, depth)
		}
	}
	walk(root, 0)
}

func (n *Node) line() string {
	s := n.Type
	if n.Label != "" {
		s += fmt.Sprintf(" %q", n.Label)
	}
	if n.ID != "" {
		s += " #" + n.ID
	}
	if n.Value != "" && n.Value != n.Label {
		s += fmt.Sprintf(" =%q", n.Value)
	}
	if n.Placeholder != "" {
		s += fmt.Sprintf(" placeholder=%q", n.Placeholder)
	}
	if n.disabled() {
		s += " disabled"
	}
	if n.Selected {
		s += " selected"
	}
	return fmt.Sprintf("%s @%.0f,%.0f %.0fx%.0f", s, n.Frame[0], n.Frame[1], n.Frame[2], n.Frame[3])
}

// Selector is what to find: Label or ID exactly, or Text anywhere in the label, id or value without regard to case.
type Selector struct{ Label, ID, Text string }

func (s Selector) empty() bool { return s.Label == "" && s.ID == "" && s.Text == "" }

func (s Selector) matches(n *Node) bool {
	switch {
	case s.Label != "":
		return n.Label == s.Label
	case s.ID != "":
		return n.ID == s.ID
	}
	t := strings.ToLower(s.Text)
	return strings.Contains(strings.ToLower(n.Label), t) || strings.Contains(strings.ToLower(n.ID), t) || strings.Contains(strings.ToLower(n.Value), t)
}

// Find is the first enabled element, in reading order, that matches sel and has room to tap.
func Find(root *Node, sel Selector) (*Node, bool) {
	if sel.empty() {
		return nil, false
	}
	var found *Node
	var walk func(n *Node)
	walk = func(n *Node) {
		if found != nil {
			return
		}
		if sel.matches(n) && !n.disabled() && n.Frame[2] > 0 && n.Frame[3] > 0 {
			found = n
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	return found, found != nil
}

// Screen is the size of the screen: the root's frame, or the widest window's when the root has none.
func Screen(root *Node) (w, h float64) {
	w, h = root.Frame[2], root.Frame[3]
	for _, c := range root.Children {
		if c.Frame[2] > w && c.Frame[3] > h && w == 0 {
			w, h = c.Frame[2], c.Frame[3]
		}
	}
	return w, h
}
