package langlang

import (
	"fmt"
	"strings"
)

// Node is the interface that defines all behavior needed by the values output by the parser
type AstNode interface {
	// Span returns the location span in which the node was found within the input text
	Span() Span

	// Text is the representation of a grammar node, meant to
	// display just what was captured, being useful for
	// stringifying the grammar again
	Text() string

	// String returns the string representation of a given node
	String() string

	// IsSyntactic returns true only for nodes that are considered
	// syntactical rules.  Outside this module, it makes sense to
	// call this method on a `DefinitionNode`, but it'd then
	// trigger the recursive call needed to answer that question
	// in such level
	IsSyntactic() bool

	Accept(AstNodeVisitor) error
}

// Node Type: Any

type AnyNode struct{ span Span }

func NewAnyNode(s Span) *AnyNode {
	n := &AnyNode{}
	n.span = s
	return n
}

func (n AnyNode) Span() Span                    { return n.span }
func (n AnyNode) IsSyntactic() bool             { return true }
func (n AnyNode) Text() string                  { return "." }
func (n AnyNode) String() string                { return fmt.Sprintf("Any @ %s", n.Span()) }
func (n AnyNode) Accept(v AstNodeVisitor) error { return v.VisitAnyNode(&n) }

// Node Type: Literal

type LiteralNode struct {
	span  Span
	Value string
}

func NewLiteralNode(v string, s Span) *LiteralNode {
	n := &LiteralNode{Value: v}
	n.span = s
	return n
}

func (n LiteralNode) Span() Span                    { return n.span }
func (n LiteralNode) IsSyntactic() bool             { return true }
func (n LiteralNode) Text() string                  { return fmt.Sprintf("'%s'", n.Value) }
func (n LiteralNode) String() string                { return fmt.Sprintf("Literal(%s) @ %s", n.Value, n.Span()) }
func (n LiteralNode) Accept(v AstNodeVisitor) error { return v.VisitLiteralNode(&n) }

// Node Type: Identifier

type IdentifierNode struct {
	span  Span
	Value string
}

func NewIdentifierNode(v string, s Span) *IdentifierNode {
	n := &IdentifierNode{Value: v}
	n.span = s
	return n
}

func (n IdentifierNode) Span() Span                    { return n.span }
func (n IdentifierNode) IsSyntactic() bool             { return false }
func (n IdentifierNode) Text() string                  { return n.Value }
func (n IdentifierNode) String() string                { return fmt.Sprintf("Identifier(%s) @ %s", n.Value, n.Span()) }
func (n IdentifierNode) Accept(v AstNodeVisitor) error { return v.VisitIdentifierNode(&n) }

// Node Type: Range

type RangeNode struct {
	span  Span
	Left  string
	Right string
}

func NewRangeNode(left, right string, s Span) *RangeNode {
	n := &RangeNode{Left: left, Right: right}
	n.span = s
	return n
}

func (n RangeNode) Span() Span                    { return n.span }
func (n RangeNode) IsSyntactic() bool             { return true }
func (n RangeNode) Text() string                  { return fmt.Sprintf("%s-%s", n.Left, n.Right) }
func (n RangeNode) Accept(v AstNodeVisitor) error { return v.VisitRangeNode(&n) }

func (n RangeNode) String() string {
	return fmt.Sprintf("Range(%s, %s) @ %s", n.Left, n.Right, n.Span())
}

// Node Type: Class

type ClassNode struct {
	span  Span
	Items []AstNode
}

func NewClassNode(items []AstNode, s Span) *ClassNode {
	n := &ClassNode{Items: items}
	n.span = s
	return n
}

func (n ClassNode) Span() Span                    { return n.span }
func (n ClassNode) IsSyntactic() bool             { return true }
func (n ClassNode) Text() string                  { return fmt.Sprintf("[%s]", nodesText(n.Items, "")) }
func (n ClassNode) Accept(v AstNodeVisitor) error { return v.VisitClassNode(&n) }

func (n ClassNode) String() string {
	var (
		s  strings.Builder
		ln = len(n.Items) - 1
	)

	s.WriteString("Class(")

	for i, child := range n.Items {
		s.WriteString(child.String())

		if i < ln {
			s.WriteString(", ")
		}
	}

	s.WriteString(") @ ")
	s.WriteString(n.Span().String())

	return s.String()
}

// Node Type: Optional

type OptionalNode struct {
	span Span
	Expr AstNode
}

func NewOptionalNode(expr AstNode, s Span) *OptionalNode {
	n := &OptionalNode{Expr: expr}
	n.span = s
	return n
}

func (n OptionalNode) Span() Span                    { return n.span }
func (n OptionalNode) IsSyntactic() bool             { return n.Expr.IsSyntactic() }
func (n OptionalNode) Text() string                  { return fmt.Sprintf("%s?", n.Expr.Text()) }
func (n OptionalNode) String() string                { return fmt.Sprintf("Optional(%s) @ %s", n.Expr, n.Span()) }
func (n OptionalNode) Accept(v AstNodeVisitor) error { return v.VisitOptionalNode(&n) }

// Node Type: ZeroOrMore

