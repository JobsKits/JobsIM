#!/bin/zsh
# shell: zsh
# Created by Jobs on 2026年11月18日，星期三.
YES=0
PREPARE_ONLY=0
SCRIPT_DIR=""
REPO=""
GO_TOOL=""
PRODUCT=""

# 仅解析有限的本机入口选项，不接受远端命令或实例路径。
parse_arguments() {
    while (( $# > 0 )); do
        case "$1" in
            --yes) YES=1 ;;
            --prepare-only) PREPARE_ONLY=1 ;;
            --help) print '用法：启动演示控制面.command [--yes] [--prepare-only]；默认保持前台，Ctrl+C只停止专属demo并保留数据。'; return 10 ;;
            *) print -u2 '未知选项；不接受任意命令、端口或实例路径。'; return 2 ;;
        esac
        shift
    done
}
# 第一次明确同意后才准备私有CA和当前产品。
show_intro_and_confirm() {
    print 'JobsIM Mac原生演示控制面'
    print '准备当前Go后端＋Web；不常驻Node，不使用Docker/VM。'
    print '控制HTTPS19544、业务HTTPS19543、双PG19545/19546、NATS19547、角色19560～19568。'
    print '仅本机私有LAN，固定NativeDemo-v1；不会操作默认9443、系统CA或现有其他实例。'
    print 'iPhone开关只能幂等启动固定demo；无远程停止/删库。退出本窗口停止本demo并保留资料/CA。'
    print '工具与模块只复用本机已有缓存；缺依赖时明确失败，不联网下载。'
    if (( YES == 0 )); then
        [[ -t 0 ]] || { print -u2 '需要本机终端确认；自动化须显式--yes。'; return 2; }
        local answer
        printf '按回车同意准备并开放本机LAN演示控制面，非空或EOF取消：'
        IFS= read -r answer || return 130
        [[ -z "$answer" ]] || return 130
    fi
}
# 从入口自身解析相对源码与现有用户级工具链。
initialize() {
    setopt NO_NOMATCH PIPE_FAIL
    set -eu
    umask 077
    SCRIPT_DIR="${${(%):-%x}:A:h}"
    REPO="${SCRIPT_DIR}/../../../.."; REPO="${REPO:A}"
    [[ -f "$REPO/代码/后端/Go/go.mod" ]] || { print -u2 '缺同目录布局下的JobsIM源码。'; return 2; }
    local candidate
    for candidate in "$HOME/Library/Application Support/JobsIM/Toolchains"/go1.27.*/go/bin/go(N); do GO_TOOL="$candidate"; done
    [[ -x "$GO_TOOL" ]] || { print -u2 '缺已有Go1.27工具链；先通过正式部署入口准备，不自动全局安装。'; return 2; }
    local available="$(/bin/df -k "$REPO" | /usr/bin/awk 'NR==2 {print $4}')"
    [[ "$available" == <-> ]] && (( available >= 5242880 )) || { print -u2 '可用空间不足5GiB；保留4GiB底线，未启动服务。'; return 2; }
}
# 每次在唯一新目录构建可追溯控制器，不覆盖正在使用的旧产品。
prepare_product() {
    local products="${SCRIPT_DIR:h}/Products"
    mkdir -p "$products"
    local build_dir="$(mktemp -d "$products/build.XXXXXXXX")"
    PRODUCT="$build_dir/JobsIMDemoHost"
    (cd "$REPO/代码/后端/Go" && GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2 "$GO_TOOL" build -p 1 -o "$PRODUCT" ./cmd/jobsimdemo) || return $?
    shasum -a 256 "$PRODUCT" > "$PRODUCT.sha256"
    "$PRODUCT" prepare || return $?
}
# 唯一入口保留前台生命周期，prepare-only不监听控制端口。
main() {
    local result=0
    parse_arguments "$@" || { result=$?; (( result == 10 )) && return 0; return "$result"; }
    show_intro_and_confirm || return $?
    initialize || return $?
    prepare_product || return $?
    (( PREPARE_ONLY == 0 )) || return 0
    exec "$PRODUCT" serve
}

main "$@"
