#!/bin/zsh
# shell: zsh
# 快捷入口：只定位并转交唯一实现。
main() {
    local script_dir="${${(%):-%x}:A:h}"
    if (( $# == 0 )); then set -- stop; fi
    exec "$script_dir/停止备份.command/停止备份.command" "$@"
}
main "$@"
