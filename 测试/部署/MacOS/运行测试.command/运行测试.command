#!/bin/zsh
# shell: zsh
# Created by Jobs on 2026年11月14日，星期六.
set -eu
setopt NO_NOMATCH PIPE_FAIL
SCRIPT_DIR="${${(%):-%x}:A:h}"
REPO="${SCRIPT_DIR}/../../../.."; REPO="${REPO:A}"
RUN="$REPO/部署/MacOS/部署运行.command/部署运行.command"
STOP="$REPO/部署/MacOS/停止备份.command/停止备份.command"
MODE="${1:-unit}"
CONTROLLER=''
cleanup() {
    if [[ -n "$CONTROLLER" ]] && kill -0 "$CONTROLLER" 2>/dev/null; then
        kill -TERM "$CONTROLLER"
        wait "$CONTROLLER" 2>/dev/null || true
    fi
}
TRAPEXIT() { cleanup; }
TRAPINT() { cleanup; exit 130; }
TRAPTERM() { cleanup; exit 143; }
unit_tests() {
    local file
    for file in "$REPO"/部署/MacOS/**/*.command(.N); do zsh -n "$file"; done
    for file in "$REPO"/部署/Linux/**/*.sh(.N); do bash -n "$file"; done
    local fixture="$(mktemp -d "${TMPDIR:-/tmp/}JobsIM-deploy-test.XXXXXX")"
    "$RUN" check --state "$fixture/untouched" > /dev/null
    [[ ! -e "$fixture/untouched" ]] || { print -u2 'check创建了state'; return 2; }
    "$RUN" plan --state "$fixture/untouched" > /dev/null
    [[ ! -e "$fixture/untouched" ]] || return 2
    mkdir "$fixture/foreign"
    printf 'keep\n' > "$fixture/foreign/sentinel"
    if "$RUN" check --state "$fixture/foreign" >/dev/null 2>&1; then print -u2 '错误接管无收据state'; return 2; fi
    [[ "$(cat "$fixture/foreign/sentinel")" == keep ]] || return 2
    rm -- "$fixture/foreign/sentinel"; rmdir "$fixture/foreign" "$fixture"
    print 'PASS：Mac/Linux语法、check/plan零state写入、陌生非空state拒绝接管。'
}
wait_ready() {
    local count=0
    while (( count < 90 )); do
        if [[ -f "$JOBS_IM_TEST_STATE/TLS/ca.crt" ]] && curl --fail --silent --cacert "$JOBS_IM_TEST_STATE/TLS/ca.crt" --max-time 1 "https://127.0.0.1:$JOBS_IM_HTTPS_PORT/api/bootstrap" > "$REPORT/bootstrap.json"; then return 0; fi
        kill -0 "$CONTROLLER" 2>/dev/null || { print -u2 "入口提前退出，见$REPORT/run.log"; return 2; }
        sleep 0.5; (( count+=1 ))
    done
    print -u2 '等候入口启动超时'; return 2
}
integration_tests() {
    [[ "${2:-}" == --yes && -n "${JOBS_IM_TEST_STATE:-}" ]] || { print -u2 'integration需显式JOBS_IM_TEST_STATE和第二参数--yes；会创建测试账户、数据库和备份。'; return 2; }
    export JOBS_IM_HTTPS_PORT="${JOBS_IM_HTTPS_PORT:-19443}"
    export JOBS_IM_PG_PORT="${JOBS_IM_PG_PORT:-17543}"
    export JOBS_IM_JOURNAL_PORT="${JOBS_IM_JOURNAL_PORT:-17544}"
    export JOBS_IM_INTERNAL_PORT="${JOBS_IM_INTERNAL_PORT:-20101}"
    local REPORT="$REPO/项目相关报告/测试/部署/MacOS/$(date +%Y%m%d-%H%M%S)"
    mkdir -p "$REPORT"
    "$RUN" run --yes --state "$JOBS_IM_TEST_STATE" > "$REPORT/run.log" 2>&1 &
    CONTROLLER=$!
    wait_ready
    NODE_EXTRA_CA_CERTS="$JOBS_IM_TEST_STATE/TLS/ca.crt" JOBS_IM_TEST_URL="https://127.0.0.1:$JOBS_IM_HTTPS_PORT" node "$REPO/测试/部署/MacOS/ContractSmoke.mjs" | tee "$REPORT/smoke.log"
    "$STOP" stop --yes --state "$JOBS_IM_TEST_STATE" | tee "$REPORT/backup-stop.log"
    wait "$CONTROLLER"; CONTROLLER=''
    if lsof -nP -iTCP:"$JOBS_IM_HTTPS_PORT" -sTCP:LISTEN >/dev/null 2>&1; then return 2; fi
    [[ -f "$JOBS_IM_TEST_STATE/MainDatabase/PG_VERSION" && -f "$JOBS_IM_TEST_STATE/JournalDatabase/PG_VERSION" ]] || return 2
    print 'PASS：真实独立双PG、源码构建、严格TLS、聊天契约、加密备份验真和保留数据停止。恢复演练未包含。' | tee "$REPORT/result.txt"
}
main() {
    case "$MODE" in
        unit) unit_tests ;;
        integration) integration_tests "$@" ;;
        *) print -u2 '用法：运行测试.command unit | integration --yes'; return 2 ;;
    esac
}
main "$@"
