package langlang

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// zigGrammar is one grammar the Zig backend must handle, with the same
// configuration its `//go:generate` line gives the Go backend.
type zigGrammar struct {
	name  string
	path  string
	setup func(cfg *Config)
}

func disableCaptureSpaces(cfg *Config) { cfg.SetBool("grammar.capture_spaces", false) }
func emitInlinedDefs(cfg *Config)      { cfg.SetBool("compiler.inline.emit.inlined", true) }

var zigGrammars = []zigGrammar{
	{name: "basic", path: "tests/basic/basic.peg", setup: emitInlinedDefs},
	{name: "charsets", path: "tests/charsets/charsets.peg", setup: emitInlinedDefs},
	{name: "recovery", path: "tests/recovery/recovery.peg", setup: disableCaptureSpaces},
	{name: "arithmetic", path: "tests/arithmetic/arithmetic.peg", setup: disableCaptureSpaces},
	{name: "arithmetic_leftrec", path: "tests/arithmetic_leftrec/arithmetic.peg", setup: disableCaptureSpaces},
	{name: "import", path: "tests/import/import_gr_expr.peg"},
	{name: "json", path: "../grammars/json.peg"},
	{name: "langlang", path: "../grammars/langlang.peg"},
}

// zigTestConfig mirrors the defaults cmd/langlang sets before a grammar
// specific setup runs.
func zigTestConfig() *Config {
	cfg := NewConfig()
	cfg.SetBool("grammar.add_builtins", true)
	cfg.SetBool("grammar.add_charsets", true)
	cfg.SetBool("grammar.captures", true)
	cfg.SetBool("grammar.capture_spaces", true)
	cfg.SetBool("grammar.handle_spaces", true)
	cfg.SetBool("compiler.inline.enabled", true)
	cfg.SetBool("compiler.inline.emit.inlined", false)
	cfg.SetBool("vm.show_fails", true)
	cfg.SetBool("vm.debug.source_map", false)
	return cfg
}

func compileZigGrammar(t *testing.T, g zigGrammar, opt GenZigOptions) string {
	t.Helper()
	cfg := zigTestConfig()
	if g.setup != nil {
		g.setup(cfg)
	}
	db := NewDatabase(cfg, NewRelativeImportLoader())
	program, err := QueryProgram(db, g.path)
	require.NoError(t, err, "compiling %s", g.path)
	opt.SourceFile = g.path
	out, err := GenZigEval(program, cfg, opt)
	require.NoError(t, err, "generating zig for %s", g.path)
	return out
}

// zigTail returns everything after the pasted runtime: the part of the
// output that depends on the grammar rather than on the runtime source.
func zigTail(output string) string {
	const marker = "// ---- END langlang runtime ----\n"
	idx := strings.Index(output, marker)
	if idx < 0 {
		return output
	}
	return output[idx+len(marker):]
}

// requireZig skips the test when no Zig toolchain is on PATH, unless
// LANGLANG_REQUIRE_ZIG is set (the CI job that installs Zig sets it so a
// missing toolchain fails loudly instead of silently skipping).
func requireZig(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("zig")
	if err != nil {
		if os.Getenv("LANGLANG_REQUIRE_ZIG") != "" {
			t.Fatalf("zig not found on PATH and LANGLANG_REQUIRE_ZIG is set")
		}
		t.Skip("zig not found on PATH; set LANGLANG_REQUIRE_ZIG=1 to make this a failure")
	}
	return path
}

func runZig(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("zig", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "zig %s failed in %s:\n%s", strings.Join(args, " "), dir, out)
}

func TestZigQuote(t *testing.T) {
	assert.Equal(t, `""`, zigQuote(""))
	assert.Equal(t, `"Program"`, zigQuote("Program"))
	assert.Equal(t, `"a\"b\\c"`, zigQuote(`a"b\c`))
	assert.Equal(t, `"tab\tnl\ncr\r"`, zigQuote("tab\tnl\ncr\r"))
	assert.Equal(t, `"\x00\x7f\xc3\xa9"`, zigQuote("\x00\x7fé"))
}

func TestZigIdent(t *testing.T) {
	assert.Equal(t, "Program", zigIdent("Program"))
	assert.Equal(t, "busclose", zigIdent("busclose"))
	assert.Equal(t, "u8", zigIdent("u8"), "primitive type names are plain identifiers to zig fmt")
	assert.Equal(t, `@"error"`, zigIdent("error"))
	assert.Equal(t, `@"while"`, zigIdent("while"))
	assert.Equal(t, `@"a-b"`, zigIdent("a-b"))
	assert.Equal(t, `@"_"`, zigIdent("_"))
	assert.Equal(t, `@"1st"`, zigIdent("1st"))
}

func TestZigRuntimeDeclaresABI(t *testing.T) {
	abi, err := zigRuntimeABI(ZigRuntimeSource())
	require.NoError(t, err)
	assert.Equal(t, "1", abi)
	_, err = zigRuntimeABI([]byte("nothing here"))
	assert.Error(t, err)
}

