#!/bin/zsh
# shell: zsh
# 快捷入口：只定位并转交唯一实现。
main() {
    local script_dir="${${(%):-%x}:A:h}"
    if (( $# == 0 )); then set -- run; fi
    exec "$script_dir/部署运行.command/部署运行.command" "$@"
}
main "$@"
