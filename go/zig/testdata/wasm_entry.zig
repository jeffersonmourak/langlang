// Freestanding wasm32 host for a generated parser.zig, used by the
// TestGenZigCompiles wasm smoke: proves the generated file links into a
// module with no libc and reports the module's size.
//
//   zig build-exe wasm_entry.zig -target wasm32-freestanding -O ReleaseSmall -fno-entry --export=parse --export=alloc
const std = @import("std");
const parser = @import("parser.zig");

var machine: ?parser.Parser = null;

export fn alloc(len: usize) ?[*]u8 {
    const buf = std.heap.wasm_allocator.alloc(u8, len) catch return null;
    return buf.ptr;
}

/// Returns 0 on a successful parse, 1 on a parse failure, -1 when the
/// parser could not be created, and -2 on any other error.
export fn parse(ptr: [*]const u8, len: usize) i32 {
    if (machine == null) {
        machine = parser.Parser.init(std.heap.wasm_allocator) catch return -1;
    }
    _ = machine.?.parse(ptr[0..len]) catch |err| switch (err) {
        error.ParseFailed => return 1,
        else => return -2,
    };
    return 0;
}