func TestZigTableLimits(t *testing.T) {
	assert.NoError(t, checkZigTableLimits(&Bytecode{code: make([]byte, 0xFFFF), strs: []string{""}}))
	assert.Error(t, checkZigTableLimits(&Bytecode{code: make([]byte, 0x10000)}))
	assert.Error(t, checkZigTableLimits(&Bytecode{strs: make([]string, 0x10000)}))
	assert.Error(t, checkZigTableLimits(&Bytecode{sets: make([]charset, 0x10000)}))
}

// TestGenZigEmit needs no Zig toolchain: it pins the grammar-dependent
// tail of the output for the recovery grammar.  Regenerate the golden with
// LANGLANG_UPDATE_GOLDENS=1 and review the diff.
func TestGenZigEmit(t *testing.T) {
	out := compileZigGrammar(t, zigGrammars[2], GenZigOptions{})

	require.True(t, strings.HasPrefix(out, "// Code generated by langlang ("), "header")
	assert.Contains(t, out, "// Runtime: langlang_runtime.zig abi=1 sha256=")
	assert.Equal(t, 1, strings.Count(out, "const std = @import(\"std\");"), "exactly one std import")
	assert.Contains(t, out, "pub const runtime = struct {")
	assert.Contains(t, out, "pub const entry_rule: Rule = .P;")
	assert.NotContains(t, out, "pub const Tree = runtime.Tree", "re-exports would be ambiguous inside the pasted namespace")

	tail := zigTail(out)
	golden := filepath.Join("testdata", "zig", "recovery.tail.zig.golden")
	if os.Getenv("LANGLANG_UPDATE_GOLDENS") != "" {
		require.NoError(t, os.MkdirAll(filepath.Dir(golden), 0755))
		require.NoError(t, os.WriteFile(golden, []byte(tail), 0644))
	}
	expected, err := os.ReadFile(golden)
	require.NoError(t, err, "missing golden; run with LANGLANG_UPDATE_GOLDENS=1")
	assert.Equal(t, string(expected), tail)
}

func TestGenZigEmitRuntimeImport(t *testing.T) {
	out := compileZigGrammar(t, zigGrammars[2], GenZigOptions{RuntimeImport: "langlang_runtime.zig"})
	assert.Contains(t, out, "pub const runtime = @import(\"langlang_runtime.zig\");")
	assert.NotContains(t, out, "pub const runtime = struct {")
	assert.Equal(t, zigTail(compileZigGrammar(t, zigGrammars[2], GenZigOptions{})), zigTail(out), "the tail does not depend on the runtime mode")
}

// TestGenZigCompiles generates every test grammar and checks that the
// output is zig-fmt clean, passes its own `verifyTables` test, and builds
// for wasm32-freestanding.  The subtests run sequentially on purpose:
// concurrent `zig` processes racing to populate a cold global cache have
// produced spurious failures.
func TestGenZigCompiles(t *testing.T) {
	requireZig(t)
	for _, g := range zigGrammars {
		g := g
		t.Run(g.name, func(t *testing.T) {
			dir := t.TempDir()
			out := compileZigGrammar(t, g, GenZigOptions{})
			file := filepath.Join(dir, "parser.zig")
			require.NoError(t, os.WriteFile(file, []byte(out), 0644))
			runZig(t, dir, "fmt", "--check", "parser.zig")
			runZig(t, dir, "test", "parser.zig")
			runZig(t, dir, "build-obj", "-target", "wasm32-freestanding", "-O", "ReleaseSmall", "parser.zig")
		})
	}
	t.Run("source-map", func(t *testing.T) {
		dir := t.TempDir()
		g := zigGrammars[2]
		cfg := zigTestConfig()
		g.setup(cfg)
		cfg.SetBool("vm.debug.source_map", true)
		db := NewDatabase(cfg, NewRelativeImportLoader())
		program, err := QueryProgram(db, g.path)
		require.NoError(t, err)
		out, err := GenZigEval(program, cfg, GenZigOptions{SourceFile: g.path})
		require.NoError(t, err)
		require.Contains(t, out, ".srcm = &runtime.SourceMap{", "the source map must be emitted when the grammar was compiled with vm.debug.source_map")
		require.Contains(t, out, ".files = &[_][]const u8{")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "parser.zig"), []byte(out), 0644))
		runZig(t, dir, "fmt", "--check", "parser.zig")
		runZig(t, dir, "test", "parser.zig")
	})
	t.Run("runtime-import", func(t *testing.T) {
		dir := t.TempDir()
		out := compileZigGrammar(t, zigGrammars[2], GenZigOptions{RuntimeImport: "langlang_runtime.zig"})
		require.NoError(t, os.WriteFile(filepath.Join(dir, "parser.zig"), []byte(out), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "langlang_runtime.zig"), ZigRuntimeSource(), 0644))
		runZig(t, dir, "fmt", "--check", "parser.zig")
		runZig(t, dir, "test", "parser.zig")
	})
	t.Run("runtime-unit-tests", func(t *testing.T) {
		runZig(t, "zig", "fmt", "--check", "langlang_runtime.zig", "langlang_runtime_test.zig")
		runZig(t, "zig", "test", "langlang_runtime_test.zig")
	})
}
