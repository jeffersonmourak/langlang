package parsing

import (
	"fmt"
	"strings"
)

type goCodeEmitter struct {
	options     GenGoOptions
	output      *strings.Builder
	indentLevel int
}

type GenGoOptions struct {
	PackageName  string
	StructSuffix string
}

func DefaultGenGoOptions() GenGoOptions {
	return GenGoOptions{
		PackageName:  "parser",
		StructSuffix: "",
	}
}

func newGoCodeEmitter(opt GenGoOptions) *goCodeEmitter {
	emitter := &goCodeEmitter{options: opt, output: &strings.Builder{}}
	emitter.write(fmt.Sprintf(`package %s

import (
	"github.com/clarete/langlang/go"
)

type Parser$StructSuffix struct {
	parsing.BaseParser
}

func NewParser$StructSuffix(input string) *Parser$StructSuffix {
	p := &Parser$StructSuffix{}
	p.SetInput([]rune(input))
	return p
}

func (p *Parser$StructSuffix) ParseAny() (parsing.Value, error) {
	start := p.Location()
	r, err := p.Any()
	if err != nil {
		var zero parsing.Value
		return zero, err
	}
	return parsing.NewValueString(string(r), parsing.NewSpan(start, p.Location())), nil
}

func (p *Parser$StructSuffix) ParseRange(left, right rune) (parsing.Value, error) {
	start := p.Location()
	r, err := p.ExpectRange(left, right)
	if err != nil {
		var zero parsing.Value
		return zero, err
	}
	return parsing.NewValueString(string(r), parsing.NewSpan(start, p.Location())), nil
}

func (p *Parser$StructSuffix) ParseLiteral(literal string) (parsing.Value, error) {
	start := p.Location()
	r, err := p.ExpectLiteral(literal)
	if err != nil {
		var zero parsing.Value
		return zero, err
	}
	return parsing.NewValueString(r, parsing.NewSpan(start, p.Location())), nil
}

func (p *Parser$StructSuffix) ParseSpacing() (parsing.Value, error) {
	start := p.Location()
	v, err := parsing.ZeroOrMore(p, func(p parsing.Parser) (rune, error) {
		return parsing.ChoiceRune(p, []rune{' ', '\t', '\r', '\n'})
	})
	if err != nil {
		return nil, err
	}
	return parsing.NewValueString(string(v), parsing.NewSpan(start, p.Location())), nil
}

func (p *Parser$StructSuffix) ParseEOF() (parsing.Value, error) {
	return (func(p parsing.Parser) (parsing.Value, error) {
		var (
			start = p.Location()
			items []parsing.Value
			item  parsing.Value
			err   error
		)
		item, err = parsing.Not(p, func(p parsing.Parser) (parsing.Value, error) {
			return p.(*Parser$StructSuffix).ParseAny()
		})
		if err != nil {
			return nil, err
		}
		if item != nil {
			items = append(items, item)
		}
		return parsing.NewValueSequence(items, parsing.NewSpan(start, p.Location())), nil
	}(p))
}
`, opt.PackageName))
	return emitter
}

func (g *goCodeEmitter) visit(node Node) {
	switch n := node.(type) {
	case *GrammarNode:
		g.visitGrammarNode(n)
	case *DefinitionNode:
		g.visitDefinitionNode(n)
	case *SequenceNode:
		g.visitSequenceNode(n)
	case *OneOrMoreNode:
		g.visitOneOrMoreNode(n)
	case *ZeroOrMoreNode:
		g.visitZeroOrMoreNode(n)
	case *OptionalNode:
		g.visitOptionalNode(n)
	case *ChoiceNode:
		g.visitChoiceNode(n)
	case *AndNode:
		g.visitAndNode(n)
	case *NotNode:
		g.visitNotNode(n)
	case *LabeledNode:
		g.visitLabeledNode(n)
	case *IdentifierNode:
		g.visitIdentifierNode(n)
	case *LiteralNode:
		g.visitLiteralNode(n)
	case *ClassNode:
		g.visitClassNode(n)
	case *RangeNode:
		g.visitRangeNode(n)
	case *AnyNode:
		g.visitAnyNode()
	}
}

func (g *goCodeEmitter) visitGrammarNode(n *GrammarNode) {
	for _, definition := range n.Items {
		g.visit(definition)
	}
}

func (g *goCodeEmitter) visitDefinitionNode(n *DefinitionNode) {
	g.writeIndent()
	g.write("\nfunc (p *Parser$StructSuffix) Parse")
	g.write(n.Name)
	g.write("() (parsing.Value, error) {\n")
	g.indent()

	g.writei("p.PushTraceSpan")
	fmt.Fprintf(g.output, `(parsing.TracerSpan{Name: "%s"})`, n.Name)
	g.write("\n")
	g.writei("defer p.PopTraceSpan()\n")
	g.writei("return ")
	g.visit(n.Expr)

	g.unindent()
	g.write("\n}\n")
}

