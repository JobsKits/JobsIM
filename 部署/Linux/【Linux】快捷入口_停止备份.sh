#!/bin/bash
set -euo pipefail
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
if (( $# == 0 )); then exec "$SCRIPT_DIR/停止备份.sh/停止备份.sh" stop; fi
exec "$SCRIPT_DIR/停止备份.sh/停止备份.sh" "$@"
