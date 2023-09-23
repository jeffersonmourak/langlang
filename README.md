
# Table of Contents

1.  [Introduction](#org4bcd411)
    1.  [Currently supported output languages](#org252fdd4)
        1.  [Notes](#orgcd042ce)
    2.  [Basic Usage](#org4250b20)
2.  [Input Language](#org5906f55)
    1.  [Productions and Expressions](#orgb378f25)
    2.  [Terminals](#org379c32a)
    3.  [Non-Terminals](#orgd2eb62d)
    4.  [Expression Composition](#orge91499c)
        1.  [Ordered Choice](#orgc13ecc9)
        2.  [Predicates (Not/And)](#org1910c26)
        3.  [Repetition ({Zero,One} Or More)](#orgdc599e6)
        4.  [Lexification](#orgc9dc5f7)
        5.  [Error reporting with Labels](#orgafba795)
        6.  [Import system](#org7a5d477)
3.  [Generator Options](#org14d7e62)
    1.  [Go](#org3db93f0)
4.  [Roadmap](#org2eee97c)


<a id="org4bcd411"></a>

# Introduction

Bring your own grammar and get a feature rich parser generated for
different languages.  The are reasons why you might want to use this:

-   Concise input grammar format and intuitive algorithm: generates
    recursive top-down parsers based on Parsing Expression Grammars
-   Automatic handling of white spaces, making grammars less cluttered
-   Error reporting with custom messages via failure `labels`
-   Partial support for declaring error recovery rules, which allow
    incremental parsing that returns an output tree even upon multiple
    parsing errors.


<a id="org252fdd4"></a>

## Currently supported output languages

-   [X] Rust¹
-   [X] Go Lang²
-   [ ] Python
-   [ ] Java Script
-   [ ] Write your own code generator


<a id="orgcd042ce"></a>

### Notes

1.  Rust is supported with a runtime virtual machine that, and not
    with a generated parser.  This design may or may not change.

2.  We're in the middle of dropping the Go implementation of the
    library in favor of a generating a go parser from the code written
    in Rust.


<a id="org4250b20"></a>

## Basic Usage

If you just want to test the waters, point the command line utility at
a grammar and pick a starting rule:

    cargo run --bin langlang run --grammar-file grammars/json.peg --start-rule JSON

That will drop you into an initeractive shell that allows you to try
out different input expressions.

Take a look at other examples at the directory `grammars` in the root
of the repository.  It contains a grammar library for commonly used
input formats.


<a id="org5906f55"></a>

# Input Language


<a id="orgb378f25"></a>

## Productions and Expressions

The input grammar is as simple as it can get.  It builds off of the
original PEG format, and other features are added conservatively.
Take the following input as an example:

    Production <- Expression

At the left side of the arrow there is an identifier and on the right
side, there is an expression.  These two together are called either
productions or (parsing) rules.  Let's go over how to compose them.
If you've ever seen or used regular expressions, you've got a head
start.


<a id="org379c32a"></a>

## Terminals

-   **Any**: matches any character, and only errors if it reaches
    the end of the input.  e.g.: `.`

-   **Literal**: anything around quotes (single and double quotes are the
    same).  e.g.: `'x'`

-   **Class and Range**: classes may contain either ranges or single
    characters.  e.g.: `[0-9]`, `[a-zA-Z]`, `[a-f0-9_]`.  This last
    example contains two ranges (`a-f` and `0-9`) and one single char
    (`_`).  It means **match either one of these**. e.g.: `[a-cA-C]` is
    translated to `'a' / 'b' / 'c' / 'A' / 'B' / 'C'`.


<a id="orgd2eb62d"></a>

## Non-Terminals

The biggest addition of this type of grammar on top of regular
expressions is the ability to define and recursively call productions.
Here's a grammar snippet for parsing numbers:

    Signed   <- ('-' / '+') Signed / Decimal
    Decimal  <- ([1-9][0-9]*) / '0'

The topmost production `Signed` calls itself or the production
`Decimal`.  It allows parsing signed and unsigned numbers
recursively. (e.g.: `+-+--1` and so forth would be accepted).


<a id="orge91499c"></a>

## Expression Composition

The following operators can be used on both Terminals and
Non-Terminals, on top of parenthesized expressions:

<table border="2" cellspacing="0" cellpadding="6" rules="groups" frame="hsides">


<colgroup>
<col  class="org-left" />

<col  class="org-left" />

<col  class="org-left" />
</colgroup>
<thead>
<tr>
<th scope="col" class="org-left">operator</th>
<th scope="col" class="org-left">example</th>
<th scope="col" class="org-left">comment</th>
</tr>
</thead>

<tbody>
<tr>
<td class="org-left"><b>ordered choice</b></td>
<td class="org-left"><code>e1 / e2</code></td>
<td class="org-left">&#xa0;</td>
</tr>


<tr>
<td class="org-left"><b>not predicate</b></td>
<td class="org-left"><code>!e</code></td>
<td class="org-left">&#xa0;</td>
</tr>


<tr>
<td class="org-left"><b>and predicate</b></td>
<td class="org-left"><code>&amp;e</code></td>
<td class="org-left">sugar for <code>!!e</code></td>
</tr>


<tr>
<td class="org-left"><b>zero or more</b></td>
<td class="org-left"><code>e*</code></td>
<td class="org-left">&#xa0;</td>
</tr>


<tr>
<td class="org-left"><b>one or more</b></td>
<td class="org-left"><code>e+</code></td>
<td class="org-left">sugar for <code>ee*</code></td>
</tr>


<tr>
<td class="org-left"><b>optional</b></td>
<td class="org-left"><code>e?</code></td>
<td class="org-left">sugar for <code>&amp;ee / !e</code></td>
</tr>


<tr>
<td class="org-left"><b>lexification</b></td>
<td class="org-left"><code>#e</code></td>
<td class="org-left">&#xa0;</td>
</tr>


<tr>
<td class="org-left"><b>label</b></td>
<td class="org-left"><code>e^label</code></td>
<td class="org-left">sugar for <code>e/throw(label)</code></td>
</tr>
</tbody>
</table>


<a id="orgc13ecc9"></a>

### Ordered Choice

This operator tries expressions one at a time, from left to right, and
stops at the first one to succeed.  Or error if no alternatives work.
E.g.:

    SomeDigits <- '0' / '1' / '2' / '3' / '4'

Passing `6` to the above expression will generate an error.


<a id="org1910c26"></a>

### Predicates (Not/And)

Predicates are the mechanism that allows unlimited look ahead, as they
do not consume any input.  e.g.:

    BracketString <- "[" (!"]" .)* "]"

In the above example, the **any** expression isn't evaluated if the
parser finds the closing square bracket.

The **and** predicate (`&`) is just syntactical sugar for `!!`.


<a id="orgdc599e6"></a>

### Repetition ({Zero,One} Or More)

-   **Zero Or More**: it never fails, as it can match its expression at
    least zero times.

-   **One Or More** is the syntax sugar for calling the expression once,
    followed by applying zero or more to the same expression.  It can
    fail at the first time it matches the expression.

-   **Optional** it will match an expression zero or one time.


<a id="orgc9dc5f7"></a>

### Lexification

By default, the generated parsers emit code to consume whitespaces
automatically before each item within a sequence of a production
that's considered not syntactic.  Productions are considered syntactic
if all their expressions are syntactic.  Expressions are considered
syntactic if their output tree is composed only of terminal matches.
If there's any path to a non-terminal match, the entire expression,
and production are considered non syntactic.  e.g.:

    NotSyntactic <- Syntactic "!"
    Syntactic    <- "a" "b" "c"

In the above example, there is no automatic space consumption injected
before the items of the sequence expression `"a" "b" "c"` as all of
them are terminals.  And the `NotSyntactic` production contains non
terminal calls, which makes it non-syntactic.  Therefore, automatic
space handling will be enabled for `NotSyntactic` and disabled for
`Syntactic`

For **disabling** automatic space handling of an expression, prefix it
with the lexification operator `#`. e.g.:

    Ordinal <- Decimal #('st' / 'nd' / 'rd' / 'th')^ord
    Decimal <- ([1-9][0-9]*) / '0'

In the above expression, `Decimal` is considered syntactic, which
disables automatic space handling.  `Ordinal` is not syntactic because
it calls out to another production with a non-terminal.  So, automatic
space handling is enabled for that production.  However, between the
non-terminal and the choice with terminals, space handling is
disabled.  This is what is expected

<table border="2" cellspacing="0" cellpadding="6" rules="groups" frame="hsides">


<colgroup>
<col  class="org-left" />

<col  class="org-left" />
</colgroup>
<thead>
<tr>
<th scope="col" class="org-left">Input</th>
<th scope="col" class="org-left">Result</th>
</tr>
</thead>

<tbody>
<tr>
<td class="org-left">" 3rd"</td>
<td class="org-left">succeeds</td>
</tr>


<tr>
<td class="org-left">"50th"</td>
<td class="org-left">succeeds</td>
</tr>


<tr>
<td class="org-left">"2 0th"</td>
<td class="org-left">fails</td>
</tr>


<tr>
<td class="org-left">"2 th"</td>
<td class="org-left">fails</td>
</tr>
</tbody>
</table>

The first input succeeds because space consumption is automatically
added to the left of the call to the non terminal `Decimal`, as
`Ordinal` is not syntactic.  But, because the expression that follows
the non terminal is marked with the lexification operator, automatic
space handling won't be injected between the call to the non terminal
and the ordered choice with the syntactic suffixed `st`, `nd`, `rd`,
and `th`.

Here is maybe the most classic example of where lexification is
needed: Non-Syntactic String Literals.  Which uses eager look ahead
and spaces are significant.  e.g.:

    SyntacticStringLiteral     <- '"' (!'"' .) '"'
    NonSyntacticStringLiteral  <- DQ #((!DQ .)  DQ)

Without using the lexification operator on the rule
`NonSyntacticStringLiteral`, it would eat up the spaces after the
first quote, which can be undesired for string fields.

The rule `SyntacticStringLiteral` doesn't need the lexification
operator because all of its sub-expressions are terminals, therefore
the rule is syntactic and space consumption won't be generated by
default anyway.

There are definitely more use-cases of the lexification operator out
there, these are just the common ones.


<a id="orgafba795"></a>

### Error reporting with Labels


<a id="org7a5d477"></a>

### Import system

Productions of one grammar can be imported from another one.  That
allows reusing rules and delivering more consolidate grammar files and
more powerful parser generated at the end.

    // file player.peg
    @import AddrSpec from "./rfc5322.peg"
    
    Player <- "Name:" Name "," "Score:" Number "," "Email:" AddrSpec
    Name   <- [a-zA-Z ]+
    Number <- [0-9]+
    // ... elided for simplicity

    // file rfc5322.peg
    // https://datatracker.ietf.org/doc/html/rfc5322#section-3.4.1
    
    // ... elided for simplicity
    AddrSpec  <- LocalPart "@" Domain
    LocalPart <- DotAtom / QuotedString / ObsLocalPart
    Domain    <- DotAtom / DomainLiteral / ObsDomain
    // ... elided for simplicity

The above example illustrates that a rather complete email parser can
be used in other grammars using imports.  Behind the scenes, the
`AddrSpec` rule and all its dependencies have been merged into the
`player.peg` grammar.


<a id="org14d7e62"></a>

# Generator Options


<a id="org3db93f0"></a>

## Go

The Go code generator provides the following additional knobs to the
command line:

-   `--go-package`: allows customizing what goes in the `package`
    directive that starts each Go file.

-   `--go-prefix`: allows customizing structs generated prefixing what
    is passed to this option.  This is especially useful if there are
    two grammars to be parsed in the same package.  At least one will
    need a prefix, so the generic `Parser` name doesn't collide. e.g.:
    `-go-prefix Tiny` would generate a `TinyParser` struct, a
    `NewTinyParser` constructor, etc.


<a id="org2eee97c"></a>

# Roadmap

-   [ ] MID: [gen<sub>go</sub>] rewrite Go generator in Rust
-   [ ] MID: [genall] generator interface to be shared by all targets
-   [ ] SML: [gen<sub>go</sub>] memoize results to guarantee O(1) parsing time
-   [ ] SML: [gen<sub>go</sub>] allocate output nodes in an arena
-   [ ] MID: [gen<sub>py</sub>] Python Code Generator: Start from scratch
-   [ ] MID: [gen<sub>js</sub>] Java Script Code Generator
-   [ ] MID: [gen<sub>go</sub>] explore generating Go ASM code instead of text
-   [ ] MID: Display Call Graph for debugging purposes
-   [ ] BIG: Bootstrap off hand written parser, so grammar writters can
    take advantage of the features baked into the parser generator

