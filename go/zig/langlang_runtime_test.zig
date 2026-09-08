// Standalone tests for the runtime: `cd go/zig && zig test langlang_runtime_test.zig`.
const std = @import("std");
const rt = @import("langlang_runtime.zig");

// `G <- 'a'` compiled and encoded by the Go toolchain: call 5; halt;
// cap_begin 1; char 'a'; cap_end; cap_return.
const tiny_code = [_]u8{ 11, 5, 0, 0, 0, 15, 1, 0, 2, 'a', 0, 16, 12 };
const tiny_strs = [_][]const u8{ "", "G" };
const tiny_rxps = [_]i32{ -1, -1 };
const tiny: rt.Bytecode = .{
    .abi = rt.abi_version,
    .code = &tiny_code,
    .strs = &tiny_strs,
    .sets = &.{},
    .sexp = &.{},
    .rxps = &tiny_rxps,
};

test "charset bit layout matches go/vm_charset.go hasByte" {
    var cs: rt.Charset = .{ .bits = [_]u8{0} ** 32 };
    cs.bits['a' >> 3] |= @as(u8, 1) << @intCast('a' & 7);
    try std.testing.expect(cs.has('a'));
    try std.testing.expect(!cs.has('b'));
    try std.testing.expect(!cs.has(0));
}

test "opSize covers every opcode with the Go byte counts" {
    try std.testing.expectEqual(@as(u8, 4), rt.opSize(.call));
    try std.testing.expectEqual(@as(u8, 4), rt.opSize(.call_lr));
    try std.testing.expectEqual(@as(u8, 9), rt.opSize(.range32));
    try std.testing.expectEqual(@as(u8, 5), rt.opSize(.cap_non_term));
    try std.testing.expectEqual(@as(u8, 1), rt.opSize(.cap_return_lr));
    var total: usize = 0;
    inline for (std.meta.fields(rt.Op)) |f| total += rt.opSize(@field(rt.Op, f.name));
    try std.testing.expect(total > 0);
    try std.testing.expectEqual(@as(usize, 33), std.meta.fields(rt.Op).len);
}

test "verifyTables accepts a well-formed program" {
    try rt.verifyTables(tiny);
}

test "verifyTables rejects a bad abi, a bad opcode, and an out-of-range jump" {
    var bad = tiny;
    bad.abi = rt.abi_version + 1;
    try std.testing.expectError(error.InvalidBytecode, rt.verifyTables(bad));

    var code = tiny_code;
    code[5] = 200;
    var bad_op = tiny;
    bad_op.code = &code;
    try std.testing.expectError(error.InvalidBytecode, rt.verifyTables(bad_op));

    var jump = tiny_code;
    jump[1] = 0xff;
    jump[2] = 0xff;
    var bad_jump = tiny;
    bad_jump.code = &jump;
    try std.testing.expectError(error.InvalidBytecode, rt.verifyTables(bad_jump));

    const short_rxps = [_]i32{-1};
    var bad_rxps = tiny;
    bad_rxps.rxps = &short_rxps;
    try std.testing.expectError(error.InvalidBytecode, rt.verifyTables(bad_rxps));
}

const Rule = enum(u16) { G = 5 };
const TinyParser = rt.Interpreter(tiny, Rule, &[_]Rule{});

test "Parser resolves labels by name and stores messages" {
    var p = try TinyParser.init(std.testing.allocator);
    defer p.deinit();
    try std.testing.expectEqual(@as(?u16, 1), TinyParser.labelId("G"));
    try std.testing.expectEqual(@as(?u16, null), TinyParser.labelId("nope"));
    p.setLabelMessages(&.{ .{ .label = "G", .message = "expected a G" }, .{ .label = "nope", .message = "ignored" } });
    try std.testing.expectEqualStrings("expected a G", p.messages()[1].?);
    try std.testing.expectEqual(@as(u16, 5), TinyParser.ruleAddress(.G));
    try std.testing.expect(!TinyParser.isLeftRecursive(.G));
}

test "ParseError renders the mkErr text" {
    var buf: [128]u8 = undefined;
    var w: std.Io.Writer = .fixed(&buf);
    const e: rt.ParseError = .{ .label_id = 1, .unexpected = 'x' };
    try e.writeMessage(&w, &tiny, &.{ null, null });
    try std.testing.expectEqualStrings("[G] Unexpected 'x'", w.buffered());

    var w2: std.Io.Writer = .fixed(&buf);
    const eof: rt.ParseError = .{};
    try eof.writeMessage(&w2, &tiny, &.{ null, null });
    try std.testing.expectEqualStrings("Unexpected EOF", w2.buffered());

    var w3: std.Io.Writer = .fixed(&buf);
    try e.writeMessage(&w3, &tiny, &.{ null, "custom" });
    try std.testing.expectEqualStrings("custom", w3.buffered());

    const owned = try e.messageAlloc(std.testing.allocator, &tiny, &.{ null, null });
    defer std.testing.allocator.free(owned);
    try std.testing.expectEqualStrings("[G] Unexpected 'x'", owned);
}

