// langlang runtime for generated Zig parsers.
//
// This file is pasted verbatim into every parser produced by
// `langlang -output-language zig` (inside `pub const runtime = struct { ... }`),
// so it must stay self-contained: exactly one top-level `@import("std")`, no
// other imports, no std.fs / std.process / std.debug / std.heap references,
// and no allocator named anywhere - callers inject one through `Parser.init`.
// It compiles for native targets and for wasm32-freestanding without libc.
//
// It is a port of the Go virtual machine (go/vm.go, vm_stack.go, vm_charset.go,
// tree.go). The bytecode ABI - opcode numbering, operand sizes, little-endian
// u16/u32 operands - is frozen and shared with the Go backend.
const std = @import("std");

/// Bumped whenever the shape of `Bytecode` or the meaning of an opcode
/// changes. The generator copies it into the tables it emits and `Parser`
/// refuses mismatched tables at compile time.
pub const abi_version: u32 = 1;

/// The langlang release this runtime was ported from.
pub const langlang_version = "go/v0.0.12";

// ---------------------------------------------------------------------------
// Bytecode
// ---------------------------------------------------------------------------

/// Opcode numbering is the Bytecode ABI (go/vm.go, the `opHalt = iota` block).
pub const Op = enum(u8) {
    halt = 0,
    any,
    char,
    range,
    fail,
    fail_twice,
    choice,
    choice_pred,
    cap_commit,
    cap_partial_commit,
    cap_back_commit,
    call,
    cap_return,
    jump,
    throw,
    cap_begin,
    cap_end,
    set,
    span,
    cap_term,
    cap_non_term,
    commit,
    back_commit,
    partial_commit,
    @"return",
    cap_term_begin_offset,
    cap_non_term_begin_offset,
    cap_end_offset,
    char32,
    range32,
    call_lr,
    return_lr,
    cap_return_lr,
};

pub const op_count: u8 = 33;

/// Instruction sizes in bytes, opcode included (go/vm.go `op*SizeInBytes`).
/// `call` carries a trailing precedence byte even when it is not left
/// recursive, so plain calls and `call_lr` are both 4 bytes.
pub fn opSize(op: Op) u8 {
    return switch (op) {
        .halt, .any, .fail, .fail_twice, .cap_return, .cap_end, .@"return", .cap_term_begin_offset, .cap_end_offset, .return_lr, .cap_return_lr => 1,
        .char, .set, .span, .choice, .choice_pred, .cap_commit, .cap_partial_commit, .cap_back_commit, .jump, .throw, .cap_begin, .cap_term, .commit, .back_commit, .partial_commit, .cap_non_term_begin_offset => 3,
        .call, .call_lr => 4,
        .char32, .range, .cap_non_term => 5,
        .range32 => 9,
    };
}

/// A 256-bit set over bytes (go/vm_charset.go). Charsets only ever hold code
/// points below 0x80, so `set`/`span` are pure single-byte tests.
pub const Charset = struct {
    bits: [32]u8,

    pub inline fn has(self: Charset, b: u8) bool {
        return (self.bits[b >> 3] & (@as(u8, 1) << @intCast(b & 7))) != 0;
    }
};

/// A precomputed hint for error messages: a single code point when `b == 0`,
/// otherwise the inclusive range `a-b`.
pub const Expected = struct {
    a: u21,
    b: u21 = 0,
};

/// Grammar source map, emitted only with `--grammar-source-map`.
pub const SourceMap = struct {
    data: []const u8,
    files: []const []const u8,
};

/// Everything the generator emits for one grammar. `rxps` is indexed by
/// string id and holds the recovery production's address, or -1 when the
/// string is not a recovery label (it subsumes the Go `rxps` map and `rxbs`
/// bitset). `strs[0]` is always the empty "no name" sentinel.
pub const Bytecode = struct {
    abi: u32,
    code: []const u8,
    strs: []const []const u8,
    sets: []const Charset,
    sexp: []const []const Expected,
    rxps: []const i32,
    srcm: ?*const SourceMap = null,

    pub fn isRecoveryLabel(self: Bytecode, id: usize) bool {
        return id < self.rxps.len and self.rxps[id] >= 0;
    }
};

