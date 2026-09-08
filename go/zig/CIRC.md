# Hand-over: switching circ-compiler to the generated Zig parser

What circ-compiler has to do to replace its Go/cgo parser with
`langlang -output-language zig`, and what this repository has already
proven about that switch. The work itself lands in circ's libcirc
initiative (its `DOCS/PLANS_PROMPT.md`, Phase 1).

## Proven here

- `tests/circ/circ.peg` is circ's `lib/grammar/proto-circ.peg`, vendored.
  `TestGenZigCompiles/circ` generates, formats, tests and builds it for
  wasm32; `TestGenZigCompiles/circ-shape` runs `testdata/circ_shape_test.zig`
  against the generated file; `TestGenZigCompiles/wasm-smoke` links it into a
  freestanding wasm32 module (~18 KB, ReleaseSmall).
- `TestGenZigDifferential/grammars/circ` replays `tests/circ/inputs/*` (circ's
  inline analyze/translate test inputs plus the recovery quirks) through the
  Go VM and the generated Zig parser, with expected-hint tracking off and on.
- `zig/scripts/diff-circ.sh` does the same over every
  `tests/fixtures/circuits/*.circ` in a circ checkout, with the label messages
  circ's `translate.zig` binds, and checks that the Zig backend's `code` table
  is byte-identical to the `parser.go` circ vendors (`TestGenZigTablesMatchVendoredGo`).

Run before cutting over:

```sh
CIRC_ROOT=/path/to/circ-compiler go/zig/scripts/diff-circ.sh -v
```

## Generate

```sh
langlang -grammar lib/grammar/proto-circ.peg -disable-capture-spaces \
         -output-language zig -output-path lib/parser/parser.zig
```

Install `langlang` from this fork (branch `zig-parser-gen` or its tag);
upstream `go/v0.0.12` rejects `-output-language zig`. The generated header's
`Runtime: … abi=N sha256=…` line is the skew signal between the fork's
runtime and the vendored file.

## build.zig

- `parser:gen` (`build.zig:19-36`) becomes the command above.
- `translate_mod` is rooted at `lib/syntax/translate.zig`, and Zig rejects
  `@import("../parser/parser.zig")` across module roots, so add
  `const parser_mod = b.createModule(.{ .root_source_file = b.path("lib/parser/parser.zig"), .target = target, .optimize = optimize });`
  and `translate_mod.addImport("parser", parser_mod);` (plus a wasm-target twin
  when translate is compiled into a wasm module). The 18
  `addImport("translate", translate_mod)` sites are untouched.
- Delete `fn linkParserArchive` (`:9-12`), the `parser:archive` step and the
  GOOS/GOARCH/CC block (`:38-77`, including both `@panic` arms), all 27
  `linkParserArchive(b, …)` calls, and the paired `.linkLibC()` calls
  **except** on artifacts whose module graph includes `tests/helpers/golden.zig`
  (it `@cImport`s `setenv`/`unsetenv`, `build.zig:79-88`; `translate_tests`
  imports it) — audit per artifact. Remove the
  `addIncludePath(".")`/`addIncludePath("./lib")` pairs that only served
  `#include "parser/parser.h"`.
- Delete `lib/parser/{go.mod,parser.go,parser.a,parser.h,shim/}`,
  `lib/syntax/CParser.zig`, the dead `lib/syntax/nodes/declaration.zig`, and
  the `lib/parser/parser.a` / `parser.h` `.gitignore` lines. CLAUDE.md and the
  README drop Go 1.21+ and `parser:archive`; langlang stays a
  grammar-change-only tool.

## translate.zig

Every walk site goes through six wrappers (`translate.zig:33-96`), so the
diff is mechanical:

| Today | After |
| --- | --- |
| `const C_Parser = @import("CParser.zig").C_Parser;` | `const parser = @import("parser");` |
| `NodeType_String/Sequence/Node/Error: u8 = 0..3` | keep the values, typed `parser.runtime.NodeType` (`.string/.sequence/.node/.err`) |
| `Range { start: c_int, end: c_int }` | `parser.runtime.Range` (usize; drops the `@max(…, 0)`/`@intCast` at `:65-66`) |
| `handle: @TypeOf(C_Parser.ParserNew())` | `tree: *const parser.runtime.Tree` |
| `nodeType` → `TreeType` | `ctx.tree.typ(id)` |
| `nodeName` → `TreeName` | `ctx.tree.name(id)` |
| `nodeRange` → `TreeSpanStart/End` | `ctx.tree.range(id)` |
| `childAt` → `TreeChildrenAt` | `ctx.tree.childAt(parent, i) orelse error.InvalidChildIndex` |
| `childCount` → `TreeChildrenLen` | `ctx.tree.childrenLen(parent)` |
| `firstChild` → `TreeChild` | `ctx.tree.child(parent) orelse error.InvalidChild` |
| `:702` `TreeRoot` | `const root = ctx.tree.root() orelse return error.ParsingFailed;` |
| `translate(allocator, handle, file_id)` (`:738-751`) | delete — no callers, and the tree carries its input |

`parseSourceCapturing` (`:769-807`) becomes:

```zig
var p = try parser.Parser.init(allocator);
defer p.deinit();
p.setLabelMessages(&circ_label_messages);
const tree = p.parse(source) catch |e| switch (e) {
    error.ParseFailed => {
        if (failure_out) |out| {
            const le = p.lastError();
            const start_lc = offsetToLineCol(source, le.start);
            const end_lc = offsetToLineCol(source, @intCast(@max(le.end, 0)));
            out.* = .{
                .start_line = start_lc.line, .start_col = start_lc.col,
                .end_line = end_lc.line, .end_col = end_lc.col,
                .message = try le.messageAlloc(allocator, &parser.bytecode, p.messages()),
            };
        }
        return error.ParsingFailed;
    },
    else => return e,
};
var ctx = TranslationContext{ .allocator = allocator, .tree = tree, .source = source, .file_id = file_id };
return translateTree(&ctx);
```

The two divergent label→message tables (`shim.go:49-55` and
`translate.zig:641-648`, which differ for `trailing`) collapse into one
`circ_label_messages: []const parser.runtime.LabelMessage` using translate's
wording — the only text users see today, since all five labels recover.
`ast.File` never references tree memory (identifier text is sliced from
`ctx.source`), so `p.deinit()` on return is safe.

## Acceptance in circ

- The 27 `tests/fixtures/expected-ast` goldens byte-identical (they encode
  byte-based spans from `offsetToLineCol`, which the Zig tree reproduces).
- `tests/syntax/translate_test.zig` (empty input → `error.ParsingFailed`)
  and the analyze recovery tests unchanged.
- `zig build test-all` green on a machine without Go.
- `zig build parser:gen` on the unchanged grammar reproduces
  `lib/parser/parser.zig` byte-identically.
- A new errors dump or translate test pinning `ErrorMark` spans and
  messages for the recovery inputs (`tests/circ/inputs` here lists them).

## Behaviour that carries over unchanged

Byte cursors only (circ derives line/col itself); `root() == null` on empty
input; `childrenLen == 1` for `node`/`err` with a child; names compared by
bytes; the Go VM's quirks (predicate restore, `fail_twice` not truncating,
the LR multiple-assignment zeroing) reproduced exactly, so recovery marks
and error positions do not move.
