package browser

import (
	"encoding/json"
	"fmt"
	"strings"
)

type axValue struct {
	Value any `json:"value"`
}

type axNode struct {
	ID       string   `json:"nodeId"`
	Ignored  bool     `json:"ignored"`
	Role     *axValue `json:"role"`
	Name     *axValue `json:"name"`
	Value    *axValue `json:"value"`
	Children []string `json:"childIds"`
	Parent   string   `json:"parentId"`
	Backend  int      `json:"backendDOMNodeId"`
	Props    []struct {
		Name  string  `json:"name"`
		Value axValue `json:"value"`
	} `json:"properties"`
}

func (v *axValue) text() string {
	if v == nil || v.Value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v.Value))
}

// quiet are the roles that only group: one without a name is left out and its children move up.
var quiet = map[string]bool{"generic": true, "none": true, "presentation": true, "group": true, "paragraph": true, "div": true,
	"LayoutTable": true, "LayoutTableRow": true, "LayoutTableCell": true, "LineBreak": true}

// shownProps are the states worth a word in the tree.
var shownProps = map[string]bool{"checked": true, "disabled": true, "expanded": true, "selected": true, "focused": true,
	"pressed": true, "required": true}

// Tree is the page's accessibility tree, one node a line: its role, its name, its state and its ref (e12) for click and type.
func (p *Page) Tree() (string, error) {
	raw, err := p.c.call("Accessibility.getFullAXTree", nil)
	if err != nil {
		return "", err
	}
	var r struct {
		Nodes []axNode `json:"nodes"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return "", err
	}
	byID := map[string]*axNode{}
	for i := range r.Nodes {
		byID[r.Nodes[i].ID] = &r.Nodes[i]
	}
	var out strings.Builder
	for i := range r.Nodes {
		if n := &r.Nodes[i]; n.Parent == "" || byID[n.Parent] == nil {
			write(&out, byID, n, 0, "")
		}
	}
	return out.String(), nil
}

const nameMax = 200

func write(out *strings.Builder, byID map[string]*axNode, n *axNode, depth int, parentName string) {
	role, name := n.Role.text(), n.Name.text()
	if len(name) > nameMax {
		name = name[:nameMax] + "…"
	}
	shown := !n.Ignored && !(quiet[role] && name == "") && role != "InlineTextBox" &&
		!(role == "StaticText" && (name == "" || name == parentName))
	if shown {
		line := strings.Repeat("  ", depth) + "- " + role
		if name != "" {
			line += fmt.Sprintf(" %q", name)
		}
		if v := n.Value.text(); v != "" && v != name {
			line += fmt.Sprintf(" value=%q", v)
		}
		for _, pr := range n.Props {
			if !shownProps[pr.Name] {
				continue
			}
			switch v := pr.Value.Value.(type) {
			case bool:
				if v {
					line += " " + pr.Name
				}
			case string:
				if v != "" && v != "false" {
					line += " " + pr.Name + "=" + v
				}
			}
		}
		if n.Backend != 0 && role != "StaticText" {
			line += fmt.Sprintf(" [e%d]", n.Backend)
		}
		out.WriteString(line + "\n")
		depth++
		parentName = name
	}
	for _, id := range n.Children {
		if c := byID[id]; c != nil {
			write(out, byID, c, depth, parentName)
		}
	}
}