pub const VerifyError = error{InvalidBytecode};

fn readU16(code: []const u8, at: usize) u16 {
    return std.mem.readInt(u16, code[at..][0..2], .little);
}

fn readU32(code: []const u8, at: usize) u32 {
    return std.mem.readInt(u32, code[at..][0..4], .little);
}

/// Decodes the whole instruction stream once and checks every operand
/// against the tables. The generator emits a `test` that calls this, so a
/// generated file that passes `zig test` cannot make the VM index out of
/// bounds.
pub fn verifyTables(bc: Bytecode) VerifyError!void {
    if (bc.abi != abi_version) return error.InvalidBytecode;
    if (bc.strs.len == 0 or bc.strs[0].len != 0) return error.InvalidBytecode;
    if (bc.rxps.len != bc.strs.len) return error.InvalidBytecode;
    if (bc.sexp.len != bc.sets.len) return error.InvalidBytecode;
    if (bc.strs.len > 0xFFFF or bc.sets.len > 0xFFFF or bc.code.len > 0xFFFF) return error.InvalidBytecode;
    for (bc.rxps) |addr| {
        if (addr >= 0 and @as(usize, @intCast(addr)) >= bc.code.len) return error.InvalidBytecode;
    }
    // The compiler always emits `call <first rule>; halt` as the prologue.
    if (bc.code.len < 5 or bc.code[0] != @intFromEnum(Op.call) and bc.code[0] != @intFromEnum(Op.call_lr)) return error.InvalidBytecode;
    if (bc.code[4] != @intFromEnum(Op.halt)) return error.InvalidBytecode;

    var pc: usize = 0;
    while (pc < bc.code.len) {
        const raw = bc.code[pc];
        if (raw >= op_count) return error.InvalidBytecode;
        const op: Op = @enumFromInt(raw);
        const size = opSize(op);
        if (pc + size > bc.code.len) return error.InvalidBytecode;
        switch (op) {
            .choice, .choice_pred, .cap_commit, .cap_partial_commit, .cap_back_commit, .jump, .commit, .back_commit, .partial_commit, .call, .call_lr => {
                if (readU16(bc.code, pc + 1) >= bc.code.len) return error.InvalidBytecode;
            },
            .set, .span => {
                if (readU16(bc.code, pc + 1) >= bc.sets.len) return error.InvalidBytecode;
            },
            .throw, .cap_begin, .cap_non_term_begin_offset => {
                if (readU16(bc.code, pc + 1) >= bc.strs.len) return error.InvalidBytecode;
            },
            .cap_non_term => {
                if (readU16(bc.code, pc + 1) >= bc.strs.len) return error.InvalidBytecode;
            },
            else => {},
        }
        pc += size;
    }
}

// ---------------------------------------------------------------------------
// Tree
// ---------------------------------------------------------------------------

pub const NodeId = u32;

/// Numbering matches go/api.go (`NodeType_String = iota`); consumers may
/// hard-code these values.
pub const NodeType = enum(u8) {
    string = 0,
    sequence = 1,
    node = 2,
    err = 3,
};

/// Struct-of-arrays layout mirroring go/tree.go. `child_id` is the single
/// child for `node`/`err`, an index into `child_ranges` for `sequence`, and
/// -1 when absent. `message_id` is the label whose message an `err` node
/// carries (-1 otherwise).
pub const Node = struct {
    typ: NodeType,
    start: u32,
    end: u32,
    name_id: i32 = -1,
    child_id: i32 = -1,
    message_id: i32 = -1,
};

pub const ChildRange = struct {
    start: u32,
    end: u32,
};

