# Zig output language

`langlang -output-language zig` emits one self-contained Zig file: the
runtime in this directory pasted as a nested namespace, followed by the
grammar's bytecode as `comptime` tables and a `Rule` enum of entry
addresses. It compiles with Zig 0.15.1 for native targets and for
`wasm32-freestanding` with no libc, and reproduces the Go virtual machine's
trees, byte spans, error-recovery nodes and error messages byte for byte.

```sh
langlang -grammar my.peg -output-language zig -output-path parser.zig
zig test parser.zig          # runs the emitted table check
```

## Files

| File | Role |
| --- | --- |
| `langlang_runtime.zig` | The runtime: opcode ABI, `verifyTables`, `Tree`, `Stack`, `Machine` (the VM over runtime tables), `Interpreter` (the comptime wrapper generated parsers use). Pasted into every generated file. |
| `langlang_runtime_test.zig` | Standalone unit tests: `cd go/zig && zig test langlang_runtime_test.zig`. |
| `driver_common.zig`, `testdriver.zig`, `vmdriver.zig` | Drivers for the Go-vs-Zig differential harness (`go test -run TestGenZig ./...`), not part of generated output. |
| `PLAN.md` | The plan this backend was built from, with the decisions and traps. |

## Using a generated parser

```zig
const parser = @import("parser.zig");

var p = try parser.Parser.init(gpa);          // any std.mem.Allocator
defer p.deinit();
p.setLabelMessages(&.{ .{ .label = "busclose", .message = "expected ')'" } });

const tree = p.parse(source) catch |err| switch (err) {
    error.ParseFailed => {
        const e = p.lastError();                // byte cursors: e.start, e.end
        const text = try e.messageAlloc(gpa, &parser.bytecode, p.messages());
        defer gpa.free(text);
        ...
    },
    else => return err,
};
const root = tree.root() orelse return error.EmptyInput;
switch (tree.typ(root)) { .node, .err, .sequence, .string => ... }
_ = tree.name(root);            // rule name or error label
_ = tree.range(root);           // byte offsets, end exclusive
_ = tree.childrenLen(root);     // 1 for node/err, N for sequence, 0 for string
_ = tree.childAt(root, 0);
```

- Nothing is re-exported from the pasted runtime — spell `parser.runtime.Tree`,
  `parser.runtime.NodeType`, and so on (a top-level `pub const Tree =
  runtime.Tree` would make every `Tree` inside the runtime an ambiguous
  reference).
- The tree is owned by the parser and reset by the next match; `Tree.copy`
  retains it.
- `matchRule(input, .Expr)` starts at a specific rule. Left-recursive rules
  (listed in `parser.left_recursive_rules`) are entered through an LR frame,
  which is the one place the Zig runtime deliberately differs from the Go VM
  (whose `MatchRule` cannot enter them).
- `setShowFails(true)` turns on expected-hint tracking, so failures render as
  `Expected 'a', 'b-c' but got 'x'` instead of `Unexpected 'x'`.

## WebAssembly

The runtime never names an allocator, touches the filesystem, or prints; a
`wasm32-freestanding` host supplies `std.heap.wasm_allocator` (or an arena
over it) from its own entry file:

```zig
const parser = @import("parser.zig");
var p: ?parser.Parser = null;
export fn parse(ptr: [*]const u8, len: usize) i32 {
    if (p == null) p = parser.Parser.init(std.heap.wasm_allocator) catch return -1;
    _ = p.?.parse(ptr[0..len]) catch return 1;
    return 0;
}
```

Build with `zig build-exe entry.zig -target wasm32-freestanding -O ReleaseSmall -fno-entry --export=parse`.

## Runtime and generator contract

- **Bytecode ABI.** Opcode numbering, operand sizes and little-endian
  u16/u32 operands are the Go VM's and are frozen; `Encode()` in `go/` is the
  single encoder for every backend. Programs and string tables are limited to
  65535 entries by the u16 operands; the Zig generator reports that instead
  of truncating.
- **`abi_version`.** Bumped whenever the shape of `Bytecode` or the meaning
  of an opcode changes. The generator copies it into the emitted tables and
  `Interpreter` refuses a mismatch at compile time; the generated header also
  carries the runtime's sha256, which is the reliable skew signal (the commit
  hash is `unknown` under `go run`).
- **Paste rules.** The runtime is pasted verbatim, indented, with its single
  `const std = @import("std");` line removed. It must not declare or reference
  a name the generated file declares at top level (`bytecode`, `Rule`,
  `entry_rule`, `left_recursive_rules`, `Parser`), must not add a second
  top-level import or `std` alias, and must not use `std.fs`, `std.process`,
  `std.debug` or `std.heap.*`. `TestGenZigCompiles` enforces all of it.
- **`zig fmt`.** The emitter writes formatter-clean output (column-aligned
  array rows, keyword-only `@"..."` quoting, no inner spaces in one-element
  lists) so generation never needs a Zig toolchain; `zig fmt --check` runs as
  a test.
- **Go quirks are copied, not fixed.** The predicate flag restored from the
  popped backtrack frame, `fail_twice` not truncating the arena, `doFailLR`'s
  `result > 0` test, and the multiple-assignment returns that zero `pc` and
  `cursor` on non-resuming LR paths all behave exactly as in `go/vm.go`, so
  the differential harness can stay byte-exact. Fix them in both runtimes
  together.

## Import mode

`-zig-runtime-import=langlang_runtime.zig` emits `pub const runtime =
@import("langlang_runtime.zig");` instead of pasting, and
`-zig-emit-runtime <path>` writes the runtime file, for packages that host
several parsers and vendor the runtime once.

## Testing

```sh
cd go && go test ./... -run TestGenZig            # skips the Zig parts when zig is not on PATH
cd go && LANGLANG_REQUIRE_ZIG=1 go test ./...     # fails instead of skipping (CI with Zig)
cd go/zig && zig test langlang_runtime_test.zig
LANGLANG_UPDATE_GOLDENS=1 go test -run TestGenZigEmit ./   # refresh testdata/zig/*.golden
```

`TestGenZigDifferential` is the acceptance test: for the VM test table
(four compiler configurations, plus non-ASCII edge cases) and every grammar
under `go/tests`, the Go VM and the Zig runtime must print the same
canonical dump — node ids, byte spans, cursor and error message included —
with expected-hint tracking both off and on.
