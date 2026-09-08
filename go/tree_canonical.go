package langlang

import (
	"fmt"
	"strings"
)

// CanonicalDump renders a match result in the line format the Zig
// runtime's `Tree.dump` and the test drivers under go/zig produce, so the
// Go VM and a generated Zig parser can be compared byte for byte:
//
//	cursor=<n>
//	error label=<label or -> start=<s> end=<e> tree=<yes|no> msg=<text>   (on failure)
//	root=<id> or noroot, then one pre-order line per node:
//	<2*depth spaces>#<id> <string|sequence|node|err> <name or -> <start> <end>[ msg=<message>]
//
// Node ids are part of the format on purpose: they pin the builders' append
// order, which the public Tree interface cannot observe.
func CanonicalDump(t Tree, cursor int, err error) string {
	var b strings.Builder
	fmt.Fprintf(&b, "cursor=%d\n", cursor)
	if err != nil {
		pe, ok := err.(ParsingError)
		if !ok {
			fmt.Fprintf(&b, "error unknown=%v\n", err)
		} else {
			label := pe.Label
			if label == "" {
				label = "-"
			}
			hasTree := "no"
			if t != nil {
				hasTree = "yes"
			}
			fmt.Fprintf(&b, "error label=%s start=%d end=%d tree=%s msg=%s\n", label, pe.Start, pe.End, hasTree, pe.Message)
		}
	}
	if t == nil {
		return b.String()
	}
	tt := t.(*tree)
	if !tt.hasRoot {
		b.WriteString("noroot\n")
		return b.String()
	}
	fmt.Fprintf(&b, "root=%d\n", tt.root)
	canonicalNode(&b, tt, tt.root, 0)
	return b.String()
}

func canonicalNode(b *strings.Builder, t *tree, id NodeID, depth int) {
	n := t.nodes[id]
	b.WriteString(strings.Repeat("  ", depth))
	name := "-"
	if n.nameID >= 0 {
		name = t.strs[n.nameID]
	}
	fmt.Fprintf(b, "#%d %s %s %d %d", id, canonicalTypeName(n.typ), name, n.start, n.end)
	if n.typ == NodeType_Error {
		fmt.Fprintf(b, " msg=%s", t.strs[n.messageID])
	}
	b.WriteByte('\n')
	for _, child := range t.Children(id) {
		canonicalNode(b, t, child, depth+1)
	}
}

func canonicalTypeName(typ NodeType) string {
	switch typ {
	case NodeType_String:
		return "string"
	case NodeType_Sequence:
		return "sequence"
	case NodeType_Node:
		return "node"
	case NodeType_Error:
		return "err"
	}
	return "?"
}