/// Byte offsets into the input; `end` is exclusive.
pub const Range = struct {
    start: usize,
    end: usize,
};

pub const LabelMessage = struct {
    label: []const u8,
    message: []const u8,
};

pub const Tree = struct {
    nodes: std.ArrayList(Node) = .empty,
    children: std.ArrayList(NodeId) = .empty,
    child_ranges: std.ArrayList(ChildRange) = .empty,
    /// The grammar's string table (names and labels).
    strs: []const []const u8 = &.{},
    /// Per-label messages installed with `Parser.setLabelMessages`.
    messages: []const ?[]const u8 = &.{},
    input: []const u8 = &.{},
    root_id: ?NodeId = null,

    pub fn deinit(t: *Tree, gpa: std.mem.Allocator) void {
        t.nodes.deinit(gpa);
        t.children.deinit(gpa);
        t.child_ranges.deinit(gpa);
        t.* = .{};
    }

    /// Drops every node but keeps the storage (the Go tree is reused between
    /// matches the same way).
    pub fn reset(t: *Tree) void {
        t.nodes.clearRetainingCapacity();
        t.children.clearRetainingCapacity();
        t.child_ranges.clearRetainingCapacity();
        t.root_id = null;
    }

    /// The top-level node, or null when the match never produced one
    /// (an empty input, for instance).
    pub fn root(t: *const Tree) ?NodeId {
        return t.root_id;
    }

    pub fn len(t: *const Tree) usize {
        return t.nodes.items.len;
    }

    pub fn typ(t: *const Tree, id: NodeId) NodeType {
        return t.nodes.items[id].typ;
    }

    pub fn name(t: *const Tree, id: NodeId) []const u8 {
        const n = t.nodes.items[id];
        if (n.name_id < 0) return "";
        return t.strs[@intCast(n.name_id)];
    }

    /// For `err` nodes: the message bound to the label, else the label name.
    pub fn message(t: *const Tree, id: NodeId) []const u8 {
        const n = t.nodes.items[id];
        if (n.message_id < 0) return "";
        const mid: usize = @intCast(n.message_id);
        if (mid < t.messages.len) {
            if (t.messages[mid]) |m| return m;
        }
        return if (mid < t.strs.len) t.strs[mid] else "";
    }

    pub fn range(t: *const Tree, id: NodeId) Range {
        const n = t.nodes.items[id];
        return .{ .start = n.start, .end = n.end };
    }

    pub fn slice(t: *const Tree, id: NodeId) []const u8 {
        const n = t.nodes.items[id];
        return t.input[n.start..n.end];
    }

    /// The single child of a `node` or `err` node.
    pub fn child(t: *const Tree, id: NodeId) ?NodeId {
        const n = t.nodes.items[id];
        return switch (n.typ) {
            .node, .err => if (n.child_id < 0) null else @as(NodeId, @intCast(n.child_id)),
            else => null,
        };
    }

    pub fn sequenceChildren(t: *const Tree, id: NodeId) []const NodeId {
        const n = t.nodes.items[id];
        if (n.typ != .sequence or n.child_id < 0) return &.{};
        const r = t.child_ranges.items[@intCast(n.child_id)];
        return t.children.items[r.start..r.end];
    }

    pub fn childrenLen(t: *const Tree, id: NodeId) usize {
        return switch (t.nodes.items[id].typ) {
            .string => 0,
            .sequence => t.sequenceChildren(id).len,
            .node, .err => if (t.child(id) != null) @as(usize, 1) else 0,
        };
    }

    pub fn childAt(t: *const Tree, id: NodeId, i: usize) ?NodeId {
        return switch (t.nodes.items[id].typ) {
            .string => null,
            .sequence => blk: {
                const cs = t.sequenceChildren(id);
                break :blk if (i < cs.len) cs[i] else null;
            },
            .node, .err => if (i == 0) t.child(id) else null,
        };
    }

    pub fn copy(t: *const Tree, gpa: std.mem.Allocator) std.mem.Allocator.Error!Tree {
        var out: Tree = .{
            .strs = t.strs,
            .messages = t.messages,
            .input = t.input,
            .root_id = t.root_id,
        };
        errdefer out.deinit(gpa);
        try out.nodes.appendSlice(gpa, t.nodes.items);
        try out.children.appendSlice(gpa, t.children.items);
        try out.child_ranges.appendSlice(gpa, t.child_ranges.items);
        return out;
    }

    /// Canonical pre-order dump used by the Go-vs-Zig differential harness:
    /// `root=<id>` then one line per node, `<2*depth spaces>#<id> <type> <name or -> <start> <end>`,
    /// with ` msg=<message>` appended to `err` lines; `noroot` when there is
    /// no root.
    pub fn dump(t: *const Tree, w: *std.Io.Writer) std.Io.Writer.Error!void {
        const r = t.root_id orelse return w.writeAll("noroot\n");
        try w.print("root={d}\n", .{r});
        try t.dumpNode(w, r, 0);
    }

    fn dumpNode(t: *const Tree, w: *std.Io.Writer, id: NodeId, depth: usize) std.Io.Writer.Error!void {
        try w.splatByteAll(' ', depth * 2);
        const n = t.nodes.items[id];
        const nm = t.name(id);
        try w.print("#{d} {s} {s} {d} {d}", .{ id, @tagName(n.typ), if (nm.len == 0) "-" else nm, n.start, n.end });
        if (n.typ == .err) try w.print(" msg={s}", .{t.message(id)});
        try w.writeByte('\n');
        var i: usize = 0;
        const count = t.childrenLen(id);
        while (i < count) : (i += 1) {
            try t.dumpNode(w, childAtUnchecked(t, id, i), depth + 1);
        }
    }

    fn childAtUnchecked(t: *const Tree, id: NodeId, i: usize) NodeId {
        return t.childAt(id, i) orelse unreachable;
    }
};

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