func (g *goCodeEmitter) visitSequenceNode(n *SequenceNode) {
	shouldConsumeSpaces := g.isUnderRuleLevel() && !n.IsSyntactic()
	g.write("(func(p parsing.Parser) (parsing.Value, error) {\n")
	g.indent()

	g.writei("var (\n")
	g.indent()
	g.writei("start = p.Location()\n")
	g.writei("items []parsing.Value\n")
	g.writei("item  parsing.Value\n")
	g.writei("err   error\n")
	g.unindent()
	g.writei(")\n")

	for _, item := range n.Items {
		if shouldConsumeSpaces {
			g.writei("item, err = p.(*Parser$StructSuffix).ParseSpacing()\n")
			g.writeIfErr()
			g.writei("items = append(items, item)\n")
		}
		g.writei("item, err = ")
		g.visit(item)
		g.write("\n")
		g.writeIfErr()

		g.writei("if item != nil {\n")
		g.indent()
		g.writei("items = append(items, item)\n")
		g.unindent()
		g.writei("}\n")
	}

	g.writei("return parsing.NewValueSequence(items, parsing.NewSpan(start, p.Location())), nil\n")

	g.unindent()
	g.writei("}(p))")
}

func (g *goCodeEmitter) visitOneOrMoreNode(n *OneOrMoreNode) {
	g.write("(func(p parsing.Parser) (parsing.Value, error) {\n")
	g.indent()

	g.writei("start := p.Location()\n")
	g.writei("items, err := parsing.OneOrMore(p, func(p parsing.Parser) (parsing.Value, error) {\n")
	g.indent()

	g.writei("return ")
	g.visit(n.Expr)
	g.write("\n")

	g.unindent()
	g.writei("})\n")
	g.writeIfErr()

	g.writei("return parsing.NewValueSequence(items, parsing.NewSpan(start, p.Location())), nil\n")

	g.unindent()
	g.writei("}(p))")
}

func (g *goCodeEmitter) visitZeroOrMoreNode(n *ZeroOrMoreNode) {
	g.write("(func(p parsing.Parser) (parsing.Value, error) {\n")
	g.indent()

	g.writei("start := p.Location()\n")
	g.writei("items, err := parsing.ZeroOrMore(p, func(p parsing.Parser) (parsing.Value, error) {\n")
	g.indent()

	g.writei("return ")
	g.visit(n.Expr)
	g.write("\n")

	g.unindent()
	g.writei("})\n")
	g.writeIfErr()

	g.writei("return parsing.NewValueSequence(items, parsing.NewSpan(start, p.Location())), nil\n")

	g.unindent()
	g.writei("}(p))")
}

func (g *goCodeEmitter) visitOptionalNode(n *OptionalNode) {
	g.write("parsing.Choice(p, []parsing.ParserFn[parsing.Value]{\n")
	g.indent()

	g.wirteExprFn(n.Expr)
	g.write(",\n")

	g.writei("func(p parsing.Parser) (parsing.Value, error) {\n")
	g.indent()
	g.writei("return nil, nil\n")
	g.unindent()
	g.writei("},\n")

	g.unindent()
	g.writei("})")
}

func (g *goCodeEmitter) visitChoiceNode(n *ChoiceNode) {
	switch len(n.Items) {
	case 0:
		return
	case 1:
		g.visit(n.Items[0])
	default:
		g.write("parsing.Choice(p, []parsing.ParserFn[parsing.Value]{\n")
		g.indent()

		for _, expr := range n.Items {
			g.wirteExprFn(expr)
			g.write(",\n")
		}

		g.unindent()
		g.writei("})")
	}
}

func (g *goCodeEmitter) visitAndNode(n *AndNode) {
	g.write("parsing.And(p, func(p parsing.Parser) (parsing.Value, error) {\n")
	g.indent()

	g.writei("return ")
	g.visit(n.Expr)
	g.write("\n")

	g.unindent()
	g.writei("})")
}

func (g *goCodeEmitter) visitNotNode(n *NotNode) {
	g.write("parsing.Not(p, func(p parsing.Parser) (parsing.Value, error) {\n")
	g.indent()

	g.writei("return ")
	g.visit(n.Expr)
	g.write("\n")

	g.unindent()
	g.writei("})")
}

