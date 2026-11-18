# 穿透失败或晚退时撤销公网展示，严格恢复同一本轮Go的本机Origin，不重建数据库。
recover_private_after_tunnel() {
    stop_browser_menu
    expire_im_qr
    stop_tunnel || { print -u2 '无法证明原隧道已停，拒绝宣称已恢复私有；保留现场。'; return 2; }
    TUNNEL_PID=""; PUBLIC_URL=""; PUBLIC_HOST=127.0.0.1
    JOBS_IM_PUBLIC_URL="https://127.0.0.1:$HTTPS_PORT"
    stop_backend || return $?
    launch_im_backend "$JOBS_IM_PUBLIC_URL" || return $?
    write_connection || return $?
    prepare_im_qr
    print '穿透当前不可用；已严格恢复本机HTTPS，原数据和本地服务继续。菜单可重试穿透、查看诊断或停止。'
}
# 只有用户选择公开后才准备cloudflared，工具/网络失败均保留已健康本机服务。
attempt_public_access() {
    local reason="${1:-initial}" answer=""
    if [[ "$reason" == retry ]]; then
        print '重试将公开本轮JobsIM用户入口；仅使用合成资料。'
        printf '按回车同意公开并重试；其它输入或输入结束取消：'
        IFS= read -r answer || return 0
        [[ -z "$answer" ]] || return 0
    fi
    if [[ -z "${CLOUDFLARED_EXECUTABLE:-}" ]]; then
        if (
            CLEANING=1; RUN_ACTIVE=0; LOCK_HELD=0
            resolve_latest_tool cloudflared || exit $?
            printf '%s\n' "$CLOUDFLARED_EXECUTABLE" > "$RUN_DIR/cloudflared-selected"
        ); then
            CLOUDFLARED_EXECUTABLE="$(cat "$RUN_DIR/cloudflared-selected")"
        else
            print '穿透工具准备未完成；本机服务保持，菜单可查看诊断或再次重试。'
            LOCAL_ACCESS=private
            return 0
        fi
    fi
    LOCAL_ACCESS=quick
    if start_im_tunnel; then
        write_connection || return $?
        expire_im_qr; prepare_im_qr
        print '本轮公网HTTPS健康通过；完整外网业务仍需对应场景验收。'
    else
        recover_private_after_tunnel || return $?
    fi
}
