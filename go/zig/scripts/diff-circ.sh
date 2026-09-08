#!/usr/bin/env sh
# Runs the Go-vs-Zig differential test over circ-compiler's own grammar and
# fixtures, plus checks that the Zig backend's tables equal the parser.go
# circ vendors today. Point CIRC_ROOT at a circ-compiler checkout
# (default: a sibling of this repository).
set -eu
here=$(cd "$(dirname "$0")" && pwd)
CIRC_ROOT=${CIRC_ROOT:-"$here/../../../../circ-compiler"}
if [ ! -f "$CIRC_ROOT/lib/grammar/proto-circ.peg" ]; then
  echo "diff-circ: no circ-compiler checkout at $CIRC_ROOT (set CIRC_ROOT)" >&2
  exit 2
fi
cd "$here/../.."
LANGLANG_REQUIRE_ZIG=1 \
LANGLANG_DIFF_GRAMMAR="$CIRC_ROOT/lib/grammar/proto-circ.peg" \
LANGLANG_DIFF_FLAGS="-disable-capture-spaces" \
LANGLANG_DIFF_CORPUS="$CIRC_ROOT/tests/fixtures/circuits/*.circ" \
LANGLANG_DIFF_LABELS="trailing=unexpected input; expected a declaration;busname=expected a port name;busassign=expected '=' after the port name;busvalue=expected a signal reference after '=';busclose=expected ')' to close the connection list" \
CIRC_PARSER_GO="$CIRC_ROOT/lib/parser/parser.go" \
go test -count=1 -run 'TestGenZigDifferentialCorpus|TestGenZigTablesMatchVendoredGo' "$@" ./