/// Built at the failure site; the message text is produced on demand by
/// `writeMessage`, reproducing go/vm.go `mkErr`.
pub const ParseError = struct {
    /// String id of the label that failed, 0 for an unlabeled failure.
    label_id: u32 = 0,
    /// Cursor where the failure was reported.
    start: u32 = 0,
    /// Furthest failure position.
    end: u32 = 0,
    /// Bytecode address of the furthest failure.
    ffp_pc: u32 = 0,
    /// The code point at the failure, or null at end of input.
    unexpected: ?u21 = null,
    /// Hints collected while `show_fails` was on (at most 20).
    expected: []const Expected = &.{},

    pub fn label(e: ParseError, bc: *const Bytecode) []const u8 {
        return if (e.label_id < bc.strs.len) bc.strs[e.label_id] else "";
    }

    pub fn writeMessage(e: ParseError, w: *std.Io.Writer, bc: *const Bytecode, messages: []const ?[]const u8) std.Io.Writer.Error!void {
        if (e.label_id < messages.len) {
            if (messages[e.label_id]) |m| return w.writeAll(m);
        }
        if (e.label_id > 0) try w.print("[{s}] ", .{e.label(bc)});
        if (e.expected.len > 0) {
            try w.writeAll("Expected ");
            for (e.expected, 0..) |x, i| {
                try w.writeByte('\'');
                try writeRune(w, x.a);
                if (x.b != 0) {
                    try w.writeByte('-');
                    try writeRune(w, x.b);
                }
                try w.writeByte('\'');
                if (i + 1 < e.expected.len) try w.writeAll(", ");
            }
            try w.writeAll(" but got ");
        } else {
            try w.writeAll("Unexpected ");
        }
        if (e.unexpected) |r| {
            try w.writeByte('\'');
            try writeRune(w, r);
            try w.writeByte('\'');
        } else {
            try w.writeAll("EOF");
        }
    }

    pub fn messageAlloc(e: ParseError, gpa: std.mem.Allocator, bc: *const Bytecode, messages: []const ?[]const u8) std.mem.Allocator.Error![]u8 {
        var out: std.Io.Writer.Allocating = .init(gpa);
        errdefer out.deinit();
        e.writeMessage(&out.writer, bc, messages) catch return error.OutOfMemory;
        return out.toOwnedSlice();
    }
};

