# 输出纯文本审计并只给实际终端加颜色，不改调用方或系统配置。
log_message() {
    local style="$1" message="$2"
    if [[ -t 1 && -z "${NO_COLOR+x}" && "${TERM:-dumb}" != dumb ]]; then printf '\033[%sm%s\033[0m\n' "$style" "$message"; else print -r -- "$message"; fi
    [[ "$LOG_FILE" == /dev/null ]] || print -r -- "$message" >> "$LOG_FILE"
}
# 工具准备失败立即退出，既有实例不被当作新部署成功。
fail() { log_message '1;31' "$1"; exit "${2:-2}"; }
# 模块获取继承当前进程的显式策略，不写Go全局环境。
configure_go_module_source() { return 0; }
# 交互后的工具与诊断仅放本轮机器私有会话，默认实例和源码不写入引导缓存。
prepare_session_context() {
    [[ -z "$RUN_DIR" ]] || return 0
    local base="${TMPDIR:-/tmp/}"
    RUN_DIR="$(mktemp -d "${base%/}/JobsIM-deploy-session.XXXXXX")"
    RUN_DIR="${RUN_DIR:A}"
    LOG_FILE="$RUN_DIR/deploy.log"; : > "$LOG_FILE"
    PROJECT_DIR="$REPO"; ENV_ROOT="$HOME/Library/Application Support/JobsIM/Toolchains"
    PROJECT_ENV_DIR="$RUN_DIR/bootstrap-environment"
    PLATFORM_OS=darwin; PLATFORM_LABEL=MacOS
    case "$(uname -m)" in arm64) PLATFORM_ARCH=arm64;; x86_64) PLATFORM_ARCH=amd64;; *) fail '当前架构无可验证工具路线。';; esac
    GO_BIN="${JOBS_IM_GO:-}"
    if [[ -z "$GO_BIN" ]]; then
        local candidate
        for candidate in "$ENV_ROOT"/go1.27.*/go/bin/go(N); do GO_BIN="$candidate"; done
    fi
    WB_FZF_BIN="${JOBS_IM_FZF_BIN:-}"
    WB_CLOUDFLARED_BIN="${JOBS_IM_CLOUDFLARED_BIN:-}"
    WB_DEPENDENCY_CHECK_TIMEOUT_SECONDS="${JOBS_IM_DEPENDENCY_CHECK_TIMEOUT_SECONDS:-30}"
    print "本轮工具检查与诊断：$RUN_DIR"
}
# 选择器自动发现健康候选，缺失才按可验证锁引导到本轮目录。
prepare_selector() {
    prepare_session_context
    prepare_fzf
}
# 首屏和模式确认均在接管旧服务之前，取消不会停止既有实例。
select_im_mode() {
    prepare_selector
    pick_operation '部署模式' '↑/↓选择、回车确认；Esc取消。本地模式会公开临时HTTPS入口。' $'local\t本地部署＋内网穿透' $'server\t服务器部署' || return $?
    [[ "$SELECTED_OPERATION" == local || "$SELECTED_OPERATION" == server ]] || return 2
    DEPLOY_MODE="$SELECTED_OPERATION"
    if [[ "$DEPLOY_MODE" == local ]]; then
        terminal_notice
        print '本轮仅公开JobsIM用户入口；持有临时网址的人可以访问，业务登录/权限继续生效。仅使用合成资料。'
        local answer
        printf '已了解公开范围，按回车继续；其它输入或输入结束取消：'
        IFS= read -r answer || return 130
        [[ -z "$answer" ]] || return 130
    else
        print '服务器模式：不建立Quick Tunnel；可用JOBS_IM_PUBLIC_URL配置真实外部HTTPS地址，并自行配置反向代理。'
    fi
}
# 在相同支持主版本内核对数据库目标，绝不在部署中跨major升级既有集群。
verify_native_tool_targets() {
    local tool candidate current target latest row kind cache archive extraction selected name
    for tool in node nats-server postgres; do
        resolve_latest_asset "$tool"
        latest="$RESOLVED_VERSION"; target="$latest"; row="$RESOLVED_ROW"; kind="$RESOLVED_KIND"
        case "$tool" in
            node) candidate="${JOBS_IM_NODE:-$(command -v node || true)}"; current="$("$candidate" --version 2>/dev/null | sed -n 's/^v//p')" ;;
            nats-server) candidate="$NATS_BIN"; current="$("$candidate" --version 2>/dev/null | sed -nE 's/.*v([0-9]+\.[0-9]+\.[0-9]+).*/\1/p')" ;;
            postgres) candidate="$PG_BIN/pg_ctl"; current="$("$candidate" --version 2>/dev/null | sed -nE 's/.* ([0-9]+\.[0-9]+).*/\1/p')"; target="$(print -r -- "$row" | cut -f4)" ;;
        esac
        [[ -n "$current" ]] || fail "无法核验当前$tool版本。"
        if [[ "$tool" == postgres && "${latest%%.*}" != "${current%%.*}" ]]; then
            print "数据库官方最新=$latest；当前受支持主版本=${current%%.*}，现有集群不跨主版本自动升级，兼容补丁目标须单独核对。"
            [[ "${current%%.*}" == 18 ]] || fail 'JobsIM当前部署仅支持既有PG18配套工具；不会把其他major当已适配。'
            continue
        fi
        print "工具核验：$tool；实际=$current；官方目标=$target；时间=$(date -u +%Y-%m-%dT%H:%M:%SZ)；位置=$candidate"
        if tool_at_least "$current" "$target"; then continue; fi
        [[ "$tool" != postgres && "$kind" != unsupported ]] || fail "当前$tool未达官方目标；缺可验证本平台原生包时须由操作者提供健康候选，不改既有数据库。"
        case "$tool" in node) [[ -z "${JOBS_IM_NODE:-}" ]] || fail '显式Node过旧，保留原文件。';; nats-server) [[ -z "${JOBS_IM_NATS_BIN:-}" ]] || fail '显式NATS过旧，保留原文件。';; esac
        cache="$PROJECT_ENV_DIR/Tools/$tool/$target/$PLATFORM_OS-$PLATFORM_ARCH"
        [[ ! -e "$cache" && ! -L "$cache" ]] || fail '目标工具缓存已存在但未通过健康验证，保留不覆盖。'
        mkdir -p "${cache:h}"
        extraction="$(mktemp -d "$RUN_DIR/native-tool.XXXXXX")"
        archive="$extraction/archive"
        download_verified "$RESOLVED_URL" "$archive" "$RESOLVED_SHA"
        mkdir "$extraction/unpacked"
        tar -xzf "$archive" -C "$extraction/unpacked" || return $?
        local -a tools
        if [[ "$tool" == node ]]; then tools=("$extraction/unpacked"/node-*/bin/node(N)); else tools=("$extraction/unpacked"/nats-server-*/nats-server(N)); fi
        (( ${#tools} == 1 )) || fail '原生归档没有唯一匹配的工具。'
        selected="${tools[1]}"; "$selected" --version || fail '新工具不可运行，不采用其他架构。'
        mv -- "$extraction/unpacked" "$cache"
        selected="$cache/${selected#$extraction/unpacked/}"
        if [[ "$tool" == node ]]; then export PATH="${selected:h}:$PATH"; else NATS_BIN="$selected"; fi
    done
}
# 只有选择模式后才查询官方稳定目标；业务锁文件以只读副本检查。
prepare_im_tools() {
    prepare_go
    prepare_dependency_resolver
    prepare_latest_tools
    GO_TOOL="$GO_EXECUTABLE"
    find_tools
    verify_native_tool_targets
    check_business_dependencies
}
# 撤销入口亲自移动走的临时顶层条目，不忘记仍存在的对象。
origin_forget_removed() {
    local relative="$1" next
    [[ ! -e "$STATE/$relative" && ! -L "$STATE/$relative" ]] || return 2
    next="$(mktemp "$STATE/.Deployment/.resources.XXXXXX")"
    awk -F '\t' -v name="$relative" '$2!=name' "$STATE/.Deployment/resources.tsv" > "$next"
    mv -- "$next" "$STATE/.Deployment/resources.tsv"
}
# 仅为本次cloudflared清除继承的Tunnel配置，CA校验始终开启。
run_im_quick_client() {
    local name
    for name in ${(f)"$(env | sed -n 's/^\(TUNNEL_[A-Za-z0-9_]*\)=.*/\1/p')"}; do unset "$name"; done
    exec "$CLOUDFLARED_EXECUTABLE" tunnel --config "$RUN_DIR/quick-empty.yml" --no-autoupdate --url "https://127.0.0.1:$HTTPS_PORT" --origin-ca-pool "$STATE/TLS/ca.crt"
}
# 严格本机健康后建Quick，等实际网址后再重启本轮Go绑定正确的BFF Origin。
start_im_tunnel() {
    [[ "$DEPLOY_MODE" == local ]] || return 0
    print '{}' > "$RUN_DIR/quick-empty.yml"
    run_im_quick_client > "$RUN_DIR/tunnel.log" 2>&1 &
    TUNNEL_PID=$!
    print "$TUNNEL_PID" > "$STATE/Processes/tunnel.pid"
    LC_ALL=C ps -p "$TUNNEL_PID" -o lstart= > "$STATE/Processes/tunnel.start"
    ps -ww -p "$TUNNEL_PID" -o command= > "$STATE/Processes/tunnel.command"
    local tries=0 timeout="${JOBS_IM_TUNNEL_START_TIMEOUT:-45}" address=""
    [[ "$timeout" =~ '^[1-9][0-9]{0,2}$' ]] && (( timeout <= 600 )) || return 2
    while (( tries < timeout * 2 )); do
        kill -0 "$TUNNEL_PID" 2>/dev/null || { print -u2 'Quick进程已退出，公网未就绪。'; return 2; }
        address="$(grep -Eo 'https://[a-zA-Z0-9-]+\.trycloudflare\.com' "$RUN_DIR/tunnel.log" | tail -1 || true)"
        [[ -z "$address" ]] || { PUBLIC_URL="$address"; break; }
        sleep .5; (( tries+=1 ))
    done
    [[ -n "$PUBLIC_URL" ]] || { print -u2 'Quick网址获取超时，未报告公网成功。'; return 2; }
    stop_backend
    launch_im_backend "$PUBLIC_URL"
    curl --fail --silent --show-error --max-time 20 "$PUBLIC_URL/api/health" > "$RUN_DIR/public-health.json" || return $?
    print '实际公网入口（仍须独立外网业务验收）：'; show_endpoint "$PUBLIC_URL"
}
# 只停止本轮登记且启动身份和完整命令相符的隧道。
stop_tunnel() {
    [[ -f "$STATE/Processes/tunnel.pid" ]] || return 0
    local pid="$(cat "$STATE/Processes/tunnel.pid")" started command
    [[ "$pid" == <-> ]] || return 2
    kill -0 "$pid" 2>/dev/null || return 0
    started="$(LC_ALL=C ps -p "$pid" -o lstart= 2>/dev/null || true)"
    command="$(ps -ww -p "$pid" -o command= 2>/dev/null || true)"
    [[ -n "$started" && "$started" == "$(cat "$STATE/Processes/tunnel.start")" && "$command" == "$(cat "$STATE/Processes/tunnel.command")" && "$command" == *"$RUN_DIR/quick-empty.yml"* ]] || { print -u2 '隧道PID身份不符，保留进程。'; return 2; }
    kill -TERM "$pid"
    local attempts=0
    while kill -0 "$pid" 2>/dev/null && (( attempts < 50 )); do sleep .1; (( attempts+=1 )); done
    ! kill -0 "$pid" 2>/dev/null || return 2
}
# 用真实部署URL启动同一个Go候选，身份、PG与NATS均不重新初始化。
launch_im_backend() {
    local url="$1" ready=0 count=0 pid
    JOBS_IM_JOURNAL_DATABASE_URL="$JOBS_IM_JOURNAL_DATABASE_URL" JOBS_IM_PUBLIC_URL="$url" "$STATE/Bin/JobsIMd" --service all --addr "127.0.0.1:$HTTPS_PORT" --internal-base-port "$ROLE_PORT" --state-dir "$STATE/Application" --tls-cert "$STATE/TLS/server.crt" --tls-key "$STATE/TLS/server.key" --web-dir "$WEB_RUNTIME" > "$STATE/Logs/server.log" 2>&1 &
    pid=$!; SERVER_PID="$pid"
    print "$pid" > "$STATE/Processes/server.pid"
    LC_ALL=C ps -p "$pid" -o lstart= > "$STATE/Processes/server.start"
    while (( count < 60 )); do
        if curl --fail --silent --cacert "$STATE/TLS/ca.crt" --max-time 1 "https://127.0.0.1:$HTTPS_PORT/api/health" >/dev/null 2>&1; then ready=1; break; fi
        kill -0 "$pid" 2>/dev/null || break
        sleep .5; (( count+=1 ))
    done
    (( ready == 1 )) || { print -u2 "本次服务未通过严格TLS健康，见$STATE/Logs/server.log"; return 2; }
}
# 用本地锁定二维码编码器生成离线页；不存在的管理网页不生成二维码。
prepare_im_qr() {
    QR_BINARY="$RUN_DIR/address-qr"
    if ! (cd "$REPO/部署/公共组件/地址二维码" && GOTOOLCHAIN=local "$GO_TOOL" build -mod=readonly -o "$QR_BINARY" .); then
        QR_BINARY=""; print -u2 '二维码编码器构建失败；服务保留，完整URL仍可用。'; return 0
    fi
    local url="${PUBLIC_URL:-https://$PUBLIC_HOST:$HTTPS_PORT}" width="${COLUMNS:-0}"
    [[ "$width" == <-> ]] || width=0
    "$QR_BINARY" --output-dir "$RUN_DIR/AccessQR" --title 'JobsIM访问入口' --entry "用户端|$url/" --entry "仅本机，手机不可用|https://127.0.0.1:$HTTPS_PORT/" --terminal-width "$width" > "$RUN_DIR/qr-output.txt" 2>&1 || { print -u2 '二维码生成失败；保留URL，业务继续。'; return 0; }
    cat "$RUN_DIR/qr-output.txt"
    [[ ! -f "$RUN_DIR/AccessQR/index.html" ]] || QR_PAGE="$RUN_DIR/AccessQR/index.html"
}
# 菜单打开失败或收起不关闭服务；回车可再次进入，服务退出则统一回收。
monitor_im_session() {
    local paused=0 selected input
    while kill -0 "$SERVER_PID" 2>/dev/null; do
        if [[ -n "$TUNNEL_PID" ]] && ! kill -0 "$TUNNEL_PID" 2>/dev/null; then print -u2 '本轮穿透已结束，停止本轮服务。'; return 2; fi
        if [[ -t 0 && $paused == 0 ]]; then
            if pick_operation '打开网页' '回车打开；Esc收起菜单，服务继续；Ctrl+C停止。' $'local\t本机用户端' $'public\t公网用户端（仅本地穿透模式）' $'qr\t打开离线扫码页面' $'pause\t收起菜单'; then
                case "$SELECTED_OPERATION" in
                    local) open "https://127.0.0.1:$HTTPS_PORT/" || print '浏览器打开失败，可再次选择。' ;;
                    public) [[ -z "$PUBLIC_URL" ]] || open "$PUBLIC_URL/" || print '浏览器打开失败，可再次选择。' ;;
                    qr) [[ -z "$QR_PAGE" ]] || open "$QR_PAGE" || print '扫码页打开失败，可再次选择。' ;;
                    pause) paused=1 ;;
                esac
            else paused=1; fi
        elif [[ -t 0 ]] && IFS= read -r -t .2 input; then paused=0
        else sleep .5; fi
    done
    wait "$SERVER_PID"
}
