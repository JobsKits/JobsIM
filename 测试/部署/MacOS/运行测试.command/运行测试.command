#!/bin/zsh
# shell: zsh
# Created by Jobs on 2026年11月14日，星期六.
set -eu
setopt NO_NOMATCH PIPE_FAIL
SCRIPT_DIR="${${(%):-%x}:A:h}"
REPO="${SCRIPT_DIR}/../../../.."; REPO="${REPO:A}"
RUN="$REPO/部署/MacOS/Deployment/【MacOS】部署JobsIM.command/【MacOS】部署JobsIM.command"
STOP="$REPO/部署/MacOS/Undeployment/【MacOS】反部署JobsIM.command/【MacOS】反部署JobsIM.command"
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
    for file in "$REPO"/部署/{Ubuntu,Debian,RockyLinux,AlibabaCloudLinux}/**/*.sh(.N); do bash -n "$file"; done
    local fixture="$(mktemp -d "${TMPDIR:-/tmp/}JobsIM-deploy-test.XXXXXX")"
    fixture="${fixture:A}"
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
    [[ "${2:-}" == --yes ]] || { print -u2 'integration需--yes，仅授权自动创建本轮新私密实例。'; return 2; }
    command -v python3 >/dev/null || { print -u2 '集成测试驱动需现有Python3，正式部署不依赖它。'; return 2; }
    python3 "$REPO/测试/部署/MacOS/成对原生集成.py/成对原生集成.py" --yes
}
main() {
    case "$MODE" in
        unit) unit_tests ;;
        integration) integration_tests "$@" ;;
        *) print -u2 '用法：运行测试.command unit | integration --yes'; return 2 ;;
    esac
}
main "$@"
