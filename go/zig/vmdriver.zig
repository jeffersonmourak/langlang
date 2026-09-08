// Differential-test driver over runtime tables: the Go test serialises a
// Bytecode into a blob (see zigTableBlob in genzig_diff_test.go) so one
// binary can replay the whole VM test table without generating and
// compiling a parser per grammar.
//
// Blob layout, all little-endian: "LLZT", u32 abi, u32 code_len, code,
// u32 nstrs, nstrs × (u32 len, bytes), u32 nsets, nsets × 32 bytes,
// nsets × (u32 count, count × (u32 a, u32 b)), u32 nrxps, nrxps × i32.
const std = @import("std");
const rt = @import("langlang_runtime.zig");
const common = @import("driver_common.zig");

const Reader = struct {
    data: []const u8,
    pos: usize = 0,

    fn u32le(r: *Reader) !u32 {
        if (r.pos + 4 > r.data.len) return error.TruncatedBlob;
        const v = std.mem.readInt(u32, r.data[r.pos..][0..4], .little);
        r.pos += 4;
        return v;
    }

    fn bytes(r: *Reader, n: usize) ![]const u8 {
        if (r.pos + n > r.data.len) return error.TruncatedBlob;
        const s = r.data[r.pos .. r.pos + n];
        r.pos += n;
        return s;
    }
};

fn loadTables(gpa: std.mem.Allocator, blob: []const u8) !rt.Bytecode {
    var r: Reader = .{ .data = blob };
    if (!std.mem.eql(u8, try r.bytes(4), "LLZT")) return error.BadMagic;
    const abi = try r.u32le();
    const code = try r.bytes(try r.u32le());

    const nstrs = try r.u32le();
    const strs = try gpa.alloc([]const u8, nstrs);
    for (strs) |*s| s.* = try r.bytes(try r.u32le());

    const nsets = try r.u32le();
    const sets = try gpa.alloc(rt.Charset, nsets);
    for (sets) |*s| s.bits = (try r.bytes(32))[0..32].*;

    const sexp = try gpa.alloc([]const rt.Expected, nsets);
    for (sexp) |*e| {
        const count = try r.u32le();
        const items = try gpa.alloc(rt.Expected, count);
        for (items) |*it| {
            it.a = try r.u32le();
            it.b = try r.u32le();
        }
        e.* = items;
    }

    const nrxps = try r.u32le();
    const rxps = try gpa.alloc(i32, nrxps);
    for (rxps) |*x| x.* = @bitCast(try r.u32le());

    return .{ .abi = abi, .code = code, .strs = strs, .sets = sets, .sexp = sexp, .rxps = rxps };
}

pub fn main() !void {
    var arena = std.heap.ArenaAllocator.init(std.heap.page_allocator);
    defer arena.deinit();
    const gpa = arena.allocator();
    const args = try std.process.argsAlloc(gpa);

    // `--tables <path>` comes first; the rest is the common argument set.
    if (args.len < 3 or !std.mem.eql(u8, args[1], "--tables")) return error.MissingTables;
    const blob = try std.fs.cwd().readFileAlloc(gpa, args[2], 1 << 30);
    const bc = try loadTables(gpa, blob);
    try rt.verifyTables(bc);

    var m = try rt.Machine.init(gpa, &bc);
    defer m.deinit();

    var buf: [8192]u8 = undefined;
    var w = std.fs.File.stdout().writer(&buf);
    const out = &w.interface;
    try common.run(rt, &m, &bc, gpa, args[3..], out);
    try out.flush();
}
