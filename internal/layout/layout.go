package layout

import (
	"fmt"
	"strings"
)

// Node is a layout cell: a leaf (Pane != "") or a split.
type Node struct {
	W, H, X, Y int
	Pane       string // "%12" for leaves
	Horiz      bool   // children side by side ({}) vs stacked ([])
	Kids       []*Node
	pct        int // first child's share (main+stack preset)
}

// Input describes the window to lay out.
type Input struct {
	W, H   int
	Sup    string   // supervisor pane id ("" if hidden/absent)
	Ov     string   // overview pane id
	Wall   []string // wall pane ids in slot order
	Preset string
}

// Build returns the full layout tree.
func (c Config) Build(in Input) *Node {
	root := &Node{W: in.W, H: in.H}
	right := root
	if in.Sup != "" && !c.SupHidden {
		sw := c.SupCols(in.W)
		if sw > in.W-20 {
			sw = in.W / 2
		}
		sup := &Node{Pane: in.Sup}
		right = &Node{}
		root.Horiz = true
		if c.SupSide == "right" {
			root.Kids = []*Node{right, sup}
		} else {
			root.Kids = []*Node{sup, right}
		}
		sizes(root, []int{sw, in.W - sw - 1}, c.SupSide == "right")
	}
	ov := &Node{Pane: in.Ov}
	if len(in.Wall) == 0 {
		*right = Node{W: right.W, H: right.H, X: right.X, Y: right.Y, Pane: in.Ov}
		return root
	}
	wall := c.wallTree(in.Wall, in.Preset)
	right.Horiz = false
	right.Kids = []*Node{ov, wall}
	oh := c.OvRows()
	if oh > right.H-4 {
		oh = right.H / 3
	}
	sizes(right, []int{oh, right.H - oh - 1}, false)
	return root
}

func (c Config) wallTree(panes []string, preset string) *Node {
	leaf := func(p string) *Node { return &Node{Pane: p} }
	if len(panes) == 1 {
		return leaf(panes[0])
	}
	if preset == "m" {
		stack := &Node{}
		for _, p := range panes[1:] {
			stack.Kids = append(stack.Kids, leaf(p))
		}
		if len(stack.Kids) == 1 {
			stack = stack.Kids[0]
		}
		return &Node{Horiz: true, Kids: []*Node{leaf(panes[0]), stack}, pct: c.MainPercent}
	}
	cols := gridCols(preset)
	if cols > len(panes) {
		cols = len(panes)
	}
	var rows []*Node
	for i := 0; i < len(panes); i += cols {
		row := &Node{Horiz: true}
		for j := i; j < i+cols && j < len(panes); j++ {
			row.Kids = append(row.Kids, leaf(panes[j]))
		}
		if len(row.Kids) == 1 {
			row = row.Kids[0]
		}
		rows = append(rows, row)
	}
	if len(rows) == 1 {
		return rows[0]
	}
	return &Node{Kids: rows}
}

// sizes sets the parent's children sizes along its axis; fixed[0] is the first child's size.
// If fromEnd, the first fixed size applies to the last child instead.
func sizes(n *Node, fixed []int, fromEnd bool) {
	if fromEnd {
		fixed = []int{fixed[1], fixed[0]}
	}
	pos := n.X
	if !n.Horiz {
		pos = n.Y
	}
	for i, k := range n.Kids {
		if n.Horiz {
			k.X, k.Y, k.W, k.H = pos, n.Y, fixed[i], n.H
		} else {
			k.X, k.Y, k.W, k.H = n.X, pos, n.W, fixed[i]
		}
		pos += fixed[i] + 1
	}
}

// split divides total into n parts separated by 1-cell borders.
func split(total, n int) []int {
	avail := total - (n - 1)
	out := make([]int, n)
	for i := range out {
		out[i] = avail / n
		if i < avail%n {
			out[i]++
		}
	}
	return out
}

// place fills in geometry for subtrees whose children have no size yet.
func place(n *Node) {
	if len(n.Kids) == 0 {
		return
	}
	if k := n.Kids[0]; k.W == 0 && k.H == 0 {
		total := n.H
		if n.Horiz {
			total = n.W
		}
		parts := split(total, len(n.Kids))
		if n.pct > 0 && len(n.Kids) == 2 {
			first := (total - 1) * n.pct / 100
			parts = []int{first, total - 1 - first}
		}
		sizes(n, parts, false)
	}
	for _, k := range n.Kids {
		place(k)
	}
}

// String renders the tmux layout body (without checksum).
func (n *Node) String() string {
	s := fmt.Sprintf("%dx%d,%d,%d", n.W, n.H, n.X, n.Y)
	if len(n.Kids) == 0 {
		return s + "," + strings.TrimPrefix(n.Pane, "%")
	}
	open, close := "[", "]"
	if n.Horiz {
		open, close = "{", "}"
	}
	var ks []string
	for _, k := range n.Kids {
		ks = append(ks, k.String())
	}
	return s + open + strings.Join(ks, ",") + close
}

// Leaves returns pane ids in traversal order (the order tmux assigns panes).
func (n *Node) Leaves() []string {
	if len(n.Kids) == 0 {
		return []string{n.Pane}
	}
	var out []string
	for _, k := range n.Kids {
		out = append(out, k.Leaves()...)
	}
	return out
}

// Checksum is tmux's layout checksum.
func Checksum(s string) string {
	var csum uint16
	for i := 0; i < len(s); i++ {
		csum = (csum >> 1) + ((csum & 1) << 15)
		csum += uint16(s[i])
	}
	return fmt.Sprintf("%04x", csum)
}

// Render returns the full "csum,body" string for select-layout.
func (c Config) Render(in Input) (string, []string) {
	root := c.Build(in)
	place(root)
	body := root.String()
	return Checksum(body) + "," + body, root.Leaves()
}
