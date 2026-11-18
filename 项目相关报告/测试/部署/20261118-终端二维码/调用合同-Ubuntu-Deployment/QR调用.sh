#!/bin/bash
set -eu
print(){ printf '%s\n' "$*"; }
REPO="$1";GO_TOOL="$2";RUN_DIR="$3";PUBLIC_URL=https://127.0.0.1:19443
prepare_access_qr() {
    QR_BINARY="$RUN_DIR/access-qr"; QR_DIR="$RUN_DIR/AccessQR"
    if ! (cd "$REPO/部署/公共组件/地址二维码" && GOROOT= GOWORK=off GOTOOLCHAIN=local "$GO_TOOL" build -mod=readonly -o "$QR_BINARY" .); then print '二维码构建失败，仍提供完整URL。'; QR_BINARY=''; return 0; fi
    local width=0 height=0 size label=用户端
    local -a qr_options=()
    if [[ -t 1 ]]; then
        size="$(stty size < /dev/tty 2>/dev/null || true)"
        if [[ "$size" =~ ^[0-9]+[[:space:]]+[0-9]+$ ]]; then read -r height width <<< "$size"; fi
        if (( width == 0 )); then width="${COLUMNS:-0}"; [[ "$width" =~ ^[0-9]+$ ]] || width=0; fi
        if [[ -n "${TERM:-}" && "${TERM:-dumb}" != dumb && -z "${NO_COLOR+x}" ]]; then qr_options+=(--ansi); fi
    fi
    [[ "$PUBLIC_URL" != https://127.0.0.1:* && "$PUBLIC_URL" != https://localhost:* ]] || label='仅本机，手机不可用'
    local display="$RUN_DIR/qr-display.tmp"
    "$QR_BINARY" --output-dir "$QR_DIR" --title JobsIM --entry "$label|$PUBLIC_URL/" --terminal-width "$width" --terminal-height "$height" "${qr_options[@]}" > "$display" 2>&1 || { print '二维码生成失败，保留可复制URL。'; return 0; }
    sed -E $'s/\033\\[[0-9;]*[[:alpha:]]//g' "$display" > "$RUN_DIR/qr-output.txt"
    if [[ -t 1 ]]; then cat "$display"; else cat "$RUN_DIR/qr-output.txt"; fi
    rm -- "$display"
    QR_PAGE="$QR_DIR/index.html"
    if [[ -t 1 ]] && { (( ${#qr_options[@]} == 0 )) || ! grep -Fq '终端二维码（' "$RUN_DIR/qr-output.txt"; }; then
        if [[ -n "${DISPLAY:-}${WAYLAND_DISPLAY:-}" ]] && command -v xdg-open >/dev/null && xdg-open "$QR_DIR/01.png" >/dev/null 2>&1; then
            print '已打开本轮本地黑白二维码图片；回环地址仅本机可用。'
        else print '终端无法完整或稳定呈现二维码且本机图形预览不可用；二维码图片与完整URL保留，不宣称已显示可扫码图。'; fi
    fi
}
prepare_access_qr
