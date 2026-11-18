#!/bin/zsh
# shell: zsh
# 快捷入口：只定位并转交唯一实现。
main() {
    local script_dir="${${(%):-%x}:A:h}"
    if (( $# == 0 )); then set -- undeploy; fi
    exec "$script_dir/Undeployment/【MacOS】反部署JobsIM.command/【MacOS】反部署JobsIM.command" "$@"
}
main "$@"
