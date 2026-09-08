// Differential-test driver over a generated parser: the Go test writes a
// `parser.zig` next to this file and builds `zig build-exe testdriver.zig`.
const std = @import("std");
const parser = @import("parser.zig");
const common = @import("driver_common.zig");

pub fn main() !void {
    var arena = std.heap.ArenaAllocator.init(std.heap.page_allocator);
    defer arena.deinit();
    const gpa = arena.allocator();
    const args = try std.process.argsAlloc(gpa);

    var p = try parser.Parser.init(gpa);
    defer p.deinit();

    var buf: [8192]u8 = undefined;
    var w = std.fs.File.stdout().writer(&buf);
    const out = &w.interface;
    try common.run(parser.runtime, &p, &parser.bytecode, gpa, args[1..], out);
    try out.flush();
}
