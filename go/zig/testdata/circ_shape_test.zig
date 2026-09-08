// Exercises exactly the surface circ-compiler's lib/syntax/translate.zig
// needs from a generated parser: the six accessors behind its wrappers
// (typ, name, range, childAt, childrenLen, child), the root, and the
// parse-failure information for its ParseFailure. Run by
// TestGenZigCompiles against a parser generated from tests/circ/circ.peg.
const std = @import("std");
const parser = @import("parser.zig");
const rt = parser.runtime;

const circ_label_messages = [_]rt.LabelMessage{
    .{ .label = "trailing", .message = "unexpected input; expected a declaration" },
    .{ .label = "busname", .message = "expected a port name" },
    .{ .label = "busassign", .message = "expected '=' after the port name" },
    .{ .label = "busvalue", .message = "expected a signal reference after '='" },
    .{ .label = "busclose", .message = "expected ')' to close the connection list" },
};

fn findByName(t: *const rt.Tree, id: rt.NodeId, name: []const u8) ?rt.NodeId {
    if (std.mem.eql(u8, t.name(id), name)) return id;
    var i: usize = 0;
    while (i < t.childrenLen(id)) : (i += 1) {
        if (findByName(t, t.childAt(id, i).?, name)) |found| return found;
    }
    return null;
}

fn countErrors(t: *const rt.Tree, id: rt.NodeId) usize {
    var n: usize = if (t.typ(id) == .err) 1 else 0;
    var i: usize = 0;
    while (i < t.childrenLen(id)) : (i += 1) n += countErrors(t, t.childAt(id, i).?);
    return n;
}

test "a clean circuit parses to a Program node with byte ranges" {
    var p = try parser.Parser.init(std.testing.allocator);
    defer p.deinit();
    p.setLabelMessages(&circ_label_messages);

    const src = "input a, b\nand g(a=a, b=b)\noutput out(in=g.out)\n";
    const tree = try p.parse(src);
    const root = tree.root() orelse return error.NoRoot;
    try std.testing.expectEqual(rt.NodeType.node, tree.typ(root));
    try std.testing.expectEqualStrings("Program", tree.name(root));
    try std.testing.expectEqual(@as(usize, 0), tree.range(root).start);
    try std.testing.expectEqual(src.len, tree.range(root).end);

    // Node/err carry exactly one child; sequences carry many.
    try std.testing.expectEqual(@as(usize, 1), tree.childrenLen(root));
    const body = tree.child(root).?;
    try std.testing.expectEqual(body, tree.childAt(root, 0).?);
    try std.testing.expect(tree.childAt(root, 1) == null);
    try std.testing.expectEqual(rt.NodeType.sequence, tree.typ(body));
    try std.testing.expectEqual(@as(usize, 3), tree.childrenLen(body));

    const decl = findByName(tree, root, "Declaration") orelse return error.NoDeclaration;
    try std.testing.expectEqualStrings("and g(a=a, b=b)", tree.slice(decl));
    try std.testing.expectEqual(@as(usize, 0), countErrors(tree, root));
    try std.testing.expectEqual(@as(usize, 0), std.mem.count(u8, tree.name(decl), "\x00"));
}

test "recovery labels surface as err nodes with the bound messages" {
    var p = try parser.Parser.init(std.testing.allocator);
    defer p.deinit();
    p.setLabelMessages(&circ_label_messages);

    const tree = try p.parse("input a\nand g(a=a b=a)\n");
    const root = tree.root() orelse return error.NoRoot;
    try std.testing.expect(countErrors(tree, root) >= 1);
    const err = findByName(tree, root, "busclose") orelse return error.NoBuscloseError;
    try std.testing.expectEqual(rt.NodeType.err, tree.typ(err));
    try std.testing.expectEqualStrings("expected ')' to close the connection list", tree.message(err));
    // busclose's recovery expression is empty: a zero-width error node.
    try std.testing.expectEqual(tree.range(err).start, tree.range(err).end);

    // The `and g(a=` case ends in a busvalue then busclose mark.
    const t2 = try p.parse("input a\nand g(a=\n");
    const r2 = t2.root() orelse return error.NoRoot;
    try std.testing.expect(findByName(t2, r2, "busvalue") != null);
    try std.testing.expect(findByName(t2, r2, "busclose") != null);
}

test "empty input yields no root, junk yields a RecoverLine" {
    var p = try parser.Parser.init(std.testing.allocator);
    defer p.deinit();

    const empty = try p.parse("");
    try std.testing.expect(empty.root() == null);

    const junk = try p.parse("%%% not circ at all %%%");
    const root = junk.root() orelse return error.NoRoot;
    try std.testing.expect(findByName(junk, root, "RecoverLine") != null);
}

test "rule enum and tables are wired for the circ grammar" {
    try std.testing.expectEqual(@as(u16, 5), parser.Parser.ruleAddress(parser.entry_rule));
    try std.testing.expectEqualStrings("Program", @tagName(parser.entry_rule));
    try std.testing.expectEqual(@as(usize, 0), parser.left_recursive_rules.len);
    try std.testing.expect(parser.Parser.labelId("busclose") != null);
    try rt.verifyTables(parser.bytecode);
}
