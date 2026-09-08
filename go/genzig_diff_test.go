package langlang

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The differential harness: the in-process Go VM and the Zig runtime parse
// the same input from the same bytecode and must print the same canonical
// dump (CanonicalDump here, Tree.dump plus driver_common.zig there), node
// ids included.  Two drivers exist: vmdriver replays runtime tables from a
// blob so the whole VM test table runs against one binary, and testdriver
// wraps a generated parser.zig so the generated file is exercised too.

// zigTableBlob serialises a Bytecode for go/zig/vmdriver.zig.  Call it
// before CompileErrorLabels, which appends message strings to strs.
func zigTableBlob(bt *Bytecode) []byte {
	var out []byte
	u32 := func(v uint32) { out = binary.LittleEndian.AppendUint32(out, v) }
	out = append(out, "LLZT"...)
	u32(1)
	u32(uint32(len(bt.code)))
	out = append(out, bt.code...)
	u32(uint32(len(bt.strs)))
	for _, s := range bt.strs {
		u32(uint32(len(s)))
		out = append(out, s...)
	}
	u32(uint32(len(bt.sets)))
	for _, set := range bt.sets {
		out = append(out, set.bits[:]...)
	}
	for _, item := range bt.sexp {
		u32(uint32(len(item)))
		for _, e := range item {
			u32(uint32(e.a))
			u32(uint32(e.b))
		}
	}
	u32(uint32(len(bt.strs)))
	for id := range bt.strs {
		addr := int32(-1)
		if a, ok := bt.rxps[id]; ok {
			addr = int32(a)
		}
		u32(uint32(addr))
	}
	return out
}

func copyZigFile(t *testing.T, dir, name string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("zig", name))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0644))
}

// buildVMDriver compiles go/zig/vmdriver.zig once per test and returns
// the executable path.
func buildVMDriver(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []string{"langlang_runtime.zig", "driver_common.zig", "vmdriver.zig"} {
		copyZigFile(t, dir, f)
	}
	runZig(t, dir, "build-exe", "vmdriver.zig", "-O", "Debug")
	return filepath.Join(dir, "vmdriver")
}

// buildParserDriver generates parser.zig for one grammar and compiles
// go/zig/testdriver.zig against it.
func buildParserDriver(t *testing.T, g zigGrammar) string {
	t.Helper()
	dir := t.TempDir()
	out := compileZigGrammar(t, g, GenZigOptions{})
	require.NoError(t, os.WriteFile(filepath.Join(dir, "parser.zig"), []byte(out), 0644))
	for _, f := range []string{"driver_common.zig", "testdriver.zig"} {
		copyZigFile(t, dir, f)
	}
	runZig(t, dir, "build-exe", "testdriver.zig", "-O", "Debug")
	return filepath.Join(dir, "testdriver")
}

func runDriver(t *testing.T, exe string, args ...string) string {
	t.Helper()
	cmd := exec.Command(exe, args...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s %s failed:\n%s", exe, strings.Join(args, " "), out)
	return string(out)
}

func messagesArg(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+labels[k])
	}
	return strings.Join(parts, ";")
}

func goDump(bt *Bytecode, labels map[string]string, input []byte) string {
	return goDumpMode(bt, labels, input, false)
}

func goDumpMode(bt *Bytecode, labels map[string]string, input []byte, showFails bool) string {
	vm := NewVirtualMachine(bt)
	vm.SetShowFails(showFails)
	vm.SetLabelMessages(bt.CompileErrorLabels(labels))
	tree, cur, err := vm.Match(input)
	return CanonicalDump(tree, cur, err)
}

// modes runs a comparison with show_fails off and on; the second exercises
// the expected-hint tracking, the sexp tables and the "Expected ... but got"
// message path on both sides.
var modes = []struct {
	name      string
	showFails bool
	args      []string
}{
	{"plain", false, nil},
	{"show_fails", true, []string{"--show-fails"}},
}