type ZeroOrMoreNode struct {
	span Span
	Expr AstNode
}

func NewZeroOrMoreNode(expr AstNode, s Span) *ZeroOrMoreNode {
	n := &ZeroOrMoreNode{Expr: expr}
	n.span = s
	return n
}

func (n ZeroOrMoreNode) Span() Span                    { return n.span }
func (n ZeroOrMoreNode) IsSyntactic() bool             { return n.Expr.IsSyntactic() }
func (n ZeroOrMoreNode) Text() string                  { return fmt.Sprintf("%s*", n.Expr.Text()) }
func (n ZeroOrMoreNode) String() string                { return fmt.Sprintf("ZeroOrMore(%s) @ %s", n.Expr, n.Span()) }
func (n ZeroOrMoreNode) Accept(v AstNodeVisitor) error { return v.VisitZeroOrMoreNode(&n) }

// Node Type: OneOrMore

type OneOrMoreNode struct {
	span Span
	Expr AstNode
}

func NewOneOrMoreNode(expr AstNode, s Span) *OneOrMoreNode {
	n := &OneOrMoreNode{Expr: expr}
	n.span = s
	return n
}

func (n OneOrMoreNode) Span() Span                    { return n.span }
func (n OneOrMoreNode) IsSyntactic() bool             { return n.Expr.IsSyntactic() }
func (n OneOrMoreNode) Text() string                  { return fmt.Sprintf("%s+", n.Expr.Text()) }
func (n OneOrMoreNode) String() string                { return fmt.Sprintf("OneOrMore(%s) @ %s", n.Expr, n.Span()) }
func (n OneOrMoreNode) Accept(v AstNodeVisitor) error { return v.VisitOneOrMoreNode(&n) }

// Node Type: And

type AndNode struct {
	span Span
	Expr AstNode
}

func NewAndNode(expr AstNode, s Span) *AndNode {
	n := &AndNode{Expr: expr}
	n.span = s
	return n
}

func (n AndNode) Span() Span                    { return n.span }
func (n AndNode) IsSyntactic() bool             { return true }
func (n AndNode) Text() string                  { return fmt.Sprintf("&%s", n.Expr.Text()) }
func (n AndNode) String() string                { return fmt.Sprintf("And(%s) @ %s", n.Expr, n.Span()) }
func (n AndNode) Accept(v AstNodeVisitor) error { return v.VisitAndNode(&n) }

// Node Type: Not

type NotNode struct {
	span Span
	Expr AstNode
}

func NewNotNode(expr AstNode, s Span) *NotNode {
	n := &NotNode{Expr: expr}
	n.span = s
	return n
}

func (n NotNode) Span() Span                    { return n.span }
func (n NotNode) IsSyntactic() bool             { return true }
func (n NotNode) Text() string                  { return fmt.Sprintf("!%s", n.Expr.Text()) }
func (n NotNode) String() string                { return fmt.Sprintf("Not(%s) @ %s", n.Expr, n.Span()) }
func (n NotNode) Accept(v AstNodeVisitor) error { return v.VisitNotNode(&n) }

// Node Type: Lex

type LexNode struct {
	span Span
	Expr AstNode
}

func NewLexNode(expr AstNode, s Span) *LexNode {
	n := &LexNode{Expr: expr}
	n.span = s
	return n
}

func (n LexNode) Span() Span                    { return n.span }
func (n LexNode) IsSyntactic() bool             { return n.Expr.IsSyntactic() }
func (n LexNode) String() string                { return fmt.Sprintf("Lex(%s) @ %s", n.Expr, n.Span()) }
func (n LexNode) Accept(v AstNodeVisitor) error { return v.VisitLexNode(&n) }

func (n LexNode) Text() string {
	if _, ok := n.Expr.(SequenceNode); ok {
		return fmt.Sprintf("#(%s)", n.Expr.Text())
	}
	return fmt.Sprintf("#%s", n.Expr)
}

// Node Type: Labeled

type LabeledNode struct {
	span  Span
	Label string
	Expr  AstNode
}

func NewLabeledNode(label string, expr AstNode, s Span) *LabeledNode {
	n := &LabeledNode{Label: label, Expr: expr}
	n.span = s
	return n
}

func (n LabeledNode) Span() Span                    { return n.span }
func (n LabeledNode) IsSyntactic() bool             { return n.Expr.IsSyntactic() }
func (n LabeledNode) Text() string                  { return fmt.Sprintf("%s^%s", n.Expr.Text(), n.Label) }
func (n LabeledNode) Accept(v AstNodeVisitor) error { return v.VisitLabeledNode(&n) }

func (n LabeledNode) String() string {
	return fmt.Sprintf("Label%s(%s) @ %s", n.Label, n.Expr, n.Span())
}

// Node Type: Sequence

type SequenceNode struct {
	span  Span
	Items []AstNode
}

func NewSequenceNode(items []AstNode, s Span) *SequenceNode {
	n := &SequenceNode{Items: items}
	n.span = s
	return n
}

func (n SequenceNode) Span() Span { return n.span }

