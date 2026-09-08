// Shared body of the two differential-test drivers. `R` is the runtime
// namespace (the standalone file for vmdriver, `parser.runtime` for
// testdriver) and `p` is a `*Machine` or `*Interpreter(...)`; both expose
// the same method set.
//
// Arguments: --input <path> [--messages "label=msg;label=msg"] [--show-fails]
// Output (must match go/tree_canonical.go CanonicalDump byte for byte):
//   cursor=<n>
//   error label=<label or -> start=<s> end=<e> tree=<yes|no> msg=<text>   (on failure)
//   root=<id> / noroot and the pre-order node lines                       (when a tree was returned)
const std = @import("std");

pub fn run(comptime R: type, p: anytype, bc: *const R.Bytecode, gpa: std.mem.Allocator, args: []const [:0]u8, out: *std.Io.Writer) !void {
    var input_path: ?[]const u8 = null;
    var show_fails = false;
    var msgs: std.ArrayList(R.LabelMessage) = .empty;
    defer msgs.deinit(gpa);

    var i: usize = 0;
    while (i < args.len) : (i += 1) {
        const a = args[i];
        if (std.mem.eql(u8, a, "--input")) {
            i += 1;
            if (i >= args.len) return error.MissingArgument;
            input_path = args[i];
        } else if (std.mem.eql(u8, a, "--show-fails")) {
            show_fails = true;
        } else if (std.mem.eql(u8, a, "--messages")) {
            i += 1;
            if (i >= args.len) return error.MissingArgument;
            var pairs = std.mem.splitScalar(u8, args[i], ';');
            while (pairs.next()) |pair| {
                if (pair.len == 0) continue;
                const eq = std.mem.indexOfScalar(u8, pair, '=') orelse return error.BadMessage;
                try msgs.append(gpa, .{ .label = pair[0..eq], .message = pair[eq + 1 ..] });
            }
        } else {
            return error.UnknownArgument;
        }
    }
    const path = input_path orelse return error.MissingArgument;
    const input = try std.fs.cwd().readFileAlloc(gpa, path, 1 << 30);

    p.setShowFails(show_fails);
    p.setLabelMessages(msgs.items);
    const r = try p.match(input);

    try out.print("cursor={d}\n", .{r.cursor});
    if (r.err) |e| {
        const label = e.label(bc);
        try out.print("error label={s} start={d} end={d} tree={s} msg=", .{
            if (label.len == 0) "-" else label,
            e.start,
            e.end,
            if (r.tree != null) "yes" else "no",
        });
        try e.writeMessage(out, bc, p.messages());
        try out.writeByte('\n');
    }
    if (r.tree) |t| try t.dump(out);
}