// zigEdgeTests extends the VM table with the cases the Zig runtime must get
// right byte for byte around non-ASCII input: char32/range32 operands,
// multi-byte and truncated or invalid UTF-8 sequences, and `.` stepping
// over garbage one byte at a time.
var zigEdgeTests = []vmTest{
	{Name: "Char32 match", Grammar: "G <- '🧠'", Input: "🧠"},
	{Name: "Char32 mismatch", Grammar: "G <- '🧠'", Input: "a"},
	{Name: "Char32 empty", Grammar: "G <- '🧠'", Input: ""},
	{Name: "Char32 truncated", Grammar: "G <- '🧠'", Input: "\xf0\x9f"},
	{Name: "Range32 match", Grammar: "G <- [😀-🙏]+", Input: "😀🙏😃"},
	{Name: "Range32 mismatch", Grammar: "G <- [😀-🙏]+", Input: "😀a"},
	{Name: "Range32 invalid lead", Grammar: "G <- [😀-🙏]+", Input: "\xff"},
	{Name: "Two byte literal", Grammar: "G <- 'a' [b-d] 'é'", Input: "abé"},
	{Name: "Two byte truncated", Grammar: "G <- 'a' [b-d] 'é'", Input: "ab\xc3"},
	{Name: "Two byte wrong", Grammar: "G <- 'a' [b-d] 'é'", Input: "abe"},
	{Name: "Any over invalid bytes", Grammar: "G <- (!'x' .)* 'x'", Input: "\xff\xfe \xc0x"},
	{Name: "Any over overlong", Grammar: "G <- .*", Input: "a\xc0\x80b"},
	{Name: "Surrogate is three garbage bytes", Grammar: "G <- .*", Input: "\xed\xa0\x80"},
	{Name: "Hiragana class", Grammar: "G <- [\u3040-\u309F]+", Input: "あいう"},
	{Name: "Hiragana class mismatch", Grammar: "G <- [\u3040-\u309F]+", Input: "あa"},
	{Name: "Set never matches a lead byte", Grammar: "G <- [a-z]+", Input: "abcé"},
	{Name: "Expected hints dedupe and drop whitespace", Grammar: "G <- ' ' 'a' / 'a' 'b' / [a-c]", Input: "d"},
	{Name: "Expected hints cap at twenty", Grammar: "G <- 'a' / 'b' / 'c' / 'd' / 'e' / 'f' / 'g' / 'h' / 'i' / 'j' / 'k' / 'l' / 'm' / 'n' / 'o' / 'p' / 'q' / 'r' / 's' / 't' / 'u' / 'v' / 'w' / 'x'", Input: "z"},
}

// TestGenZigDifferential/vm replays the VM test table in its four
// configurations through the Zig runtime; /grammars generates a parser
// for each non-LR test grammar and replays the inputs under
// tests/<grammar>/inputs.
func TestGenZigDifferential(t *testing.T) {
	requireZig(t)

	t.Run("vm", func(t *testing.T) {
		driver := buildVMDriver(t)
		work := t.TempDir()
		configs := []struct {
			name     string
			optimize int
			charsets bool
		}{
			{"With_Charset_O0", 0, true},
			{"With_Charset_O1", 1, true},
			{"NO_Charset_O0", 0, false},
			{"NO_Charset_O1", 1, false},
		}
		table := append(append([]vmTest{}, vmTests...), zigEdgeTests...)
		for _, cfgSpec := range configs {
			for i, test := range table {
				name := fmt.Sprintf("%s/%s", cfgSpec.name, test.Name)
				t.Run(name, func(t *testing.T) {
					cfg := NewConfig()
					cfg.SetInt("compiler.optimize", cfgSpec.optimize)
					cfg.SetBool("grammar.add_charsets", cfgSpec.charsets)
					loader := NewInMemoryImportLoader()
					loader.Add("test.peg", []byte(test.Grammar))
					db := NewDatabase(cfg, loader)
					bt, err := QueryBytecode(db, "test.peg")
					require.NoError(t, err)

					blob := zigTableBlob(bt)
					tables := filepath.Join(work, fmt.Sprintf("%s_%d.tables", cfgSpec.name, i))
					input := filepath.Join(work, fmt.Sprintf("%s_%d.input", cfgSpec.name, i))
					require.NoError(t, os.WriteFile(tables, blob, 0644))
					require.NoError(t, os.WriteFile(input, []byte(test.Input), 0644))

					for _, mode := range modes {
						args := []string{"--tables", tables, "--input", input}
						if len(test.ErrLabels) > 0 {
							args = append(args, "--messages", messagesArg(test.ErrLabels))
						}
						args = append(args, mode.args...)
						zig := runDriver(t, driver, args...)
						expected := goDumpMode(bt, test.ErrLabels, []byte(test.Input), mode.showFails)
						assert.Equal(t, expected, zig, "mode %s, grammar:\n%s\ninput: %q", mode.name, test.Grammar, test.Input)
					}
				})
			}
		}
	})

	t.Run("grammars", func(t *testing.T) {
		for _, g := range zigGrammars {
			g := g
			t.Run(g.name, func(t *testing.T) {
				cfg := zigTestConfig()
				if g.setup != nil {
					g.setup(cfg)
				}
				db := NewDatabase(cfg, NewRelativeImportLoader())
				bt, err := QueryBytecode(db, g.path)
				require.NoError(t, err)
				inputs := grammarInputs(t, g)
				if len(inputs) == 0 {
					t.Skip("no inputs for this grammar")
				}
				driver := buildParserDriver(t, g)
				for _, path := range inputs {
					path := path
					t.Run(filepath.Base(path), func(t *testing.T) {
						data, err := os.ReadFile(path)
						require.NoError(t, err)
						for _, mode := range modes {
							args := append([]string{"--input", path}, mode.args...)
							zig := runDriver(t, driver, args...)
							expected := goDumpMode(bt, nil, data, mode.showFails)
							assert.Equal(t, expected, zig, "mode %s", mode.name)
						}
					})
				}
			})
		}
	})
}