func (n SequenceNode) IsSyntactic() bool {
	for _, expr := range n.Items {
		if !expr.IsSyntactic() {
			return false
		}
	}
	return true
}

func (n SequenceNode) Text() string                  { return nodesText(n.Items, " ") }
func (n SequenceNode) String() string                { return nodesString("Sequence", n, n.Items) }
func (n SequenceNode) Accept(v AstNodeVisitor) error { return v.VisitSequenceNode(&n) }

// Node Type: Choice

type ChoiceNode struct {
	span  Span
	Items []AstNode
}

func NewChoiceNode(items []AstNode, s Span) *ChoiceNode {
	n := &ChoiceNode{Items: items}
	n.span = s
	return n
}

func (n ChoiceNode) Span() Span { return n.span }

func (n ChoiceNode) IsSyntactic() bool {
	for _, expr := range n.Items {
		if !expr.IsSyntactic() {
			return false
		}
	}
	return true
}

func (n ChoiceNode) Text() string                  { return nodesText(n.Items, " / ") }
func (n ChoiceNode) String() string                { return nodesString("Choice", n, n.Items) }
func (n ChoiceNode) Accept(v AstNodeVisitor) error { return v.VisitChoiceNode(&n) }

// Node Type: Definition

type DefinitionNode struct {
	span Span
	Name string
	Expr AstNode
}

func NewDefinitionNode(name string, expr AstNode, s Span) *DefinitionNode {
	n := &DefinitionNode{Name: name, Expr: expr}
	n.span = s
	return n
}

func (n DefinitionNode) Span() Span                    { return n.span }
func (n DefinitionNode) IsSyntactic() bool             { return n.Expr.IsSyntactic() }
func (n DefinitionNode) Text() string                  { return fmt.Sprintf("%s <- %s", n.Name, n.Expr.Text()) }
func (n DefinitionNode) Accept(v AstNodeVisitor) error { return v.VisitDefinitionNode(&n) }

func (n DefinitionNode) String() string {
	return fmt.Sprintf("Definition[%s](%s) @ %s", n.Name, n.Expr, n.Span())
}

// Node Type: Import

type ImportNode struct {
	span  Span
	Path  *LiteralNode
	Names []*LiteralNode
}

func NewImportNode(path *LiteralNode, names []*LiteralNode, s Span) *ImportNode {
	n := &ImportNode{Path: path, Names: names}
	n.span = s
	return n
}

func (n ImportNode) Span() Span                    { return n.span }
func (n ImportNode) IsSyntactic() bool             { return false }
func (n ImportNode) Accept(v AstNodeVisitor) error { return v.VisitImportNode(&n) }

func (n ImportNode) Text() string {
	names := strings.Join(n.GetNames(), ", ")
	return fmt.Sprintf("import %s from \"%s\"", names, n.GetPath())
}

func (n ImportNode) String() string {
	names := strings.Join(n.GetNames(), ", ")
	return fmt.Sprintf("Import([%s], %s) @ %s", names, n.GetPath(), n.Span())
}

func (n ImportNode) GetPath() string {
	return n.Path.Value
}

func (n ImportNode) GetNames() []string {
	var names []string
	for _, name := range n.Names {
		names = append(names, name.Value)
	}
	return names
}

// Node Type: Grammar

type GrammarNode struct {
	span        Span
	Imports     []*ImportNode
	Definitions []*DefinitionNode
	DefsByName  map[string]*DefinitionNode
}

func NewGrammarNode(
	imps []*ImportNode,
	defs []*DefinitionNode,
	defsByName map[string]*DefinitionNode,
	s Span,
) *GrammarNode {
	n := &GrammarNode{Imports: imps, Definitions: defs, DefsByName: defsByName}
	n.span = s
	return n
}

func (n GrammarNode) Span() Span                    { return n.span }
func (n GrammarNode) IsSyntactic() bool             { return false }
func (n GrammarNode) Text() string                  { return nodesText(n.GetItems(), "\n") }
func (n GrammarNode) String() string                { return nodesString("Grammar", n, n.GetItems()) }
func (n GrammarNode) Accept(v AstNodeVisitor) error { return v.VisitGrammarNode(&n) }

func (n GrammarNode) GetItems() []AstNode {
	var items []AstNode
	for _, imp := range n.Imports {
		items = append(items, imp)
	}
	for _, def := range n.Definitions {
		items = append(items, def)
	}
	return items
}

// Helpers

type asString interface{ String() string }

func nodesString[T asString](name string, n AstNode, items []T) string {
	var (
		s  strings.Builder
		ln = len(items) - 1
	)

	s.WriteString(name)
	s.WriteString("(")

	for i, child := range items {
		s.WriteString(child.String())

		if i < ln {
			s.WriteString(", ")
		}
	}

	s.WriteString(") @ ")
	s.WriteString(n.Span().String())

	return s.String()
}

type asText interface{ Text() string }

func nodesText[T asText](items []T, sep string) string {
	var (
		s  strings.Builder
		ln = len(items) - 1
	)
	for i, child := range items {
		s.WriteString(child.Text())

		if i < ln {
			s.WriteString(sep)
		}
	}
	return s.String()
}