fn writeRune(w: *std.Io.Writer, r: u21) std.Io.Writer.Error!void {
    var buf: [4]u8 = undefined;
    const n = std.unicode.utf8Encode(r, &buf) catch {
        // Go writes the replacement character for an invalid rune.
        return w.writeAll("\u{FFFD}");
    };
    try w.writeAll(buf[0..n]);
}

/// Go's `(Tree, int, error)` triple: `tree` is present on stack exhaustion
/// and absent on a throw without a recovery production.
pub const MatchResult = struct {
    tree: ?*const Tree,
    cursor: u32,
    err: ?ParseError,
};

pub const Error = error{ ParseFailed, OutOfMemory, InvalidBytecode };

// ---------------------------------------------------------------------------
// Parser
// ---------------------------------------------------------------------------

/// Instantiated by the generated file as `runtime.Interpreter(bytecode, Rule, &left_recursive_rules)`.
/// Nothing in this runtime may declare or reference a name the generated
/// file declares at top level (`bytecode`, `Rule`, `entry_rule`,
/// `left_recursive_rules`, `Parser`): inside the pasted namespace such a
/// name would resolve to both declarations and Zig rejects the reference
/// as ambiguous. `RuleT` is an `enum(u16)` of entry addresses; `lr_rules`
/// lists the ones that are left recursive and need an LR frame when entered
/// directly.
pub fn Interpreter(comptime bc: Bytecode, comptime RuleT: type, comptime lr_rules: []const RuleT) type {
    comptime {
        if (bc.abi != abi_version) {
            @compileError(std.fmt.comptimePrint("langlang: generated tables carry abi {d} but this runtime is abi {d}; regenerate the parser", .{ bc.abi, abi_version }));
        }
    }
    return struct {
        const Self = @This();

        pub const RuleType = RuleT;
        pub const left_recursive = lr_rules;
        pub const tables = bc;
        pub const messages_len = bc.strs.len;

        gpa: std.mem.Allocator,
        show_fails: bool = false,
        messages: [bc.strs.len]?[]const u8 = [_]?[]const u8{null} ** bc.strs.len,
        tree_storage: Tree = .{ .strs = bc.strs },
        last_error: ParseError = .{},

        pub fn init(gpa: std.mem.Allocator) Self {
            return .{ .gpa = gpa };
        }

        pub fn deinit(self: *Self) void {
            self.tree_storage.deinit(self.gpa);
        }

        pub fn setShowFails(self: *Self, v: bool) void {
            self.show_fails = v;
        }

        /// Binds messages to labels by name; unknown labels are ignored, as
        /// the Go `CompileErrorLabels` does.
        pub fn setLabelMessages(self: *Self, msgs: []const LabelMessage) void {
            for (msgs) |m| {
                if (labelId(m.label)) |id| self.messages[id] = m.message;
            }
        }

        pub fn labelId(label_name: []const u8) ?u16 {
            for (bc.strs, 0..) |s, i| {
                if (std.mem.eql(u8, s, label_name)) return @intCast(i);
            }
            return null;
        }

        pub fn ruleAddress(rule: RuleT) u16 {
            return @intFromEnum(rule);
        }

        pub fn isLeftRecursive(rule: RuleT) bool {
            for (lr_rules) |r| {
                if (r == rule) return true;
            }
            return false;
        }

        /// The tree of the last match. Owned by the parser and reset by the
        /// next match; `Tree.copy` retains it.
        pub fn tree(self: *const Self) *const Tree {
            return &self.tree_storage;
        }
    };
}
