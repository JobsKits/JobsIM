#!/bin/zsh
# shell: zsh
# Created by Jobs on 2026年11月16日，星期一.
FINISHING=0
COMPLETED=0
# 展示本专项准确范围；不触碰现有应用或数据库。
show_script_intro() {
    print 'NATS隔离专项：仅新建本任务broker/私密状态，动态端口；无Go构建、PG初始化或现有realm启停。'
}
# 只在显式授权后加载实际生产函数并创建唯一隔离路径。
prepare_fixture() {
    setopt NO_NOMATCH PIPE_FAIL
    set -eu
    [[ "${1:-}" == --yes ]] || { print -u2 '用法：NATS专项.command --yes；会启停仅本轮NATS。'; return 2; }
    umask 077
    TEST_DIR="${${(%):-%x}:A:h}"
    REPO="${TEST_DIR}/../../../.."; REPO="${REPO:A}"
    RUN="$REPO/部署/MacOS/Deployment/【MacOS】部署JobsIM.command/【MacOS】部署JobsIM.command"
    SOURCE_HASH="$(shasum -a 256 "$RUN" | cut -d' ' -f1)"
    TEST_ROOT="$HOME/Library/Application Support/JobsIM/DeploymentTests"
    mkdir -p "$TEST_ROOT"
    STATE="$(mktemp -d "$TEST_ROOT/NATS-script.XXXXXX")"
    REPORT="$REPO/项目相关报告/测试/部署/NATS/$(date +%Y%m%d-%H%M%S)-$$"
    mkdir -p "$REPORT"
    # Load the real production function prefix without its cleanup hooks or entry point.
    sed '/^cleanup() {/,$d' "$RUN" > "$STATE/definitions.zsh"
    source "$STATE/definitions.zsh"
    rm -- "$STATE/definitions.zsh"; rmdir "$STATE"
    DEPLOY_MODE=""; RUN_DIR=""; LOG_FILE=/dev/null; LOCAL_ACCESS=private; CLOUDFLARED_EXECUTABLE=""; FZF_EXECUTABLE=""; TUNNEL_PID=""; BROWSER_MENU_PID=""; QR_PAGE=""; QR_BINARY=""
    ACTION=check; YES=1; MAIN_PORT=16543; JOURNAL_PORT=16544; HTTPS_PORT=9443; ROLE_PORT=18101; PUBLIC_HOST=127.0.0.1
    NATS_MAX_FILE=536870912; NATS_MAX_MEMORY=33554432
    TEST_NODE="${JOBS_IM_TEST_NODE:-$(command -v node)}"
    [[ -x "$TEST_NODE" ]] || { print -u2 '测试缺Node，可用JOBS_IM_TEST_NODE指定已有工具。'; return 2; }
    NATS_PORT="$("$TEST_NODE" -e 'const n=require("net");const s=n.createServer();s.listen(0,"127.0.0.1",()=>{console.log(s.address().port);s.close()})')"
    discover_roles
    parse_arguments check --state "$STATE"
    OPENSSL="${JOBS_IM_OPENSSL:-$(command -v openssl)}"
    find_nats
    prepare_state
    export JOBS_IM_NATS_PORT="$NATS_PORT" JOBS_IM_NATS_TOKEN_FILE
    export JOBS_IM_NATS_TEST_ID="$(date +%s)_$$"
}
# 只清理本专项可验证的进程，保留私有状态和证据。
finish_fixture() {
    local result="$1"
    (( FINISHING == 0 )) || return "$result"
    FINISHING=1
    if (( result == 0 && COMPLETED != 1 )); then result=2; fi
    [[ -n "${STATE:-}" && -n "${REPORT:-}" ]] || return "$result"
    if [[ -f "$STATE/Processes/nats.pid.original" ]]; then mv -- "$STATE/Processes/nats.pid.original" "$STATE/Processes/nats.pid"; fi
    if nats_process_matches; then stop_nats || result=2; fi
    if [[ -n "${SLEEP_PID:-}" ]] && kill -0 "$SLEEP_PID" 2>/dev/null; then kill -TERM "$SLEEP_PID"; wait "$SLEEP_PID" 2>/dev/null || true; fi
    printf 'exit=%s\nprivateState=%s\nNATSPort=%s\nroles=%s\n' "$result" "$STATE" "${NATS_PORT:-not-created}" "${SERVICES[*]:-not-created}" > "$REPORT/结果.txt"
    print "专项退出码=$result；报告=$REPORT；仅本轮私有测试数据保留。"
    exit "$result"
}
# 验证真实认证拒绝、重启保留和原密钥/配置不被替换。
verify_broker() {
    local protocol="$REPO/测试/部署/MacOS/NATS协议.mjs/NATS协议.mjs"
    start_nats
    local original_token="$(cat "$JOBS_IM_NATS_TOKEN_FILE")"
    export JOBS_IM_NATS_TEST_PID="$(cat "$STATE/Processes/nats.pid")" JOBS_IM_NATS_TEST_LOG="$STATE/Logs/nats.log"
    "$TEST_NODE" "$protocol" create | tee "$REPORT/首次协议.txt"
    stop_nats
    start_nats
    "$TEST_NODE" "$protocol" verify | tee "$REPORT/重启协议.txt"
    [[ "$(cat "$JOBS_IM_NATS_TOKEN_FILE")" == "$original_token" ]] || { print -u2 '原token被替换'; return 2; }
    unset original_token
    stop_nats
    write_nats_backup_manifest "$REPORT/broker-manifest.txt"
    cp "$NATS_CONFIG" "$STATE/Secrets/nats.original"
    printf '\n# isolated-drift\n' >> "$NATS_CONFIG"
    if prepare_nats >/dev/null 2>&1; then print -u2 '配置漂移未拒绝'; return 2; fi
    mv -- "$STATE/Secrets/nats.original" "$NATS_CONFIG"
    print 'PASS 配置漂移拒绝且原件保留' | tee "$REPORT/归属保护.txt"
}
# 用本轮sleep作陌生PID夹具，证明不会按PID或名称误停。
verify_foreign_pid() {
    sleep 30 &
    SLEEP_PID=$!
    cp "$STATE/Processes/nats.pid" "$STATE/Processes/nats.pid.original"
    print "$SLEEP_PID" > "$STATE/Processes/nats.pid"
    if stop_nats >/dev/null 2>&1; then print -u2 '陌生PID未拒绝'; return 2; fi
    kill -0 "$SLEEP_PID"
    mv -- "$STATE/Processes/nats.pid.original" "$STATE/Processes/nats.pid"
    kill -TERM "$SLEEP_PID"; wait "$SLEEP_PID" 2>/dev/null || true; SLEEP_PID=''
    print 'PASS 外来PID保留；自有sleep已精确结束' | tee -a "$REPORT/归属保护.txt"
    [[ "$(shasum -a 256 "$RUN" | cut -d' ' -f1)" == "$SOURCE_HASH" ]] || { print -u2 '专项期间源码变化，不能绑定当前SHA，需新轮重验。'; return 2; }
    shasum -a 256 "$RUN" "$REPO/部署/MacOS/Undeployment/【MacOS】反部署JobsIM.command/【MacOS】反部署JobsIM.command" > "$REPORT/源码SHA256.txt"
    COMPLETED=1
}
# 脚本级退出钩子避免函数返回时提前清理，始终只处理本专项收据。
TRAPEXIT() { finish_fixture "$?"; }
TRAPINT() { exit 130; }
TRAPTERM() { exit 143; }
TRAPHUP() { exit 143; }
# 唯一测试入口；只有显式--yes才启停本轮broker。
main() {
    show_script_intro # 展示准确副作用和现有实例保护。
    prepare_fixture "$@" # 建立唯一私密目录与动态端口。
    verify_broker # 验证真实NATS认证、持久重启和配置漂移。
    verify_foreign_pid # 验证未知进程不被误停并封存摘要。
}
main "$@"