// TestGenZigLeftRecursiveEntry starts a match directly at a left-recursive
// rule through matchAddress(addr, true).  The Go MatchRule crashes on that
// path (it pushes a plain call frame that return_lr cannot unwind), so
// the oracle is Go's Match on a grammar whose first rule is the LR one:
// the prologue calls it with precedence 1, which is what the LR entry
// reproduces.
func TestGenZigLeftRecursiveEntry(t *testing.T) {
	requireZig(t)
	g := zigGrammar{name: "leftrec_entry", path: "tests/arithmetic_leftrec/expr_only.peg", setup: disableCaptureSpaces}
	cfg := zigTestConfig()
	g.setup(cfg)
	db := NewDatabase(cfg, NewRelativeImportLoader())
	program, err := QueryProgram(db, g.path)
	require.NoError(t, err)
	rules := zigRules(program)
	require.NotEmpty(t, rules)
	require.True(t, rules[0].leftRecursive, "the first rule must be left recursive for this test to mean anything")
	bt, err := QueryBytecode(db, g.path)
	require.NoError(t, err)

	driver := buildParserDriver(t, g)
	for _, path := range grammarInputs(t, zigGrammar{name: "arithmetic_leftrec", path: "tests/arithmetic_leftrec/arithmetic.peg"}) {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			expected := goDump(bt, nil, data)
			viaEntry := runDriver(t, driver, "--input", path, "--rule-address", fmt.Sprintf("%d", rules[0].address), "--rule-lr")
			assert.Equal(t, expected, viaEntry)
		})
	}
}

