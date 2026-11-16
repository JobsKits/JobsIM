#!/bin/bash
set -euo pipefail
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
if (( $# == 0 )); then exec "$SCRIPT_DIR/部署运行.sh/部署运行.sh" run; fi
exec "$SCRIPT_DIR/部署运行.sh/部署运行.sh" "$@"