func (g *goCodeEmitter) visitLabeledNode(n *LabeledNode) {
	g.write("func(p parsing.Parser) (parsing.Value, error) {\n")
	g.indent()
	g.writei("start = p.Location()\n")

	g.writei("return parsing.Choice(p, []parsing.ParserFn[parsing.Value]{\n")
	g.indent()

	// Write the expression as the first option
	g.wirteExprFn(n.Expr)
	g.write(",\n")

	// if the expression failed, throw an error
	g.writei("func(p parsing.Parser) (parsing.Value, error) {\n")
	g.indent()
	g.writei("return nil, p.Throw")
	g.write(fmt.Sprintf(`("%s", parsing.NewSpan(start, p.Location()))`, n.Label))
	g.write("\n")

	g.unindent()
	g.writei("},\n")

	g.unindent()
	g.writei("})\n")

	g.unindent()
	g.writei("}(p)\n")
}

func (g *goCodeEmitter) visitIdentifierNode(n *IdentifierNode) {
	s := "p.(*Parser$StructSuffix).Parse%s()"
	if g.isAtRuleLevel() {
		s = "p.Parse%s()"
	}
	g.write(fmt.Sprintf(s, n.Value))
}

var quoteSanitizer = strings.NewReplacer(`"`, `\"`)

func (g *goCodeEmitter) visitLiteralNode(n *LiteralNode) {
	s := `p.(*Parser$StructSuffix).ParseLiteral("%s")`
	if g.isAtRuleLevel() {
		s = `p.ParseLiteral("%s")`
	}
	g.write(fmt.Sprintf(s, quoteSanitizer.Replace(n.Value)))
}

func (g *goCodeEmitter) visitClassNode(n *ClassNode) {
	switch len(n.Items) {
	case 0:
	case 1:
		g.visit(n.Items[0])
	default:
		g.write("parsing.Choice(p, []parsing.ParserFn[parsing.Value]{\n")
		g.indent()

		for _, expr := range n.Items {
			g.wirteExprFn(expr)
			g.write(",\n")
		}

		g.unindent()
		g.writei("})")
	}
}

func (g *goCodeEmitter) visitRangeNode(n *RangeNode) {
	s := "p.(*Parser$StructSuffix).ParseRange('%s', '%s')"
	if g.isAtRuleLevel() {
		s = "p.ParseRange('%s', '%s')"
	}
	g.write(fmt.Sprintf(s, n.Left, n.Right))
}

func (g *goCodeEmitter) visitAnyNode() {
	s := "p.(*Parser$StructSuffix).ParseAny()"
	if g.isAtRuleLevel() {
		s = "p.ParseAny()"
	}
	g.write(s)
}

// Utilities to write data into the output buffer

func (g *goCodeEmitter) wirteExprFn(expr Node) {
	g.writei("func(p parsing.Parser) (parsing.Value, error) {\n")
	g.indent()

	g.writei("return ")
	g.visit(expr)
	g.write("\n")

	g.unindent()
	g.writei("}")
}

func (g *goCodeEmitter) writeIfErr() {
	g.writei("if err != nil {\n")
	g.indent()
	g.writei("return nil, err\n")
	g.unindent()
	g.writei("}\n")
}

func (g *goCodeEmitter) writei(s string) {
	g.writeIndent()
	g.write(s)
}

func (g *goCodeEmitter) write(s string) {
	g.output.WriteString(strings.ReplaceAll(s, "$StructSuffix", g.options.StructSuffix))
}

func (g *goCodeEmitter) writeIndent() {
	for i := 0; i < g.indentLevel; i++ {
		g.output.WriteString("	")
	}
}

// Indentation related utilities

func (g *goCodeEmitter) indent() {
	g.indentLevel++
}

func (g *goCodeEmitter) unindent() {
	g.indentLevel--
}

// other helpers

// isInRuleLevel returns true exclusively if the traversal is exactly
// one indent within the `DefinitionNode` traversal.  That's useful to
// know because that's the only level in the generated parser that
// doesn't need type casting the variable `p` from `parsing.Parser`
// into the local concrete `Parser`.
func (g *goCodeEmitter) isAtRuleLevel() bool {
	return g.indentLevel == 1
}

// isUnderRuleLevel returns true when the traversal is any level
// within the `DefinitionNode`.  It's only in that level that we
// should be automatically handling spaces.
func (g *goCodeEmitter) isUnderRuleLevel() bool {
	return g.indentLevel >= 1
}

func (g *goCodeEmitter) String() string {
	return g.output.String()
}

func GenGo(node Node, opt GenGoOptions) (string, error) {
	g := newGoCodeEmitter(opt)
	g.visit(node)
	return g.String(), nil
}