// TestGenZigDifferential/corpus is the hook for an external grammar and
// corpus, driven by environment variables so it can run against a
// consumer's checkout (see zig/scripts/diff-circ.sh):
//
//	LANGLANG_DIFF_GRAMMAR  path to the .peg (required; the test skips otherwise)
//	LANGLANG_DIFF_FLAGS    space-separated langlang flags: -disable-capture-spaces,
//	                       -disable-captures, -disable-charsets, -disable-spaces,
//	                       -disable-builtins, -disable-inline, -disable-inline-defs=false
//	LANGLANG_DIFF_CORPUS   glob of input files (required)
//	LANGLANG_DIFF_LABELS   "label=message;label=message" bound on both sides
func TestGenZigDifferentialCorpus(t *testing.T) {
	grammar := os.Getenv("LANGLANG_DIFF_GRAMMAR")
	corpus := os.Getenv("LANGLANG_DIFF_CORPUS")
	if grammar == "" || corpus == "" {
		t.Skip("set LANGLANG_DIFF_GRAMMAR and LANGLANG_DIFF_CORPUS to run the external corpus")
	}
	requireZig(t)
	cfg := zigTestConfig()
	for _, flag := range strings.Fields(os.Getenv("LANGLANG_DIFF_FLAGS")) {
		switch flag {
		case "-disable-capture-spaces":
			cfg.SetBool("grammar.capture_spaces", false)
		case "-disable-captures":
			cfg.SetBool("grammar.captures", false)
		case "-disable-charsets":
			cfg.SetBool("grammar.add_charsets", false)
		case "-disable-spaces":
			cfg.SetBool("grammar.handle_spaces", false)
		case "-disable-builtins":
			cfg.SetBool("grammar.add_builtins", false)
		case "-disable-inline":
			cfg.SetBool("compiler.inline.enabled", false)
		case "-disable-inline-defs=false":
			cfg.SetBool("compiler.inline.emit.inlined", true)
		default:
			t.Fatalf("unsupported flag in LANGLANG_DIFF_FLAGS: %q", flag)
		}
	}
	labels := map[string]string{}
	for _, pair := range strings.Split(os.Getenv("LANGLANG_DIFF_LABELS"), ";") {
		if eq := strings.IndexByte(pair, '='); eq > 0 {
			labels[pair[:eq]] = pair[eq+1:]
		}
	}
	inputs, err := filepath.Glob(corpus)
	require.NoError(t, err)
	require.NotEmpty(t, inputs, "no inputs match %s", corpus)
	sort.Strings(inputs)

	g := zigGrammar{name: "corpus", path: grammar, setup: func(c *Config) { *c = *cfg }}
	db := NewDatabase(cfg, NewRelativeImportLoader())
	bt, err := QueryBytecode(db, grammar)
	require.NoError(t, err)
	driver := buildParserDriver(t, g)
	var msgArgs []string
	if len(labels) > 0 {
		msgArgs = []string{"--messages", messagesArg(labels)}
	}
	for _, path := range inputs {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			for _, mode := range modes {
				args := append(append([]string{"--input", path}, msgArgs...), mode.args...)
				zig := runDriver(t, driver, args...)
				expected := goDumpMode(bt, labels, data, mode.showFails)
				assert.Equal(t, expected, zig, "mode %s", mode.name)
			}
		})
	}
}

// TestGenZigTablesMatchVendoredGo checks that the Zig backend emits the
// same `code` bytes a consumer already vendors in a Go parser generated
// from the same grammar: set CIRC_PARSER_GO to circ-compiler's
// lib/parser/parser.go (its grammar is vendored as tests/circ/circ.peg).
func TestGenZigTablesMatchVendoredGo(t *testing.T) {
	path := os.Getenv("CIRC_PARSER_GO")
	if path == "" {
		t.Skip("set CIRC_PARSER_GO to the vendored parser.go to compare tables")
	}
	src, err := os.ReadFile(path)
	require.NoError(t, err)
	vendored := goByteLiteral(t, string(src), "code: []byte{")

	g := circGrammar()
	cfg := zigTestConfig()
	g.setup(cfg)
	db := NewDatabase(cfg, NewRelativeImportLoader())
	bt, err := QueryBytecode(db, g.path)
	require.NoError(t, err)
	assert.Equal(t, vendored, bt.code, "the Go and Zig backends must share one Encode() output")
}

// goByteLiteral parses the decimal bytes of a `field: []byte{ ... }`
// literal out of Go source.
func goByteLiteral(t *testing.T, src, marker string) []byte {
	t.Helper()
	start := strings.Index(src, marker)
	require.GreaterOrEqual(t, start, 0, "marker %q not found", marker)
	start += len(marker)
	end := strings.Index(src[start:], "}")
	require.GreaterOrEqual(t, end, 0)
	var out []byte
	for _, tok := range strings.Split(src[start:start+end], ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		var b int
		_, err := fmt.Sscanf(tok, "%d", &b)
		require.NoError(t, err, "token %q", tok)
		out = append(out, byte(b))
	}
	return out
}

// grammarInputs lists the corpus for a grammar: tests/<name>/inputs/*
// when present, plus the langlang grammars themselves for the langlang
// grammar and the smallest checked-in JSON document for json.
func grammarInputs(t *testing.T, g zigGrammar) []string {
	t.Helper()
	var paths []string
	dir := filepath.Join(filepath.Dir(g.path), "inputs")
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				paths = append(paths, filepath.Join(dir, e.Name()))
			}
		}
	}
	switch g.name {
	case "langlang":
		matches, err := filepath.Glob("../grammars/*.peg")
		require.NoError(t, err)
		paths = append(paths, matches...)
	case "json":
		paths = append(paths, "tests/json/input_30kb.json")
	}
	sort.Strings(paths)
	return paths
}
