# 已存在的TLS只核对不覆盖；服务器模式要求明确的URL、证书、私钥与信任链。
prepare_server_tls() {
    local cert="${JOBS_IM_TLS_CERT:-}" key="${JOBS_IM_TLS_KEY:-}" ca="${JOBS_IM_TLS_CA:-}" item
    for item in "$cert" "$key" "$ca"; do
        [[ -n "$item" && -f "$item" && ! -L "$item" ]] || { print -u2 '服务器模式需JOBS_IM_TLS_CERT/JOBS_IM_TLS_KEY/JOBS_IM_TLS_CA指定真实证书及匹配私钥/信任链。'; return 2; }
    done
    "$OPENSSL" verify -x509_strict -purpose sslserver -verify_hostname "$PUBLIC_HOST" -CAfile "$ca" -untrusted "$cert" "$cert" || return $?
    [[ "$("$OPENSSL" x509 -in "$cert" -pubkey -noout | "$OPENSSL" pkey -pubin -outform DER | shasum -a 256)" == "$("$OPENSSL" pkey -in "$key" -pubout -outform DER | shasum -a 256)" ]] || { print -u2 '服务器证书与私钥不匹配。'; return 2; }
    if [[ -n "$(ls -A "$STATE/TLS")" ]]; then
        cmp -s "$cert" "$STATE/TLS/server.crt" && cmp -s "$key" "$STATE/TLS/server.key" && cmp -s "$ca" "$STATE/TLS/ca.crt" || { print -u2 '服务器TLS与既有实例不同；原信任和密钥保留，需单独轮换。'; return 2; }
    else
        cp -- "$cert" "$STATE/TLS/server.crt"
        cp -- "$key" "$STATE/TLS/server.key"
        cp -- "$ca" "$STATE/TLS/ca.crt"
        chmod 600 "$STATE/TLS"/*
    fi
}
# 地址变化或退出让本轮离线二维码失效，不再把旧网址当有效入口。
expire_im_qr() {
    if [[ -n "${QR_PAGE:-}" && -x "${QR_BINARY:-}" ]]; then
        "$QR_BINARY" --expire --output-dir "${QR_PAGE:h}" >/dev/null 2>&1 || print -u2 '二维码失效标记失败；勿继续使用本轮旧码。'
    fi
    QR_PAGE=""
}
# 每个后台菜单先登记身份，清理时不误停被复用的PID。
start_im_browser_menu() {
    local menu_file="$RUN_DIR/menu.tsv"
    printf 'local\t本机用户端\n' > "$menu_file"
    [[ -z "$PUBLIC_URL" ]] || printf 'public\t公网用户端\n' >> "$menu_file"
    [[ -z "$QR_PAGE" ]] || printf 'qr\t打开离线扫码页面\n' >> "$menu_file"
    printf 'pause\t收起菜单（服务继续，回车重开）\n' >> "$menu_file"
    FZF_DEFAULT_OPTS='' FZF_DEFAULT_OPTS_FILE=/dev/null FZF_DEFAULT_COMMAND='' "$FZF_EXECUTABLE" --no-sort --no-multi --delimiter=$'\t' --with-nth=2 --height=40% --border --bind='up:up,down:down' --prompt='打开网页 > ' --header='回车交给浏览器打开；Esc收起菜单；Ctrl+C停止本轮服务。' < "$menu_file" > "$RUN_DIR/menu-result" &
    BROWSER_MENU_PID=$!
    BROWSER_MENU_IDENTITY="$(LC_ALL=C ps -p "$BROWSER_MENU_PID" -o lstart= -o command= 2>/dev/null || true)"
}
# 菜单与实际服务一起监控，浏览器失败或收起不丢失服务生命周期。
monitor_im_session() {
    local paused=0 input code result
    while kill -0 "$SERVER_PID" 2>/dev/null; do
        if [[ -n "$TUNNEL_PID" ]] && ! kill -0 "$TUNNEL_PID" 2>/dev/null; then print -u2 '本轮穿透已结束，停止本轮服务。'; return 2; fi
        if [[ -n "$BROWSER_MENU_PID" ]]; then
            if ! kill -0 "$BROWSER_MENU_PID" 2>/dev/null; then
                code=0; wait "$BROWSER_MENU_PID" || code=$?
                BROWSER_MENU_PID=""
                result="$(cat "$RUN_DIR/menu-result")"
                if (( code != 0 )) || [[ -z "$result" ]]; then paused=1
                else
                    case "${result%%$'\t'*}" in
                        local) open "https://127.0.0.1:$HTTPS_PORT/" || print '浏览器打开失败，可再次选择。' ;;
                        public) open "$PUBLIC_URL/" || print '浏览器打开失败，可再次选择。' ;;
                        qr) open "$QR_PAGE" || print '扫码页打开失败，可再次选择。' ;;
                        pause) paused=1 ;;
                        *) paused=1 ;;
                    esac
                fi
            fi
        elif [[ -t 0 && $paused == 0 ]]; then start_im_browser_menu
        elif [[ -t 0 ]] && IFS= read -r -t .1 input; then paused=0
        fi
        sleep .2
    done
    wait "$SERVER_PID"
}
# 回收菜单仅按本轮完整启动身份，不按名称或终端全局查杀。
stop_browser_menu() {
    if [[ -n "${BROWSER_MENU_PID:-}" && -n "${BROWSER_MENU_IDENTITY:-}" && "$(LC_ALL=C ps -p "$BROWSER_MENU_PID" -o lstart= -o command= 2>/dev/null || true)" == "$BROWSER_MENU_IDENTITY" ]]; then
        kill -TERM "$BROWSER_MENU_PID" 2>/dev/null || true
        wait "$BROWSER_MENU_PID" 2>/dev/null || true
    fi
    BROWSER_MENU_PID=""
}
# 独立守护检测入口/原终端父进程异常退出，复用相同身份核验停止本轮资源。
start_session_guardian() {
    local controller="$$" controller_identity parent="$PPID" parent_identity
    controller_identity="$(LC_ALL=C ps -p "$controller" -o lstart= -o command=)"
    parent_identity="$(LC_ALL=C ps -p "$parent" -o lstart= -o command=)"
    (
        TRAPEXIT() { return 0; }
        TRAPZERR() { return 0; }
        trap '' HUP INT TERM
        LOCK_HELD=0
        while [[ "$(LC_ALL=C ps -p "$controller" -o lstart= -o command= 2>/dev/null || true)" == "$controller_identity" && "$(LC_ALL=C ps -p "$parent" -o lstart= -o command= 2>/dev/null || true)" == "$parent_identity" ]]; do sleep .5; done
        CLEANING=0
        cleanup
    ) &
    GUARDIAN_PID=$!
}