test "Tree accessors and dump on a hand-built tree" {
    var t: rt.Tree = .{ .strs = &tiny_strs, .input = "ab" };
    defer t.deinit(std.testing.allocator);
    try t.nodes.append(std.testing.allocator, .{ .typ = .string, .start = 0, .end = 1 });
    try t.nodes.append(std.testing.allocator, .{ .typ = .string, .start = 1, .end = 2 });
    try t.children.appendSlice(std.testing.allocator, &.{ 0, 1 });
    try t.child_ranges.append(std.testing.allocator, .{ .start = 0, .end = 2 });
    try t.nodes.append(std.testing.allocator, .{ .typ = .sequence, .start = 0, .end = 2, .child_id = 0 });
    try t.nodes.append(std.testing.allocator, .{ .typ = .node, .start = 0, .end = 2, .name_id = 1, .child_id = 2 });
    t.root_id = 3;

    try std.testing.expectEqual(@as(usize, 1), t.childrenLen(3));
    try std.testing.expectEqual(@as(?rt.NodeId, 2), t.child(3));
    try std.testing.expectEqual(@as(usize, 2), t.childrenLen(2));
    try std.testing.expectEqual(@as(?rt.NodeId, 1), t.childAt(2, 1));
    try std.testing.expectEqualStrings("G", t.name(3));
    try std.testing.expectEqualStrings("b", t.slice(1));

    var buf: [256]u8 = undefined;
    var w: std.Io.Writer = .fixed(&buf);
    try t.dump(&w);
    try std.testing.expectEqualStrings(
        \\root=3
        \\#3 node G 0 2
        \\  #2 sequence - 0 2
        \\    #0 string - 0 1
        \\    #1 string - 1 2
        \\
    , w.buffered());

    var c = try t.copy(std.testing.allocator);
    defer c.deinit(std.testing.allocator);
    try std.testing.expectEqual(t.len(), c.len());
    t.reset();
    try std.testing.expectEqual(@as(?rt.NodeId, null), t.root());
    try std.testing.expectEqual(@as(usize, 4), c.len());
}

test "decodeRune follows Go's utf8.DecodeRune contract" {
    const D = rt.Decoded;
    try std.testing.expectEqual(D{ .rune = 'a', .size = 1 }, rt.decodeRune("a", 0));
    try std.testing.expectEqual(D{ .rune = 0xE9, .size = 2 }, rt.decodeRune("\xc3\xa9", 0));
    try std.testing.expectEqual(D{ .rune = 0x3042, .size = 3 }, rt.decodeRune("あ", 0));
    try std.testing.expectEqual(D{ .rune = 0x1F9E0, .size = 4 }, rt.decodeRune("🧠", 0));
    // invalid lead, truncated, overlong, surrogate, out of range: (U+FFFD, 1)
    try std.testing.expectEqual(D{ .rune = 0xFFFD, .size = 1 }, rt.decodeRune("\xff", 0));
    try std.testing.expectEqual(D{ .rune = 0xFFFD, .size = 1 }, rt.decodeRune("\x80", 0));
    try std.testing.expectEqual(D{ .rune = 0xFFFD, .size = 1 }, rt.decodeRune("\xe3\x81", 0));
    try std.testing.expectEqual(D{ .rune = 0xFFFD, .size = 1 }, rt.decodeRune("\xc0\x80", 0));
    try std.testing.expectEqual(D{ .rune = 0xFFFD, .size = 1 }, rt.decodeRune("\xed\xa0\x80", 0));
    try std.testing.expectEqual(D{ .rune = 0xFFFD, .size = 1 }, rt.decodeRune("\xf4\x90\x80\x80", 0));
    // a literal U+FFFD decodes as itself, size 3
    try std.testing.expectEqual(D{ .rune = 0xFFFD, .size = 3 }, rt.decodeRune("\xef\xbf\xbd", 0));
}

test "Machine matches the tiny program and reports failures like Go" {
    var m = try rt.Machine.init(std.testing.allocator, &tiny);
    defer m.deinit();

    const ok = try m.match("a");
    try std.testing.expect(ok.err == null);
    try std.testing.expectEqual(@as(u32, 1), ok.cursor);
    const t = ok.tree.?;
    const r = t.root().?;
    try std.testing.expectEqual(rt.NodeType.node, t.typ(r));
    try std.testing.expectEqualStrings("G", t.name(r));
    try std.testing.expectEqual(rt.NodeType.string, t.typ(t.child(r).?));
    try std.testing.expectEqualStrings("a", t.slice(t.child(r).?));

    const bad = try m.match("b");
    try std.testing.expect(bad.err != null);
    try std.testing.expect(bad.tree != null);
    try std.testing.expectEqual(@as(?u32, 'b'), bad.err.?.unexpected);
    try std.testing.expectEqual(@as(i32, 0), bad.err.?.end);
    try std.testing.expect(bad.tree.?.root() == null);

    const empty = try m.match("");
    try std.testing.expect(empty.err != null);
    try std.testing.expect(empty.err.?.unexpected == null);

    try std.testing.expectError(error.ParseFailed, m.parse("b"));
    try std.testing.expectEqual(@as(u32, 0), m.last_error.start);
}

test "Stack windows: capture, popAndCapture adoption, collectCaptures, truncate" {
    const gpa = std.testing.allocator;
    var s: rt.Stack = .{};
    defer s.deinit(gpa);
    try s.push(gpa, .{ .kind = .capture, .cap_id = 1 });
    try s.push(gpa, .{ .kind = .capture, .cap_id = 2 });
    try s.capture(gpa, 10);
    try s.capture(gpa, 11);
    try std.testing.expectEqualSlices(rt.NodeId, &.{ 10, 11 }, s.frameNodes(s.top().*));
    const inner = (try s.popAndCapture(gpa)).?;
    try std.testing.expectEqual(@as(u32, 2), inner.cap_id);
    try std.testing.expectEqualSlices(rt.NodeId, &.{ 10, 11 }, s.frameNodes(s.top().*));
    s.truncateArena(s.top().nodes_start);
    try std.testing.expectEqual(@as(usize, 0), s.node_arena.items.len);
    try s.capture(gpa, 12);
    try s.collectCaptures(gpa);
    try std.testing.expectEqualSlices(rt.NodeId, &.{12}, s.nodes.items);
    _ = try s.popAndCapture(gpa);
    try std.testing.expectEqual(@as(usize, 0), s.len());
    try std.testing.expect(s.pop() == null);
}
