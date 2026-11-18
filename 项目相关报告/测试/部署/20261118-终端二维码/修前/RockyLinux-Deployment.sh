#!/bin/bash
# shell: bash
# Created by Jobs on 2026年11月14日，星期六.
# >>> Jobs terminal style protection v1 >>>
# 临时保护当前终端配色，业务进程自行管理原有退出清理。
jobs_terminal_style_run() {
    # 无法可靠保存原色时，沿用脚本已有的纯文本约定。
    jobs_terminal_style_plain() {
        PLAIN_OUTPUT=1
        NO_COLOR=1
        FORCE_COLOR=0
        CLICOLOR=0
        ANSI_COLORS_DISABLED=1
        npm_config_color=false
        export PLAIN_OUTPUT NO_COLOR FORCE_COLOR CLICOLOR ANSI_COLORS_DISABLED npm_config_color
        if "$@"; then
            jobs_style_exit_status=0
        else
            jobs_style_exit_status=$?
        fi
        exit "$jobs_style_exit_status"
    }
    if [ ! -t 0 ] || [ ! -t 1 ] || [ -z "${TERM:-}" ] ||
       [ "${TERM:-}" = dumb ] || [ "${NO_COLOR+x}" = x ] ||
       [ "${PLAIN_OUTPUT:-0}" = 1 ] || [ "${IS_SOURCETREE_RUNTIME:-0}" = 1 ] ||
       env | LC_ALL=C grep -qiE '^(SOURCETREE|SOURCE_TREE)'; then
        if "$@"; then
            jobs_style_exit_status=0
        else
            jobs_style_exit_status=$?
        fi
        exit "$jobs_style_exit_status"
    fi
    if [ -n "${TMUX:-}${STY:-}" ]; then
        jobs_terminal_style_plain "$@"
    fi
    jobs_style_tty=$(tty 2>/dev/null) || jobs_terminal_style_plain "$@"
    if ! ps -o pgid= -o tpgid= -p "$$" 2>/dev/null |
         LC_ALL=C awk 'NF == 2 && $1 == $2 && $1 > 0 { foreground = 1 } END { exit !foreground }'; then
        jobs_terminal_style_plain "$@"
    fi
    case ${JOBS_TERMINAL_STYLE_OWNER:-} in
        ''|*[!0-9]*) ;;
        *)
            if [ "${JOBS_TERMINAL_STYLE_TTY:-}" = "$jobs_style_tty" ] &&
               kill -0 "$JOBS_TERMINAL_STYLE_OWNER" 2>/dev/null; then
                if "$@"; then
                    jobs_style_exit_status=0
                else
                    jobs_style_exit_status=$?
                fi
                exit "$jobs_style_exit_status"
            fi
            ;;
    esac
    jobs_style_stty=$(stty -g 2>/dev/null < /dev/tty) || jobs_terminal_style_plain "$@"
    jobs_style_saved=''
    jobs_style_saved_rows=''
    jobs_style_original_rows=''
    jobs_style_applied_rows=''
    jobs_style_changed=0
    jobs_style_applied_complete=0
    jobs_style_query_active=0
    jobs_style_recovery=''
    jobs_style_exit_status=0
    jobs_style_esc=$(printf '\033')
    jobs_style_bel=$(printf '\007')
    # 完整颜色帧有界重发；设置核验与退出恢复容忍帧外回车和明确方向键。
    jobs_terminal_style_query() {
        jobs_style_query_key=$1
        jobs_style_reply=''
        jobs_style_query_buffer=''
        jobs_style_query_target_reply=''
        jobs_style_query_target_normalized=''
        jobs_style_query_target_rgb=''
        jobs_style_query_st="${jobs_style_esc}\\"
        jobs_style_query_stale_count=0
        jobs_style_query_phase=${2:-strict}
        jobs_style_query_cr=$(printf '\r')
        jobs_style_query_lf=$(printf '\n.')
        jobs_style_query_lf=${jobs_style_query_lf%.}
        jobs_style_query_failure='未收到目标颜色的完整答复（最多 3 次查询，每次约 2 秒）'
        for jobs_style_query_round in 1 2 3; do
            printf '\033]%s;?\007' "$jobs_style_query_key" > /dev/tty || {
                jobs_style_query_failure='无法向原终端发送配色查询'
                return 1
            }
            for jobs_style_attempt in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
                # 哨兵保留末尾换行，不能把只有回车的键盘输入误当成无答复。
                jobs_style_piece=$(dd bs=256 count=1 2>/dev/null < /dev/tty; printf '.')
                jobs_style_piece=${jobs_style_piece%.}
                jobs_style_query_buffer=$jobs_style_query_buffer$jobs_style_piece
                while [ -n "$jobs_style_query_buffer" ]; do
                    # 设置核验与退出恢复阶段允许帧外的明确控制键，普通文字仍然失败。
                    if [ "$jobs_style_query_phase" = cleanup ] || [ "$jobs_style_query_phase" = applied ]; then
                        jobs_style_query_control_sequence=''
                        jobs_style_query_control_partial=0
                        case $jobs_style_query_buffer in
                            "$jobs_style_query_cr"*) jobs_style_query_control_sequence=$jobs_style_query_cr ;;
                            "$jobs_style_query_lf"*) jobs_style_query_control_sequence=$jobs_style_query_lf ;;
                        esac
                        if [ -z "$jobs_style_query_control_sequence" ]; then
                            for jobs_style_query_direction in A B C D; do
                                for jobs_style_query_control_candidate in \
                                    "${jobs_style_esc}[${jobs_style_query_direction}" \
                                    "${jobs_style_esc}O${jobs_style_query_direction}" \
                                    "${jobs_style_esc}[1;2${jobs_style_query_direction}" \
                                    "${jobs_style_esc}[1;3${jobs_style_query_direction}" \
                                    "${jobs_style_esc}[1;4${jobs_style_query_direction}" \
                                    "${jobs_style_esc}[1;5${jobs_style_query_direction}" \
                                    "${jobs_style_esc}[1;6${jobs_style_query_direction}" \
                                    "${jobs_style_esc}[1;7${jobs_style_query_direction}" \
                                    "${jobs_style_esc}[1;8${jobs_style_query_direction}"; do
                                    case $jobs_style_query_buffer in
                                        "$jobs_style_query_control_candidate"*)
                                            jobs_style_query_control_sequence=$jobs_style_query_control_candidate
                                            break 2
                                            ;;
                                    esac
                                    case $jobs_style_query_control_candidate in
                                        "$jobs_style_query_buffer"*) jobs_style_query_control_partial=1 ;;
                                    esac
                                done
                            done
                        fi
                        if [ -n "$jobs_style_query_control_sequence" ]; then
                            jobs_style_phase_control_keys=$(( ${jobs_style_phase_control_keys:-0} + 1 ))
                            if [ "$jobs_style_phase_control_keys" -gt 64 ]; then
                                jobs_style_query_failure='本阶段已识别控制键超过 64 个，已停止自动恢复'
                                return 1
                            fi
                            jobs_style_query_buffer=${jobs_style_query_buffer#"$jobs_style_query_control_sequence"}
                            continue
                        fi
                        if [ "$jobs_style_query_control_partial" = 1 ]; then
                            break
                        fi
                    fi
                    case $jobs_style_query_buffer in
                        "$jobs_style_esc"|"$jobs_style_esc]") break ;;
                        "$jobs_style_esc]"*) ;;
                        *) jobs_style_query_failure='收到非配色查询输入，已停止读取'; return 1 ;;
                    esac
                    jobs_style_query_head=''
                    jobs_style_query_terminator=''
                    case $jobs_style_query_buffer in
                        *"$jobs_style_bel"*)
                            jobs_style_query_head=${jobs_style_query_buffer%%"$jobs_style_bel"*}
                            jobs_style_query_terminator=$jobs_style_bel
                            ;;
                    esac
                    case $jobs_style_query_buffer in
                        *"$jobs_style_query_st"*)
                            jobs_style_query_st_head=${jobs_style_query_buffer%%"$jobs_style_query_st"*}
                            if [ -z "$jobs_style_query_terminator" ] ||
                               [ "${#jobs_style_query_st_head}" -lt "${#jobs_style_query_head}" ]; then
                                jobs_style_query_head=$jobs_style_query_st_head
                                jobs_style_query_terminator=$jobs_style_query_st
                            fi
                            ;;
                    esac
                    [ -n "$jobs_style_query_terminator" ] || break
                    jobs_style_query_frame=$jobs_style_query_head$jobs_style_query_terminator
                    jobs_style_query_buffer=${jobs_style_query_buffer#"$jobs_style_query_frame"}
                    jobs_style_query_body=${jobs_style_query_head#"$jobs_style_esc]"}
                    case $jobs_style_query_body in
                        *';rgb:'*) ;;
                        *) jobs_style_query_failure='配色答复格式或 RGB 数据无效'; return 1 ;;
                    esac
                    jobs_style_query_frame_key=${jobs_style_query_body%%';rgb:'*}
                    case $jobs_style_query_frame_key in
                        10|11|12|'4;'[0-9]|'4;1'[0-5]) ;;
                        *) jobs_style_query_failure='收到不属于本次颜色协议的答复'; return 1 ;;
                    esac
                    jobs_style_query_frame_rgb=${jobs_style_query_body#"$jobs_style_query_frame_key;rgb:"}
                    jobs_style_query_frame_normalized=$(printf '%s\n' "$jobs_style_query_frame_rgb" | LC_ALL=C awk -F/ '
                        BEGIN { valid = 1 }
                        NR != 1 || NF != 3 { valid = 0; exit }
                        {
                            for (i = 1; i <= 3; i++) {
                                h = tolower($i)
                                if (length(h) < 1 || length(h) > 4 || h ~ /[^0-9a-f]/) {
                                    valid = 0
                                    exit
                                }
                                n = 0
                                for (j = 1; j <= length(h); j++) n = n * 16 + index("0123456789abcdef", substr(h, j, 1)) - 1
                                value[i] = int(n * 65535 / (16 ^ length(h) - 1) + 0.5)
                            }
                        }
                        END {
                            if (!valid || NR != 1) exit 1
                            printf "%d/%d/%d", value[1], value[2], value[3]
                        }
                    ')
                    if [ -z "$jobs_style_query_frame_normalized" ]; then
                        jobs_style_query_failure='配色答复格式或 RGB 数据无效'
                        return 1
                    fi
                    if [ "$jobs_style_query_frame_key" = "$jobs_style_query_key" ]; then
                        if [ -n "$jobs_style_query_target_reply" ] &&
                           [ "$jobs_style_query_target_normalized" != "$jobs_style_query_frame_normalized" ]; then
                            jobs_style_query_failure='同一颜色的重复答复不一致，已停止自动恢复'
                            return 1
                        fi
                        jobs_style_query_target_reply=$jobs_style_query_frame
                        jobs_style_query_target_normalized=$jobs_style_query_frame_normalized
                        jobs_style_query_target_rgb=$jobs_style_query_frame_rgb
                    else
                        jobs_style_query_stale_count=$((jobs_style_query_stale_count + 1))
                        if [ "$jobs_style_query_stale_count" -gt 128 ]; then
                            jobs_style_query_failure='收到过多旧颜色答复，已停止读取'
                            return 1
                        fi
                    fi
                done
                if [ -n "$jobs_style_query_target_reply" ] && [ -z "$jobs_style_query_buffer" ]; then
                    jobs_style_reply=$jobs_style_query_target_reply
                    jobs_style_normalized=$jobs_style_query_target_normalized
                    jobs_style_rgb=$jobs_style_query_target_rgb
                    return 0
                fi
            done
            # 残缺帧不能通过重发拼成另一个答复，更不能被当成键盘空输入。
            if [ -n "$jobs_style_query_buffer" ]; then
                jobs_style_query_failure='配色答复帧不完整，已停止自动恢复'
                return 1
            fi
        done
        return 1
    }
    # 查询可用不代表终端允许改色，按目标值核对实际生效结果。
    jobs_terminal_style_accepts_target() {
        jobs_style_requested_hex=''
        case $1 in
            10|12) jobs_style_requested_hex=ffffff ;;
            11) jobs_style_requested_hex=000000 ;;
            '4;'*)
                jobs_style_requested_index=0
                for jobs_style_requested_color in 000000 ff5f56 5fd787 ffd75f 5fafff d787ff 5fd7ff eeeeee 9e9e9e ff8787 87ffaf ffff87 87bfff ffafff afffff ffffff; do
                    if [ "4;$jobs_style_requested_index" = "$1" ]; then
                        jobs_style_requested_hex=$jobs_style_requested_color
                        break
                    fi
                    jobs_style_requested_index=$((jobs_style_requested_index + 1))
                done
                ;;
        esac
        [ -n "$jobs_style_requested_hex" ] || return 1
        printf '%s|%s\n' "$jobs_style_normalized" "$jobs_style_requested_hex" | LC_ALL=C awk -F'|' '
            function hexbyte(h) {
                return (index("0123456789abcdef", substr(h, 1, 1)) - 1) * 16 + index("0123456789abcdef", substr(h, 2, 1)) - 1
            }
            {
                if (split($1, actual, "/") != 3 || length($2) != 6) exit 1
                for (i = 1; i <= 3; i++) {
                    difference = actual[i] - hexbyte(substr($2, i * 2 - 1, 2)) * 257
                    if (difference < -255 || difference > 255) exit 1
                }
            }
        '
    }
    # 核验中断时重新读取当前值，只撤回仍等于本轮目标值的颜色。
    jobs_terminal_style_rollback() {
        jobs_style_rollback_issue='无法读取或设置原终端输入模式'
        jobs_style_rollback_safe=''
        jobs_style_rollback_conflicts=''
        jobs_style_phase_control_keys=0
        jobs_style_stty=$(stty -g 2>/dev/null < /dev/tty) || jobs_style_stty=''
        if [ -n "$jobs_style_stty" ] && stty -echo -icanon min 0 time 1 2>/dev/null < /dev/tty; then
            jobs_style_query_active=1
            jobs_style_rollback_ok=1
            while IFS='|' read -r jobs_style_key jobs_style_original_normalized jobs_style_original; do
                [ -n "$jobs_style_key" ] || continue
                if ! jobs_terminal_style_query "$jobs_style_key" cleanup; then
                    jobs_style_rollback_ok=0
                    jobs_style_rollback_issue="回退颜色项 $jobs_style_key：$jobs_style_query_failure"
                    break
                fi
                if [ "$jobs_style_normalized" = "$jobs_style_original_normalized" ]; then
                    continue
                elif jobs_terminal_style_accepts_target "$jobs_style_key"; then
                    jobs_style_rollback_safe=$jobs_style_rollback_safe$jobs_style_original
                else
                    jobs_style_rollback_conflicts="$jobs_style_rollback_conflicts $jobs_style_key"
                fi
            done <<JOBS_STYLE_ROLLBACK
$jobs_style_original_rows
JOBS_STYLE_ROLLBACK
            stty "$jobs_style_stty" 2>/dev/null < /dev/tty || { jobs_style_rollback_ok=0; jobs_style_rollback_issue='无法恢复原终端输入模式'; }
            jobs_style_query_active=0
            if [ "$jobs_style_rollback_ok" = 1 ] && printf '%s\033[0m' "$jobs_style_rollback_safe" > /dev/tty; then
                if [ -z "$jobs_style_rollback_conflicts" ]; then
                    rm -f "$jobs_style_recovery"
                    rmdir "${jobs_style_recovery%/*}" 2>/dev/null || :
                    return 0
                fi
                printf '检测到运行期间的配色变更，已保留这些项：%s\n' "$jobs_style_rollback_conflicts" >&2
                jobs_style_rollback_issue="核验中断后检测到其它配色变更：$jobs_style_rollback_conflicts"
            fi
        fi
        printf '终端配色未能安全自动恢复（%s）；确认覆盖当前配色后可运行：sh "%s"\n' "$jobs_style_rollback_issue" "$jobs_style_recovery" >&2
        return 1
    }
    # 检测到后续改色时只恢复仍由本次保护持有的项，保留独立恢复入口。
    jobs_terminal_style_cleanup() {
        jobs_style_result=$1
        trap '' HUP INT TERM
        if [ "$jobs_style_query_active" = 1 ]; then
            stty "$jobs_style_stty" 2>/dev/null < /dev/tty || :
            jobs_style_query_active=0
        fi
        if [ "$jobs_style_changed" = 1 ] && [ "$jobs_style_applied_complete" != 1 ]; then
            if jobs_terminal_style_rollback; then
                jobs_style_changed=0
            else
                [ "$jobs_style_result" -ne 0 ] || jobs_style_result=1
            fi
            if [ "${2:-}" = continue ]; then
                jobs_style_cleanup_status=$jobs_style_result
                return 0
            fi
            exit "$jobs_style_result"
        fi
        if [ "$jobs_style_changed" = 1 ]; then
            jobs_style_cleanup_ok=0
            jobs_style_cleanup_issue='无法读取或设置原终端输入模式，或临时配色未完成核验'
            jobs_style_conflicts=''
            jobs_style_restore_safe=''
            jobs_style_phase_control_keys=0
            if [ "$jobs_style_applied_complete" = 1 ]; then
                jobs_style_stty=$(stty -g 2>/dev/null < /dev/tty) || jobs_style_stty=''
                if [ -n "$jobs_style_stty" ] && stty -echo -icanon min 0 time 1 2>/dev/null < /dev/tty; then
                    jobs_style_query_active=1
                    jobs_style_cleanup_ok=1
                    while IFS='|' read -r jobs_style_key jobs_style_expected jobs_style_original; do
                        [ -n "$jobs_style_key" ] || continue
                        if ! jobs_terminal_style_query "$jobs_style_key" cleanup; then
                            jobs_style_cleanup_ok=0
                            jobs_style_cleanup_issue="颜色项 $jobs_style_key：$jobs_style_query_failure"
                            break
                        fi
                        if [ "$jobs_style_normalized" = "$jobs_style_expected" ]; then
                            jobs_style_restore_safe=$jobs_style_restore_safe$jobs_style_original
                        else
                            jobs_style_conflicts="$jobs_style_conflicts $jobs_style_key"
                        fi
                    done <<JOBS_STYLE_APPLIED
$jobs_style_applied_rows
JOBS_STYLE_APPLIED
                    stty "$jobs_style_stty" 2>/dev/null < /dev/tty || { jobs_style_cleanup_ok=0; jobs_style_cleanup_issue='无法恢复原终端输入模式'; }
                    jobs_style_query_active=0
                fi
            fi
            if [ "$jobs_style_cleanup_ok" = 1 ] && printf '%s\033[0m' "$jobs_style_restore_safe" > /dev/tty; then
                if [ -z "$jobs_style_conflicts" ]; then
                    rm -f "$jobs_style_recovery"
                    rmdir "${jobs_style_recovery%/*}" 2>/dev/null || :
                else
                    printf '检测到运行期间的配色变更，已保留这些项：%s\n' "$jobs_style_conflicts" >&2
                    printf '确认要覆盖后续改色时才使用恢复入口：sh "%s"\n' "$jobs_style_recovery" >&2
                    [ "$jobs_style_result" -ne 0 ] || jobs_style_result=1
                fi
            else
                printf '终端配色未能安全自动恢复（%s）；确认覆盖当前配色后可运行：sh "%s"\n' "$jobs_style_cleanup_issue" "$jobs_style_recovery" >&2
                [ "$jobs_style_result" -ne 0 ] || jobs_style_result=1
            fi
        fi
        if [ "${2:-}" = continue ]; then
            jobs_style_cleanup_status=$jobs_style_result
            return 0
        fi
        exit "$jobs_style_result"
    }
    trap 'jobs_terminal_style_cleanup "$jobs_style_exit_status"' 0
    trap 'jobs_style_exit_status=129; jobs_terminal_style_cleanup 129 continue; jobs_style_changed=0; exit 129' HUP
    trap 'jobs_style_exit_status=130; jobs_terminal_style_cleanup 130 continue; jobs_style_changed=0; exit 130' INT
    trap 'jobs_style_exit_status=143; jobs_terminal_style_cleanup 143 continue; jobs_style_changed=0; exit 143' TERM
    if ! stty -echo -icanon min 0 time 1 2>/dev/null < /dev/tty; then
        jobs_terminal_style_plain "$@"
    fi
    jobs_style_query_active=1
    jobs_style_probe_ok=1
    for jobs_style_key in 10 11 12 '4;0' '4;1' '4;2' '4;3' '4;4' '4;5' '4;6' '4;7' '4;8' '4;9' '4;10' '4;11' '4;12' '4;13' '4;14' '4;15'; do
        if ! jobs_terminal_style_query "$jobs_style_key"; then
            jobs_style_probe_ok=0
            break
        fi
        jobs_style_saved=$jobs_style_saved$jobs_style_reply
        jobs_style_original_rows="$jobs_style_original_rows$jobs_style_key|$jobs_style_normalized|$jobs_style_reply
"
        jobs_style_saved_rows="$jobs_style_saved_rows$jobs_style_key|$jobs_style_reply
"
    done
    stty "$jobs_style_stty" 2>/dev/null < /dev/tty || { jobs_style_exit_status=1; exit 1; }
    jobs_style_query_active=0
    if [ "$jobs_style_probe_ok" != 1 ]; then
        printf '%s\n' '终端不支持完整配色保存，或启动时收到其它输入；保留原配色并使用纯文本。' >&2
        jobs_terminal_style_plain "$@"
    fi
    jobs_style_recovery_dir=$(umask 077; mktemp -d "${TMPDIR:-/tmp}/jobs-terminal-style.XXXXXX") || jobs_terminal_style_plain "$@"
    jobs_style_recovery=$jobs_style_recovery_dir/restore.sh
    if ! (umask 077; {
        printf '#!/bin/sh\n'
        printf '[ -t 0 ] && [ "$(tty)" = '\''%s'\'' ] || { printf '\''请在原终端运行恢复入口。\\n'\'' >&2; exit 1; }\n' "$jobs_style_tty"
        printf 'printf '\''本操作会覆盖此终端当前配色。确认恢复脚本启动前的配色，请输入 YES：'\'' >&2\n'
        printf 'IFS= read -r jobs_restore_choice\n[ "$jobs_restore_choice" = YES ] || exit 1\n'
        printf 'printf '\''%%s\\033[0m'\'' '\''%s'\'' > /dev/tty\n' "$jobs_style_saved"
    } > "$jobs_style_recovery"); then
        printf '%s\n' '无法保存终端配色恢复入口；保留原配色并使用纯文本。' >&2
        jobs_terminal_style_plain "$@"
    fi
    printf '正在准备本次终端临时配色；强退后恢复入口：sh "%s"\n' "$jobs_style_recovery" >&2
    jobs_style_changed=1
    printf '\033]10;#ffffff\007\033]11;#000000\007\033]12;#ffffff\007' > /dev/tty
    jobs_style_index=0
    for jobs_style_color in 000000 ff5f56 5fd787 ffd75f 5fafff d787ff 5fd7ff eeeeee 9e9e9e ff8787 87ffaf ffff87 87bfff ffafff afffff ffffff; do
        printf '\033]4;%s;#%s\007' "$jobs_style_index" "$jobs_style_color" > /dev/tty
        jobs_style_index=$((jobs_style_index + 1))
    done
    printf '\033[0m' > /dev/tty
    jobs_style_stty=$(stty -g 2>/dev/null < /dev/tty) || { jobs_style_exit_status=1; exit 1; }
    stty -echo -icanon min 0 time 1 2>/dev/null < /dev/tty || { jobs_style_exit_status=1; exit 1; }
    jobs_style_query_active=1
    jobs_style_probe_ok=1
    jobs_style_targets_ok=1
    jobs_style_phase_control_keys=0
    while IFS='|' read -r jobs_style_key jobs_style_original; do
        [ -n "$jobs_style_key" ] || continue
        if ! jobs_terminal_style_query "$jobs_style_key" applied; then
            jobs_style_probe_ok=0
            break
        fi
        jobs_style_applied_rows="$jobs_style_applied_rows$jobs_style_key|$jobs_style_normalized|$jobs_style_original
"
        if ! jobs_terminal_style_accepts_target "$jobs_style_key"; then
            jobs_style_targets_ok=0
        fi
    done <<JOBS_STYLE_ORIGINAL
$jobs_style_saved_rows
JOBS_STYLE_ORIGINAL
    stty "$jobs_style_stty" 2>/dev/null < /dev/tty || { jobs_style_exit_status=1; exit 1; }
    jobs_style_query_active=0
    if [ "$jobs_style_probe_ok" != 1 ]; then
        printf '临时配色核验失败（颜色项 %s：%s），正在核对原色恢复。\n' "$jobs_style_key" "$jobs_style_query_failure" >&2
        jobs_terminal_style_cleanup 0 continue
        jobs_style_changed=0
        if [ "$jobs_style_cleanup_status" != 0 ]; then
            jobs_style_exit_status=1
            exit 1
        fi
        trap 'jobs_style_exit_status=129; jobs_terminal_style_cleanup 129 continue; jobs_style_changed=0; exit 129' HUP
        trap 'jobs_style_exit_status=130; jobs_terminal_style_cleanup 130 continue; jobs_style_changed=0; exit 130' INT
        trap 'jobs_style_exit_status=143; jobs_terminal_style_cleanup 143 continue; jobs_style_changed=0; exit 143' TERM
        printf '%s\n' '已恢复原配色并使用纯文本。' >&2
        jobs_terminal_style_plain "$@"
    fi
    if [ "$jobs_style_targets_ok" != 1 ]; then
        jobs_terminal_style_cleanup 0 continue
        jobs_style_changed=0
        if [ "$jobs_style_cleanup_status" != 0 ]; then
            jobs_style_exit_status=1
            exit 1
        fi
        trap 'jobs_style_exit_status=129; jobs_terminal_style_cleanup 129 continue; jobs_style_changed=0; exit 129' HUP
        trap 'jobs_style_exit_status=130; jobs_terminal_style_cleanup 130 continue; jobs_style_changed=0; exit 130' INT
        trap 'jobs_style_exit_status=143; jobs_terminal_style_cleanup 143 continue; jobs_style_changed=0; exit 143' TERM
        printf '%s\n' '终端未接受本次配色，已恢复原配色并使用纯文本。' >&2
        jobs_terminal_style_plain "$@"
    fi
    jobs_style_applied_complete=1
    printf '%s\n' '本次终端已临时使用黑底白字。' >&2
    JOBS_TERMINAL_STYLE_TTY=$jobs_style_tty
    JOBS_TERMINAL_STYLE_OWNER=$$
    export JOBS_TERMINAL_STYLE_TTY JOBS_TERMINAL_STYLE_OWNER
    if "$@"; then
        jobs_style_exit_status=0
    else
        jobs_style_exit_status=$?
    fi
    exit "$jobs_style_exit_status"
}
# 仅监督直接执行的入口；被 source 时不切换配色、不重新执行业务。
jobs_terminal_style_bootstrap() {
    jobs_style_bootstrap_script=$1
    shift
    if [ -n "${ZSH_VERSION:-}" ]; then
        case ${ZSH_EVAL_CONTEXT:-} in
            *:file|*:file:*) return 0 ;;
        esac
    elif [ -n "${BASH_VERSION:-}" ] && [ "${BASH_SOURCE:-$jobs_style_bootstrap_script}" != "$jobs_style_bootstrap_script" ]; then
        return 0
    fi
    if [ "${JOBS_TERMINAL_STYLE_REEXEC:-}" = "$jobs_style_bootstrap_script" ]; then
        unset JOBS_TERMINAL_STYLE_REEXEC
        return 0
    fi
    if [ ! -t 0 ] || [ ! -t 1 ] || [ -z "${TERM:-}" ] ||
       [ "${TERM:-}" = dumb ] || [ "${NO_COLOR+x}" = x ] ||
       [ "${PLAIN_OUTPUT:-0}" = 1 ] || [ "${IS_SOURCETREE_RUNTIME:-0}" = 1 ] ||
       [ -n "${TMUX:-}${STY:-}" ] ||
       env | LC_ALL=C grep -qiE '^(SOURCETREE|SOURCE_TREE)'; then
        return 0
    fi
    case ${JOBS_TERMINAL_STYLE_OWNER:-} in
        ''|*[!0-9]*) ;;
        *)
            if kill -0 "$JOBS_TERMINAL_STYLE_OWNER" 2>/dev/null &&
               [ "${JOBS_TERMINAL_STYLE_TTY:-}" = "$(tty 2>/dev/null)" ]; then
                return 0
            fi
            ;;
    esac
    JOBS_TERMINAL_STYLE_REEXEC=$jobs_style_bootstrap_script
    export JOBS_TERMINAL_STYLE_REEXEC
    jobs_terminal_style_run /bin/bash "$jobs_style_bootstrap_script" "$@"
    jobs_style_bootstrap_result=$?
    exit "$jobs_style_bootstrap_result"
}
jobs_terminal_style_bootstrap "$0" "$@"
# <<< Jobs terminal style protection v1 <<<

DEFAULT_ACTION="menu"
DISTRO="RockyLinux"
# 为Bash提供明确的stdout与stderr输出。
print() {
    local destination=1
    if [[ "${1:-}" == -u2 ]]; then destination=2; shift; fi
    if [[ "${1:-}" == -r ]]; then shift; fi
    if [[ "${1:-}" == -- ]]; then shift; fi
    printf '%s\n' "$*" >&"$destination"
    if [[ -n "${LOG_FILE:-}" && "$LOG_FILE" != /dev/null && -f "$LOG_FILE" && ! -L "$LOG_FILE" ]]; then printf '%s\n' "$*" >> "$LOG_FILE"; fi
}
# 初始化当前解释器、项目路径和实例参数。
initialize() {
    set -Eeuo pipefail
    umask 077
    SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
    REPO="$(cd -- "$SCRIPT_DIR/../../../.." && pwd -P)"
    REPO_ID="$(printf '%s' "$REPO" | sha256sum | cut -c1-16)"
    STATE="${JOBS_IM_DEPLOY_STATE:-${XDG_STATE_HOME:-$HOME/.local/state}/JobsIM/Instances/$REPO_ID}"
    DEFAULT_INSTANCE_STATE="$STATE"
    if [[ -n "${JOBS_IM_DEPLOY_STATE:-}" ]]; then DEFAULT_INSTANCE_STATE=""; fi
    MAIN_PORT="${JOBS_IM_PG_PORT:-16543}"; JOURNAL_PORT="${JOBS_IM_JOURNAL_PORT:-16544}"
    HTTPS_PORT="${JOBS_IM_HTTPS_PORT:-9443}"; ROLE_PORT="${JOBS_IM_INTERNAL_PORT:-18101}"
    NATS_PORT="${JOBS_IM_NATS_PORT:-4222}"
    NATS_MAX_FILE="${JOBS_IM_NATS_MAX_FILE_BYTES:-536870912}"; NATS_MAX_MEMORY="${JOBS_IM_NATS_MAX_MEMORY_BYTES:-33554432}"
    PUBLIC_HOST="${JOBS_IM_PUBLIC_HOST:-127.0.0.1}"
    ACTION="$DEFAULT_ACTION"; YES=0; NO_BACKUP=0; RUN_ACTIVE=0; LOCK_HELD=0; CLEANING=0
    DEPLOY_MODE="${JOBS_IM_DEPLOY_MODE:-server}"; MODE_EXPLICIT=0; STATE_EXPLICIT=0
    LOCAL_ACCESS=local; ACCESS_EXPLICIT=0
    [[ -z "${JOBS_IM_DEPLOY_STATE:-}" ]] || STATE_EXPLICIT=1
    TUNNEL_PID=""; TUNNEL_START=""; RUN_DIR=""; QR_DIR=""; QR_PAGE=""; QR_BINARY=""; BROWSER_MENU_PID=""; RESOURCE_INITIAL=()
    discover_roles
}
# 从当前Go入口读取端口顺序；治理沿用既有identity数据库，journal保持独立集群。
discover_roles() {
    local raw role
    raw="$(sed -n 's/^[[:space:]]*roles := \[\]string{\(.*\)}/\1/p' "$REPO/代码/后端/Go/cmd/jobsimd/main.go" | tr '", ' '   ')"
    read -r -a SERVICES <<< "$raw"; ROLES=()
    (( ${#SERVICES[@]} > 0 )) || { print -u2 '无法从Go入口取得角色顺序，拒绝猜测端口。'; return 2; }
    for role in "${SERVICES[@]}"; do
        [[ "$role" =~ ^[a-z][a-z_]*$ ]] || return 2
        [[ "$role" == journal || "$role" == governance ]] || ROLES+=("$role")
    done
    [[ " ${SERVICES[*]} " == *' sync '* && " ${SERVICES[*]} " == *' journal '* ]] || { print -u2 '当前源码缺sync/journal角色；不发布不完整编排。'; return 2; }
}
# 仅生成展示；不把URL着色写回机器可读配置。
show_endpoint() {
    if [[ -t 1 && -z "${NO_COLOR+x}" && "${TERM:-dumb}" != dumb ]]; then printf '\033[32;4m%s\033[0m\n' "$1"; else print -r -- "$1"; fi
}
# 恢复只读布局不执行脚本；显式进程环境仍由操作者优先选择。
load_restore_layout() {
    local layout="$STATE/restore-layout.conf" key value format=""
    [[ -e "$layout" ]] || return 0
    [[ -f "$layout" && ! -L "$layout" ]] || { print -u2 '恢复布局须为普通文件。'; return 2; }
    local seen="|" count=0
    while IFS='=' read -r key value; do
        [[ -n "$key" && "$seen" != *"|$key|"* ]] || { print -u2 '恢复布局为空键或重复键。'; return 2; }
        seen+="$key|"; (( count+=1 ))
        case "$key" in
            format) [[ "$value" == 1 ]] || return 2; format=1 ;;
            main_port|journal_port|https_port|role_port|nats_port) [[ "$value" =~ ^[1-9][0-9]{0,4}$ ]] || return 2
                case "$key" in
                    main_port) [[ -n "${JOBS_IM_PG_PORT:-}" ]] || MAIN_PORT="$value" ;;
                    journal_port) [[ -n "${JOBS_IM_JOURNAL_PORT:-}" ]] || JOURNAL_PORT="$value" ;;
                    https_port) [[ -n "${JOBS_IM_HTTPS_PORT:-}" ]] || HTTPS_PORT="$value" ;;
                    role_port) [[ -n "${JOBS_IM_INTERNAL_PORT:-}" ]] || ROLE_PORT="$value" ;;
                    nats_port) [[ -n "${JOBS_IM_NATS_PORT:-}" ]] || NATS_PORT="$value" ;;
                esac ;;
            public_host) [[ "$value" =~ ^[A-Za-z0-9][A-Za-z0-9.-]*$ ]] || return 2; [[ -n "${JOBS_IM_PUBLIC_HOST:-}" ]] || PUBLIC_HOST="$value" ;;
            *) print -u2 '恢复布局含未知字段。'; return 2 ;;
        esac
    done < "$layout"
    [[ "$format" == 1 && $count == 7 ]] || { print -u2 '恢复布局字段不完整。'; return 2; }
}
# 校验显式动作、私密目录、端口与保护范围。
parse_arguments() {
    if (( $# > 0 )) && [[ "$1" != --* ]]; then ACTION="$1"; shift; fi
    while (( $# > 0 )); do
        case "$1" in
            --yes) YES=1; shift ;;
            --no-backup) NO_BACKUP=1; shift ;;
            --state) (( $# >= 2 )) || return 2; STATE="$2"; STATE_EXPLICIT=1; shift 2 ;;
            --mode) (( $# >= 2 )) || return 2; DEPLOY_MODE="$2"; MODE_EXPLICIT=1; shift 2 ;;
            --access) (( $# >= 2 )) || return 2; LOCAL_ACCESS="$2"; ACCESS_EXPLICIT=1; shift 2 ;;
            --help) ACTION="help"; shift ;;
            *) print -u2 "未知参数：$1"; return 2 ;;
        esac
    done
    [[ "$DEPLOY_MODE" == local || "$DEPLOY_MODE" == server ]] || return 2
    [[ "$LOCAL_ACCESS" == local || "$LOCAL_ACCESS" == quick ]] || return 2
    [[ "$DEPLOY_MODE" == local || "$ACCESS_EXPLICIT" == 0 ]] || return 2
    configure_mode_state
    [[ "$STATE" == /* && "$STATE" != / && "$STATE" != "$HOME" && ! -L "$STATE" ]] || { print -u2 'state须为独立、非符号链接的绝对目录。'; return 2; }
    load_restore_layout
    if [[ "$ACTION" == run && -f "$STATE/Application/restore.pending" ]]; then print -u2 '隔离恢复尚未完成；按原operation继续，当前实例不启动。'; return 2; fi
    BACKUPS="${JOBS_IM_BACKUP_DIR:-$(dirname -- "$STATE")/Backups/$(basename -- "$STATE")}"
    RECOVERY="${JOBS_IM_RECOVERY_DIR:-$(dirname -- "$STATE")/RecoveryKeys/$(basename -- "$STATE")}"
    assert_no_symlink_components "$STATE"; assert_no_symlink_components "$BACKUPS"; assert_no_symlink_components "$RECOVERY"
    STATE="$(realpath -m -- "$STATE")"; BACKUPS="$(realpath -m -- "$BACKUPS")"; RECOVERY="$(realpath -m -- "$RECOVERY")"
    local canonical_home="$(realpath -m -- "$HOME")"
    [[ "$STATE" != / && "$STATE" != "$canonical_home" && "$REPO" != "$STATE"/* ]] || { print -u2 'state不能是用户主目录、根或源码祖先。'; return 2; }
    local private_path
    for private_path in "$STATE" "$BACKUPS" "$RECOVERY"; do
        [[ "$private_path" == /* && "$private_path" != / && "$private_path" != "$canonical_home" && "$private_path" != "$REPO" && "$private_path" != "$REPO"/* && ! -L "$private_path" ]] || { print -u2 '实例、归档和恢复钥须为源码外的独立绝对目录。'; return 2; }
    done
    [[ "$STATE" != *"'"* && "$STATE" != *$'\n'* && "$STATE" != *$'\r'* ]] || { print -u2 'state不接受单引号或换行，避免PG启动参数歧义。'; return 2; }
    [[ "$BACKUPS" != "$STATE" && "$BACKUPS" != "$STATE"/* && "$RECOVERY" != "$STATE" && "$RECOVERY" != "$STATE"/* && "$RECOVERY" != "$BACKUPS" && "$RECOVERY" != "$BACKUPS"/* && "$BACKUPS" != "$RECOVERY"/* ]] || { print -u2 '数据、加密备份与解密私钥必须在彼此独立的目录。'; return 2; }
    for value in "$MAIN_PORT" "$JOURNAL_PORT" "$HTTPS_PORT" "$ROLE_PORT" "$NATS_PORT"; do
        [[ "$value" =~ ^[1-9][0-9]{0,4}$ ]] && (( value > 1024 && value < 65536 )) || { print -u2 '端口须为1025～65535的整数。'; return 2; }
    done
    ROLE_LAST=$((ROLE_PORT+${#SERVICES[@]}-1))
    (( ROLE_LAST < 65536 )) || { print -u2 '动态角色端口越界。'; return 2; }
    [[ "$NATS_MAX_FILE" =~ ^[1-9][0-9]{0,12}$ && "$NATS_MAX_MEMORY" =~ ^[1-9][0-9]{0,12}$ ]] && (( NATS_MAX_FILE >= 536870912 && NATS_MAX_MEMORY >= 33554432 )) || { print -u2 'NATS资源下限为磁盘512MiB／内存32MiB，可显式调大。'; return 2; }
    NATS_CONFIG="$STATE/Secrets/nats.conf"; NATS_STORE="$STATE/Application/JetStream"
    JOBS_IM_NATS_TOKEN_FILE="${JOBS_IM_NATS_TOKEN_FILE:-$STATE/Secrets/nats.token}"
    [[ "$JOBS_IM_NATS_TOKEN_FILE" == "$STATE/Secrets/"* && "$JOBS_IM_NATS_TOKEN_FILE" != *'/../'* && ! -L "$JOBS_IM_NATS_TOKEN_FILE" ]] || { print -u2 'NATS token文件须在本实例Secrets内且不是链接。'; return 2; }
    JOBS_IM_NATS_URL="${JOBS_IM_NATS_URL:-nats://127.0.0.1:$NATS_PORT}"
    [[ "$JOBS_IM_NATS_URL" == "nats://127.0.0.1:$NATS_PORT" ]] || { print -u2 '本入口只管理匹配端口的loopback NATS；不接管外部broker。'; return 2; }
    export JOBS_IM_NATS_URL JOBS_IM_NATS_TOKEN_FILE
    [[ "$PUBLIC_HOST" =~ ^[A-Za-z0-9][A-Za-z0-9.-]*$ ]] || { print -u2 '公开主机须为DNS名或IPv4，不含协议、端口或换行。'; return 2; }
}
# 只读检查本平台所需的既有工具。
find_management_tools() {
    linux_platform_check || return $?
    PG_BIN="${JOBS_IM_PG_BIN:-}"
    if [[ -z "$PG_BIN" ]]; then local pg="$(command -v pg_ctl || true)"; [[ -z "$pg" ]] || PG_BIN="$(dirname -- "$pg")"; [[ -n "$PG_BIN" || ! -d /usr/lib/postgresql/18/bin ]] || PG_BIN=/usr/lib/postgresql/18/bin; fi
    OPENSSL="${JOBS_IM_OPENSSL:-$(command -v openssl || true)}"
    [[ -x "$PG_BIN/pg_ctl" && -x "$PG_BIN/pg_controldata" && -x "$PG_BIN/pg_dump" && -x "$PG_BIN/pg_restore" && -x "$OPENSSL" ]] && command -v lsof >/dev/null || { print -u2 '需既有PG18、OpenSSL和lsof；不要求重装Go/Node或升级数据库。'; return 2; }
}
start_parent_guardian() {
    local parent="$$" parent_start="$(LC_ALL=C ps -p "$$" -o lstart=)" original_server="$(cat "$STATE/Processes/server.pid")" original_start="$(cat "$STATE/Processes/server.start")" original_nats="$(cat "$STATE/Processes/nats.pid")"
    (
        trap - EXIT ERR INT TERM HUP
        while [[ "$(LC_ALL=C ps -p "$parent" -o lstart= 2>/dev/null || true)" == "$parent_start" ]]; do sleep 1; done
        [[ "$(cat "$STATE/Processes/server.pid" 2>/dev/null)" == "$original_server" && "$(cat "$STATE/Processes/server.start" 2>/dev/null)" == "$original_start" && "$(cat "$STATE/Processes/nats.pid" 2>/dev/null)" == "$original_nats" ]] || exit 0
        LOCK_HELD=0; CLEANING=0; cleanup 143; exit 143
    ) &
    GUARDIAN_PID=$!; GUARDIAN_START="$(LC_ALL=C ps -p "$GUARDIAN_PID" -o lstart=)"
}
stop_parent_guardian() {
    if [[ -n "${GUARDIAN_PID:-}" && "$(LC_ALL=C ps -p "$GUARDIAN_PID" -o lstart= 2>/dev/null || true)" == "${GUARDIAN_START:-}" && "$(ps -p "$GUARDIAN_PID" -o ppid= 2>/dev/null | tr -d ' ')" == "$$" ]]; then kill -TERM "$GUARDIAN_PID" 2>/dev/null; wait "$GUARDIAN_PID" 2>/dev/null || true; fi
}

find_tools() {
    [[ "$(uname -s)" == Linux ]] || { print -u2 '此入口仅支持Linux，不代表本机已完成Linux验收。'; return 2; }
    [[ "${BASH_VERSINFO[0]}" -ge 4 ]] || { print -u2 '需要Bash4或更高。'; return 2; }
    GO_TOOL="${JOBS_IM_GO:-$(command -v go || true)}"
    PG_BIN="${JOBS_IM_PG_BIN:-}"
    if [[ -z "$PG_BIN" ]]; then
        local pg="$(command -v pg_ctl || true)"
        [[ -z "$pg" ]] || PG_BIN="$(dirname -- "$pg")"
        [[ -n "$PG_BIN" || ! -d /usr/lib/postgresql/18/bin ]] || PG_BIN=/usr/lib/postgresql/18/bin
    fi
    OPENSSL="${JOBS_IM_OPENSSL:-$(command -v openssl || true)}"
    [[ -x "$GO_TOOL" && -x "$PG_BIN/initdb" && -x "$PG_BIN/pg_ctl" && -x "$PG_BIN/psql" && -x "$PG_BIN/pg_dump" && -x "$PG_BIN/pg_restore" ]] || { print -u2 '需自行准备Go1.27与PG18，或指定JOBS_IM_GO/JOBS_IM_PG_BIN；不自动sudo/安装。'; return 2; }
    [[ -x "$OPENSSL" ]] && command -v curl >/dev/null && command -v npm >/dev/null && command -v node >/dev/null && command -v lsof >/dev/null || { print -u2 '缺OpenSSL、curl、Node/npm或lsof。'; return 2; }
    [[ "$(id -u)" != 0 ]] || { print -u2 '必须由普通专用用户运行；PG不接受root。'; return 2; }
    "$GO_TOOL" version
    "$PG_BIN/pg_ctl" --version
    "$OPENSSL" version
    node --version
    find_nats
}
# 只发现已安装的NATS；不sudo、不调用包管理器、不远程安装。
find_nats() {
    NATS_BIN="${JOBS_IM_NATS_BIN:-$(command -v nats-server || true)}"
    [[ "$NATS_BIN" != *'://'* && "$NATS_BIN" != *$'\n'* && "$NATS_BIN" != *$'\r'* ]] || { print -u2 'NATS工具须为本地可执行路径；无效输入不回显。'; return 2; }
    [[ -n "$NATS_BIN" && -x "$NATS_BIN" ]] || { print -u2 "缺nats-server：检查位置=${NATS_BIN:-PATH}；用JOBS_IM_NATS_BIN指定已有文件，不自动安装。"; return 2; }
    NATS_BIN="$(realpath -- "$NATS_BIN")"; NC="$(command -v nc || true)"
    [[ -x "$NC" ]] || { print -u2 '缺nc，无法做有界NATS认证检查。'; return 2; }
    "$NATS_BIN" --version
}
# 拒绝其他项目或无收据的非空实例。
assert_owner() {
    if [[ -n "${REF_ID:-}" && "$STATE" == "$DEFAULT_INSTANCE_STATE" ]]; then
        [[ -f "$STATE/Application/instance.json" && ! -L "$STATE/Application/instance.json" ]] || { print -u2 '本机引用对应的原实例缺失，保留引用，不生成替代身份。'; return 2; }
        local bound
        bound="$(sed -n 's/.*"deploymentId":[[:space:]]*"\([^"]*\)".*/\1/p' "$STATE/Application/instance.json")"
        [[ "$bound" == "$REF_DEPLOYMENT" ]] || { print -u2 '本机引用与原部署身份不同，拒绝操作。'; return 2; }
    fi
    if [[ -e "$STATE/owner" ]]; then
        [[ -f "$STATE/owner" && ! -L "$STATE/owner" && -O "$STATE/owner" && "$(sed -n '2p' "$STATE/owner")" == "$(id -u)" ]] || { print -u2 '实例收据用户或文件归属不匹配，拒绝操作。'; return 2; }
        local former actual
        former="$(sed -n '1p' "$STATE/owner")"
        if [[ "$former" != "$REPO" ]]; then
            actual="$(sed -n 's/.*"deploymentId":[[:space:]]*"\([^"]*\)".*/\1/p' "$STATE/Application/instance.json")"
            [[ -n "$REF_ID" && "$STATE" == "$DEFAULT_INSTANCE_STATE" && "$former" == /* && ! -e "$former" && ! -L "$former" && "$actual" == "$REF_DEPLOYMENT" && -f "$STATE/Application/instance.json" && ! -L "$STATE/Application/instance.json" ]] || { print -u2 '实例属于其他路径且没有匹配的本机搬迁引用，拒绝接管。'; return 2; }
            RELOCATE_OWNER=1
            print '检测到工作树移动；只读动作保留原收据，确认运行／停止后接续原实例。'
        fi
    elif [[ -d "$STATE" && -n "$(ls -A "$STATE")" ]]; then
        print -u2 '既有非空state没有归属收据，拒绝接管。'; return 2
    fi
}
# 本机引用只存公开实例ID；随工作树移动，Git克隆仍各自初始化。
read_instance_reference() {
    REF_ID=""; REF_DEPLOYMENT=""; RELOCATE_OWNER=0
    REF_DIR="$REPO/.jobsim-local"; REF_FILE="$REF_DIR/${REFERENCE_NAME:-instance.ref}"
    [[ -e "$REF_DIR" || -L "$REF_DIR" ]] || return 0
    [[ -d "$REF_DIR" && ! -L "$REF_DIR" && -O "$REF_DIR" ]] || { print -u2 '本机实例引用目录归属不明，拒绝使用。'; return 2; }
    [[ "$(stat -c %a "$REF_DIR")" == 700 ]] || { print -u2 '本机实例引用目录须为私有目录。'; return 2; }
    [[ -e "$REF_FILE" || -L "$REF_FILE" ]] || return 0
    [[ -f "$REF_FILE" && ! -L "$REF_FILE" && -O "$REF_FILE" ]] || { print -u2 '本机实例引用必须是本人普通文件。'; return 2; }
    [[ "$(stat -c %a "$REF_DIR")" == 700 && "$(stat -c %a "$REF_FILE")" == 600 ]] || { print -u2 '本机实例引用须为私有目录及文件。'; return 2; }
    local format uid extra
    format="$(sed -n '1p' "$REF_FILE")"; REF_ID="$(sed -n '2p' "$REF_FILE")"
    REF_DEPLOYMENT="$(sed -n '3p' "$REF_FILE")"; uid="$(sed -n '4p' "$REF_FILE")"
    extra="$(sed -n '5,$p' "$REF_FILE")"
    [[ "$format" == JobsIM-instance-ref-v1 && "$uid" == "$(id -u)" && -z "$extra" && "$REF_ID" =~ ^[0-9a-f]{16}$ && "$REF_DEPLOYMENT" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$ ]] || { print -u2 '本机实例引用格式或用户不匹配，保留原件。'; return 2; }
    REPO_ID="$REF_ID"
}
# 只在获得锁和操作确认后转移路径收据；绝不改变实例或CA。
apply_owner_relocation() {
    (( RELOCATE_OWNER == 1 )) || return 0
    (( LOCK_HELD == 1 )) || { print -u2 '缺实例锁，拒绝转移路径收据。'; return 2; }
    local previous next
    local former_owner_identity="$(resource_identity "$STATE/owner")"
    previous="$(mktemp "$STATE/owner-relocation.XXXXXX")"
    record_resource_if_managed "${previous##*/}"
    cp -p -- "$STATE/owner" "$previous"
    next="$(mktemp "$STATE/owner-new.XXXXXX")"
    printf '%s\n%s\n' "$REPO" "$(id -u)" > "$next"
    mv -- "$next" "$STATE/owner"
    record_replacement owner "$former_owner_identity"
    RELOCATE_OWNER=0
    print '已保留旧归属凭据并接续原实例；部署身份、CA及数据未改变。'
}
# 默认实例的公开引用不包含密钥，也不写入分发版本。
write_instance_reference() {
    [[ "$STATE" == "$DEFAULT_INSTANCE_STATE" ]] || return 0
    local deployment="$1" next
    [[ -z "$REF_DEPLOYMENT" || "$REF_DEPLOYMENT" == "$deployment" ]] || { print -u2 '本机引用与服务端部署身份不同，拒绝替换。'; return 2; }
    [[ ! -L "$REF_DIR" ]] || return 2
    mkdir -p -- "$REF_DIR"
    [[ -d "$REF_DIR" && -O "$REF_DIR" ]] || return 2
    next="$(mktemp "$REF_DIR/.instance-ref.XXXXXX")"
    printf 'JobsIM-instance-ref-v1\n%s\n%s\n%s\n' "$REPO_ID" "$deployment" "$(id -u)" > "$next"
    mv -- "$next" "$REF_FILE"
}
# 展示前台终端生命周期。
terminal_notice() {
    if [[ -t 1 && -z "${NO_COLOR:-}" && "${TERM:-dumb}" != dumb ]]; then printf '\033[1;31m%s\033[0m\n' '关闭此运行终端，服务将结束'; else print '关闭此运行终端，服务将结束'; fi
}
# 展示变更范围并确认，显式自动化授权除外。
confirm_action() {
    print "JobsIM Linux · $ACTION"
    print "实例：$STATE"
    print "HTTPS=${HTTPS_PORT}；业务PG=${MAIN_PORT}；独立journalPG=${JOURNAL_PORT}；内部角色=${ROLE_PORT}～$ROLE_LAST"
    print '只操作此收据匹配实例；不安装全局工具、不清理数据、不修改系统CA。备份会暂停本实例，完成后保持停止。'
    [[ "$ACTION" != run ]] || terminal_notice
    if (( YES == 0 )); then
        [[ -t 0 ]] || { print -u2 '非交互执行须显式传--yes；check/plan不需确认。'; return 2; }
        local answer
        read -r -p '按回车确认，输入任意文字取消：' answer || return 130
        [[ -z "$answer" ]] || return 130
    fi
}
# 确认后创建仅本实例的私有状态结构。
prepare_state() {
    assert_owner
    [[ -d "$STATE/.Deployment" ]] || capture_resource_origin
    mkdir -p "$STATE" "$STATE/Processes" "$STATE/Logs" "$STATE/Secrets" "$STATE/TLS" "$STATE/Application" "$STATE/Bin" "$STATE/Sockets/Main" "$STATE/Sockets/Journal"
    [[ -f "$STATE/owner" ]] || printf '%s\n%s\n' "$REPO" "$(id -u)" > "$STATE/owner"
    chmod 700 "$STATE" "$STATE/Secrets" "$STATE/TLS"
}
# 取得本实例生命周期事务锁，不接管未知锁。
acquire_lock() {
    if ! mkdir "$STATE/Lock" 2>/dev/null; then
        print -u2 '已有部署/停止事务锁；不删除锁或接管未知事务。请核对实例Processes和Lock归属。'; return 2
    fi
    LOCK_HELD=1
    printf '%s\n' "$$" > "$STATE/Lock/pid"
}
# 仅释放当前进程拥有的锁。
release_lock() {
    if (( LOCK_HELD == 1 )); then
        [[ "$(cat "$STATE/Lock/pid" 2>/dev/null)" == "$$" ]] && { rm -- "$STATE/Lock/pid"; rmdir "$STATE/Lock"; }
        LOCK_HELD=0
    fi
}
# 核验后端PID、启动时刻与原可执行路径。
process_matches() {
    [[ -f "$STATE/Processes/server.pid" && -f "$STATE/Processes/server.start" ]] || return 1
    local pid="$(cat "$STATE/Processes/server.pid")" start command
    [[ "$pid" =~ ^[0-9]+$ ]] || return 1
    start="$(LC_ALL=C ps -p "$pid" -o lstart= 2>/dev/null || true)"
    [[ -n "$start" && "$start" == "$(cat "$STATE/Processes/server.start")" ]] || return 1
    command="$(ps -ww -p "$pid" -o command=)"
    [[ "$command" == *"$STATE/Bin/JobsIMd"* && "$command" == *"$STATE/Application"* ]]
}
# 优雅结束本实例后端，身份不明或超时保留。
stop_backend() {
    if process_matches; then
        local pid="$(cat "$STATE/Processes/server.pid")"
        kill -TERM "$pid" || return $?
        local count=0
        while kill -0 "$pid" 2>/dev/null && (( count < 70 )); do sleep 0.1; (( count+=1 )); done
        if kill -0 "$pid" 2>/dev/null; then print -u2 '服务未按时退出，保留数据和进程，拒绝强杀。'; return 2; fi
    elif [[ -f "$STATE/Processes/server.pid" ]]; then
        local pid="$(cat "$STATE/Processes/server.pid")"
        if [[ "$pid" =~ ^[0-9]+$ ]] && kill -0 "$pid" 2>/dev/null; then print -u2 'PID存在但启动身份不匹配，不停止陌生进程。'; return 2; fi
    fi
    if lsof -nP -iTCP:"$HTTPS_PORT" -sTCP:LISTEN >/dev/null 2>&1; then print -u2 '应用端口仍有监听，未证明应用退出；保留broker与数据库。'; return 2; fi
}
# 校验本入口NATS的PID、启动时刻、原工具、配置及摘要；不按名称或端口认领。
nats_process_matches() {
    local name
    for name in pid start binary binary.sha256 config.sha256; do [[ -f "$STATE/Processes/nats.$name" && ! -L "$STATE/Processes/nats.$name" ]] || return 1; done
    local broker_pid="$(cat "$STATE/Processes/nats.pid")" binary="$(cat "$STATE/Processes/nats.binary")" started command
    [[ "$broker_pid" =~ ^[1-9][0-9]*$ && -f "$binary" && ! -L "$binary" && -f "$NATS_CONFIG" && ! -L "$NATS_CONFIG" ]] || return 1
    started="$(LC_ALL=C ps -p "$broker_pid" -o lstart= 2>/dev/null || true)"
    [[ -n "$started" && "$started" == "$(cat "$STATE/Processes/nats.start")" ]] || return 1
    [[ "$(sha256sum "$binary" | cut -d' ' -f1)" == "$(cat "$STATE/Processes/nats.binary.sha256")" && "$(sha256sum "$NATS_CONFIG" | cut -d' ' -f1)" == "$(cat "$STATE/Processes/nats.config.sha256")" ]] || return 1
    command="$(ps -ww -p "$broker_pid" -o command=)"
    [[ "$command" == "$binary -c $NATS_CONFIG" ]]
}
# 应用停止后才优雅停止本入口broker；未知身份或超时保留，不强杀。
stop_nats() {
    if nats_process_matches; then
        local broker_pid="$(cat "$STATE/Processes/nats.pid")" count=0
        kill -TERM "$broker_pid" || return $?
        while kill -0 "$broker_pid" 2>/dev/null && (( count < 100 )); do sleep 0.1; (( count+=1 )); done
        if kill -0 "$broker_pid" 2>/dev/null; then print -u2 '本实例NATS未退出；保留JetStream及进程，不强杀。'; return 2; fi
    elif [[ -f "$STATE/Processes/nats.pid" ]]; then
        local broker_pid="$(cat "$STATE/Processes/nats.pid")"
        if [[ "$broker_pid" =~ ^[1-9][0-9]*$ ]] && kill -0 "$broker_pid" 2>/dev/null; then print -u2 'NATS PID存在但身份/配置不匹配，不停止。'; return 2; fi
    fi
}
# 将路径编码到NATS字符串；凭据只经私有文件，不进入argv或日志。
nats_quoted() {
    local value="$1"; value="${value//\\/\\\\}"; value="${value//\"/\\\"}"; printf '%s' "$value"
}
# 在确认后的本实例内准备token与配置；既有数据缺钥或配置漂移拒绝覆盖。
prepare_nats() {
    [[ ! -L "$NATS_STORE" && ! -L "$NATS_CONFIG" && "$(dirname -- "$JOBS_IM_NATS_TOKEN_FILE")" == "$STATE/Secrets" ]] || { print -u2 'NATS状态/配置/密钥路径不合格。'; return 2; }
    if [[ ! -f "$JOBS_IM_NATS_TOKEN_FILE" ]]; then
        [[ ! -e "$NATS_CONFIG" && ! -e "$NATS_STORE" ]] || { print -u2 '既有JetStream或配置缺token，不生成替代密钥。'; return 2; }
        "$OPENSSL" rand -hex 32 > "$JOBS_IM_NATS_TOKEN_FILE"
    fi
    [[ ! -L "$JOBS_IM_NATS_TOKEN_FILE" && "$(stat -c '%a' "$JOBS_IM_NATS_TOKEN_FILE")" == 600 && "$(stat -c '%u' "$JOBS_IM_NATS_TOKEN_FILE")" == "$(id -u)" ]] || { print -u2 'NATS token须为当前用户0600普通文件。'; return 2; }
    local token="$(cat "$JOBS_IM_NATS_TOKEN_FILE")" quoted_store quoted_pid temporary="$STATE/Secrets/.nats-config-$$"
    [[ "$token" =~ ^[0-9a-f]{64}$ ]] || { print -u2 'NATS token格式无效；不输出或替换原秘密。'; return 2; }
    quoted_store="$(nats_quoted "$NATS_STORE")"; quoted_pid="$(nats_quoted "$STATE/Processes/nats.runtime.pid")"
    printf 'listen: "127.0.0.1:%s"\npid_file: "%s"\nauthorization { token: "%s" }\njetstream { store_dir: "%s", max_file_store: %s, max_memory_store: %s }\n' "$NATS_PORT" "$quoted_pid" "$token" "$quoted_store" "$NATS_MAX_FILE" "$NATS_MAX_MEMORY" > "$temporary"
    unset token
    if [[ -e "$NATS_CONFIG" ]]; then
        if ! cmp -s "$temporary" "$NATS_CONFIG"; then rm -- "$temporary"; print -u2 '既有NATS配置与所选参数不一致；保留原件，需受控变更。'; return 2; fi
        rm -- "$temporary"
    else mv -- "$temporary" "$NATS_CONFIG"; fi
    chmod 600 "$NATS_CONFIG"
    mkdir -p "$NATS_STORE"; chmod 700 "$NATS_STORE"
    "$NATS_BIN" -t -c "$NATS_CONFIG" > "$STATE/Logs/nats-config-check.log" 2>&1 || { print -u2 'NATS私有配置校验失败；错误日志仅留本实例私有目录。'; return 2; }
}
# 用token经标准输入做PING；认证内容不进入nc参数或输出。
nats_ready() {
    local token="$(cat "$JOBS_IM_NATS_TOKEN_FILE")" response
    response="$(printf 'CONNECT {"auth_token":"%s","verbose":true}\r\nPING\r\n' "$token" | "$NC" -w 1 127.0.0.1 "$NATS_PORT" 2>/dev/null)" || true
    unset token
    [[ "$response" == *PONG* && "$response" == *'"jetstream":true'* && "$response" != *'-ERR'* ]]
}
# 启动受管broker并在任何业务启动前验证认证就绪。
start_nats() {
    prepare_nats
    "$NATS_BIN" -c "$NATS_CONFIG" > "$STATE/Logs/nats.log" 2>&1 &
    local broker_pid=$! count=0
    print "$broker_pid" > "$STATE/Processes/nats.pid"
    LC_ALL=C ps -p "$broker_pid" -o lstart= > "$STATE/Processes/nats.start"
    print -r -- "$NATS_BIN" > "$STATE/Processes/nats.binary"
    sha256sum "$NATS_BIN" | cut -d' ' -f1 > "$STATE/Processes/nats.binary.sha256"
    sha256sum "$NATS_CONFIG" | cut -d' ' -f1 > "$STATE/Processes/nats.config.sha256"
    while (( count < 20 )); do
        if nats_process_matches && nats_ready; then return 0; fi
        kill -0 "$broker_pid" 2>/dev/null || break
        sleep 0.2; (( count+=1 ))
    done
    print -u2 'NATS认证未就绪，拒绝启动应用；保留数据与私有日志。'; return 2
}
# 记录冷态broker归档范围；从未启用时明确为空，不把PG导出写成broker备份。
write_nats_backup_manifest() {
    local destination="$1"
    if [[ -d "$NATS_STORE" && -f "$NATS_CONFIG" && -f "$JOBS_IM_NATS_TOKEN_FILE" ]]; then
        [[ -f "$STATE/Processes/nats.pid" && -f "$STATE/Processes/nats.config.sha256" ]] || { print -u2 'NATS原归属收据缺失，不能证明broker已停，保留原件。'; return 2; }
        local original_pid="$(cat "$STATE/Processes/nats.pid")"
        [[ "$original_pid" =~ ^[1-9][0-9]*$ ]] && ! kill -0 "$original_pid" 2>/dev/null || { print -u2 'NATS原PID未证实退出，拒绝冷态归档。'; return 2; }
        [[ "$(sha256sum "$NATS_CONFIG" | cut -d' ' -f1)" == "$(cat "$STATE/Processes/nats.config.sha256")" ]] || { print -u2 'NATS配置偏离原收据，保留并拒绝标完整。'; return 2; }
        printf 'component=NATS JetStream\nstate=stopped-before-archive\nstore=Application/JetStream\nconfig=Secrets/nats.conf\ntoken=Secrets/%s\nrestore=offline verification required; not tested by archive listing\n' "$(basename -- "$JOBS_IM_NATS_TOKEN_FILE")" > "$destination"
    elif [[ -e "$NATS_STORE" || -e "$NATS_CONFIG" ]]; then print -u2 'NATS备份范围材料不完整，拒绝标完整归档。'; return 2
    else printf 'component=NATS JetStream\nstate=never-initialized\nstoreIncluded=false\n' > "$destination"; fi
}
# 读取物理PostgreSQL集群标识。
cluster_identity() { "$PG_BIN/pg_controldata" "$1" | awk -F: '/Database system identifier/{gsub(/ /,"",$2);print $2}'; }
# 核对当前数据目录与原集群收据。
check_cluster() {
    local name="$1" data="$2"
    [[ -f "$data/PG_VERSION" ]] || return 1
    [[ -f "$STATE/Processes/$name.cluster" && "$(cluster_identity "$data")" == "$(cat "$STATE/Processes/$name.cluster")" ]] || { print -u2 "$name 集群身份不符，拒绝控制。"; return 2; }
}
# 仅停止收据匹配的本实例数据库。
stop_clusters() {
    local name data
    for name in Journal Main; do
        data="$STATE/${name}Database"
        if [[ -f "$data/PG_VERSION" ]]; then
            check_cluster "$name" "$data" || return $?
            if "$PG_BIN/pg_ctl" -D "$data" status >/dev/null 2>&1; then "$PG_BIN/pg_ctl" -D "$data" -m fast -w -t 30 stop || return $?; fi
        fi
    done
}
# 检查所有动态角色及依赖端口，不接管占用者。
check_ports() {
    local -a ports; ports=("$HTTPS_PORT" "$MAIN_PORT" "$JOURNAL_PORT" "$NATS_PORT")
    local i port
    for (( i=0; i<${#SERVICES[@]}; i++ )); do ports+=("$((ROLE_PORT+i))"); done
    local -a unique_ports; mapfile -t unique_ports < <(printf '%s\n' "${ports[@]}" | sort -u)
    [[ "${#ports}" == "${#unique_ports}" ]] || { print -u2 '端口配置重复。'; return 2; }
    for port in "${ports[@]}"; do
        if lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then print -u2 "端口${port}已被占用，不接管。"; return 2; fi
    done
}
# 仅向当前PG子进程传递操作员凭据。
pg_operator() {
    local port="$1" database="$2"; shift 2
    PGPASSWORD="$(cat "$STATE/Secrets/operator.password")" "$PG_BIN/psql" -X -v ON_ERROR_STOP=1 -h 127.0.0.1 -p "$port" -U jobsim_operator -d "$database" "$@"
}
# 准备并验真业务与独立journal集群。
prepare_clusters() {
    RUN_ACTIVE=1
    local name port data
    if [[ ! -f "$STATE/Secrets/operator.password" ]]; then
        [[ ! -f "$STATE/MainDatabase/PG_VERSION" && ! -f "$STATE/JournalDatabase/PG_VERSION" ]] || { print -u2 '数据库已存在但操作密钥缺失，拒绝重置。'; return 2; }
        "$OPENSSL" rand -hex 32 > "$STATE/Secrets/operator.password" || return $?
    fi
    for name in Main Journal; do
        data="$STATE/${name}Database"; port="$MAIN_PORT"; [[ "$name" == Main ]] || port="$JOURNAL_PORT"
        if [[ ! -f "$data/PG_VERSION" ]]; then
            "$PG_BIN/initdb" -D "$data" -U jobsim_operator --pwfile="$STATE/Secrets/operator.password" --auth-local=trust --auth-host=scram-sha-256 --encoding=UTF8 --locale=C > "$STATE/Logs/init-$name.log" || return $?
            record_resource_if_managed "${data##*/}" || return $?
            cluster_identity "$data" > "$STATE/Processes/$name.cluster" || return $?
        fi
        check_cluster "$name" "$data" || return $?
        if ! "$PG_BIN/pg_ctl" -D "$data" status >/dev/null 2>&1; then
            "$PG_BIN/pg_ctl" -D "$data" -l "$STATE/Logs/pg-$name.log" -w -t 30 -o "-p $port -h 127.0.0.1 -k '' -c max_connections=80 -c shared_buffers=128MB -c fsync=on -c full_page_writes=on -c synchronous_commit=on" start || return $?
        fi
    done
    RUN_ACTIVE=1
}
# 按当前源码创建所属域库及最小用户。
prepare_databases() {
    local role password exists
    local -a all_roles; all_roles=("${ROLES[@]}" journal)
    for role in "${all_roles[@]}"; do
        local port="$MAIN_PORT"; [[ "$role" != journal ]] || port="$JOURNAL_PORT"
        if [[ ! -f "$STATE/Secrets/$role.password" ]]; then
            exists="$(pg_operator "$port" postgres -Atc "SELECT 1 FROM pg_roles WHERE rolname='jobsim_$role'")"
            [[ -z "$exists" ]] || { print -u2 "${role}密钥缺失，不重置既有角色。"; return 2; }
            "$OPENSSL" rand -hex 32 > "$STATE/Secrets/$role.password"
        fi
        password="$(cat "$STATE/Secrets/$role.password")"
        exists="$(pg_operator "$port" postgres -Atc "SELECT 1 FROM pg_roles WHERE rolname='jobsim_$role'")"
        if [[ -z "$exists" ]]; then pg_operator "$port" postgres -q -c "CREATE ROLE jobsim_$role LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD '$password'"; fi
        exists="$(pg_operator "$port" postgres -Atc "SELECT 1 FROM pg_database WHERE datname='jobsim_$role'")"
        if [[ -z "$exists" ]]; then pg_operator "$port" postgres -q -c "CREATE DATABASE jobsim_$role OWNER jobsim_$role"; fi
        pg_operator "$port" "jobsim_$role" -q -c "GRANT EXECUTE ON FUNCTION pg_control_system() TO jobsim_$role"
        export "JOBS_IM_${role^^}_DATABASE_URL=postgresql://jobsim_$role:$password@127.0.0.1:$port/jobsim_$role?sslmode=disable"
    done
    JOBS_IM_DATABASE_URL="$JOBS_IM_IDENTITY_DATABASE_URL"; export JOBS_IM_DATABASE_URL
    # Governance schema stays in the identity database covered by identity.dump.
    JOBS_IM_GOVERNANCE_DATABASE_URL="$JOBS_IM_IDENTITY_DATABASE_URL"; export JOBS_IM_GOVERNANCE_DATABASE_URL
}
# 准备实例专用TLS材料，不覆盖既有信任根。
prepare_tls() {
    if [[ -f "$STATE/TLS/server.crt" && -f "$STATE/TLS/server.key" && -f "$STATE/TLS/ca.crt" ]]; then
        "$OPENSSL" verify -x509_strict -purpose sslserver -CAfile "$STATE/TLS/ca.crt" "$STATE/TLS/server.crt" >/dev/null || { print -u2 '现有TLS证书未通过严格验证；保留CA、指纹与原文件，需区分CA与服务端证书：CA有效才可原CA重签，CA缺陷需受控信任迁移。'; return 2; }
        "$OPENSSL" x509 -in "$STATE/TLS/server.crt" -checkend 86400 -noout >/dev/null || { print -u2 '证书将到期；需受控轮换，当前入口不重置信任根。'; return 2; }
        return
    fi
    [[ -z "$(ls -A "$STATE/TLS")" ]] || { print -u2 'TLS材料不完整或存在未知内容，保留全部原件并停止。'; return 2; }
    local former_tls_identity="$(resource_identity "$STATE/TLS")"
    local tls_stage="$(mktemp -d "$STATE/.TLS-pending.XXXXXX")"
    record_resource_if_managed "${tls_stage##*/}"
    [[ -n "$tls_stage" && -d "$tls_stage" ]] || return 2
    "$OPENSSL" req -x509 -newkey rsa:3072 -nodes -days 365 -keyout "$tls_stage/ca.key" -out "$tls_stage/ca.crt" -subj '/CN=JobsIM Local Instance CA' -addext 'basicConstraints=critical,CA:TRUE' -addext 'keyUsage=critical,keyCertSign,cRLSign' -addext 'subjectKeyIdentifier=hash' > "$STATE/Logs/tls.log" 2>&1 || return $?
    "$OPENSSL" req -new -newkey rsa:2048 -nodes -keyout "$tls_stage/server.key" -out "$tls_stage/server.csr" -subj "/CN=$PUBLIC_HOST" >> "$STATE/Logs/tls.log" 2>&1 || return $?
    printf 'subjectAltName=DNS:localhost,IP:127.0.0.1,IP:10.0.2.2\n' > "$tls_stage/server.ext"
    if [[ "$PUBLIC_HOST" != 127.0.0.1 && "$PUBLIC_HOST" != localhost ]]; then
        if [[ "$PUBLIC_HOST" =~ ^[0-9.]+$ ]]; then print "subjectAltName=DNS:localhost,IP:127.0.0.1,IP:10.0.2.2,IP:$PUBLIC_HOST" > "$tls_stage/server.ext"; else print "subjectAltName=DNS:localhost,IP:127.0.0.1,IP:10.0.2.2,DNS:$PUBLIC_HOST" > "$tls_stage/server.ext"; fi
    fi
    printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth\nsubjectKeyIdentifier=hash\nauthorityKeyIdentifier=keyid,issuer\n' >> "$tls_stage/server.ext"
    "$OPENSSL" x509 -req -in "$tls_stage/server.csr" -CA "$tls_stage/ca.crt" -CAkey "$tls_stage/ca.key" -CAcreateserial -days 90 -out "$tls_stage/server.crt" -extfile "$tls_stage/server.ext" >> "$STATE/Logs/tls.log" 2>&1 || return $?
    "$OPENSSL" verify -x509_strict -purpose sslserver -CAfile "$tls_stage/ca.crt" "$tls_stage/server.crt" >> "$STATE/Logs/tls.log" 2>&1 || return $?
    # 生成或验真失败只留下私有pending；实例锁内提交前再次确认目标为空。
    rmdir -- "$STATE/TLS" || return $?
    mv -n -- "$tls_stage" "$STATE/TLS" || return $?
    record_replacement TLS "$former_tls_identity"
    [[ -f "$STATE/TLS/server.crt" && ! -e "$tls_stage" ]] || { print -u2 'TLS提交发生并发变化，保留材料并停止。'; return 2; }
}
# 构建当前源码，失败不替换为假成功。
build_source() {
    print '构建Go角色与WebDist；依赖缓存留在用户级，未安装全局工具。'
    (cd "$REPO/代码/后端/Go" && GOROOT= GOWORK=off GOOS=linux GOARCH="$MACHINE_ARCH" GOTOOLCHAIN=local "$GO_TOOL" build -mod=readonly -trimpath -o "$STATE/Bin/JobsIMd.new" ./cmd/jobsimd)
    mv -- "$STATE/Bin/JobsIMd.new" "$STATE/Bin/JobsIMd"
    # 编译产物完全位于本实例，不覆盖源码Dist或共享工具。
    # 运行资产放实例持久目录，源码再次移动也不影响已经启动的首页。
    WEB_RUNTIME="$(mktemp -d "$STATE/WebDist.XXXXXX")"
    record_resource_if_managed "${WEB_RUNTIME##*/}"
    (cd "$REPO/代码/前端/Web" && node "$REPO/代码/前端/Web/node_modules/typescript/bin/tsc" --noEmit && node "$REPO/代码/前端/Web/node_modules/vite/bin/vite.js" build --outDir "$WEB_RUNTIME")
    [[ -f "$WEB_RUNTIME/index.html" && ! -L "$WEB_RUNTIME/index.html" ]] || { print -u2 'Web运行资产不完整，保留本轮副本并拒绝启动。'; return 2; }
    [[ -f "$WEB_RUNTIME/index.html" ]] || { print -u2 'WebDist缺失。'; return 2; }
}
# 应用与broker停止后生成范围明确的加密归档。
backup_instance() {
    stop_backend || return $?
    stop_nats || return $?
    prepare_clusters || return $?
    assert_no_symlink_components "$BACKUPS" || return $?
    assert_no_symlink_components "$RECOVERY" || return $?
    mkdir -p "$BACKUPS" "$RECOVERY" || return $?
    chmod 700 "$BACKUPS" "$RECOVERY" || return $?
    local measured
    measured="$(files_actual_bytes "$STATE/MainDatabase" "$STATE/JournalDatabase" "$STATE/Application" "$STATE/TLS" "$STATE/Secrets")" || return $?
    disk_budget_bytes "$BACKUPS" "$((measured*2))" || return $?
    if [[ ! -f "$RECOVERY/key.pem" ]]; then
        [[ ! -e "$RECOVERY/cert.pem" ]] || { print -u2 '恢复密钥材料不完整。'; return 2; }
        "$OPENSSL" req -x509 -newkey rsa:3072 -nodes -days 3650 -keyout "$RECOVERY/key.pem" -out "$RECOVERY/cert.pem" -subj '/CN=JobsIM Backup Recovery' > "$STATE/Logs/backup-key.log" 2>&1 || return $?
    fi
    local staging="$BACKUPS/.staging-$(date +%Y%m%d-%H%M%S)-$$" file="$BACKUPS/JobsIM-$(date +%Y%m%d-%H%M%S)-$$.cms" verified role port operator_password
    mkdir "$staging" || return $?
    write_nats_backup_manifest "$staging/broker-manifest.txt" || return $?
    [[ -s "$staging/broker-manifest.txt" && ! -L "$staging/broker-manifest.txt" ]] || return 2
    operator_password="$(cat "$STATE/Secrets/operator.password")" || return $?
    for role in "${ROLES[@]}" journal; do
        port="$MAIN_PORT"; [[ "$role" != journal ]] || port="$JOURNAL_PORT"
        PGPASSWORD="$operator_password" "$PG_BIN/pg_dump" -h 127.0.0.1 -p "$port" -U jobsim_operator -d "jobsim_$role" --format=custom --file="$staging/$role.dump" || return $?
        [[ -s "$staging/$role.dump" && ! -L "$staging/$role.dump" ]] || return 2
        "$PG_BIN/pg_restore" --list "$staging/$role.dump" > "$staging/$role.list" || return $?
        [[ -s "$staging/$role.list" ]] || return 2
    done
    measured="$(files_actual_bytes "$STATE/Application" "$STATE/TLS" "$STATE/Secrets" "$staging")" || return $?
    disk_budget_bytes "$BACKUPS" "$((measured*2))" || return $?
    (set -o pipefail; tar -czf - -C "$STATE" owner Application TLS Secrets -C "$staging" . | "$OPENSSL" cms -encrypt -aes-256-gcm -binary -outform DER -out "$file" "$RECOVERY/cert.pem") || return $?
    (set -o pipefail; "$OPENSSL" cms -decrypt -binary -inform DER -in "$file" -inkey "$RECOVERY/key.pem" | tar -tzf -) > "$file.manifest" || return $?
    grep -Eq '(^|/)broker-manifest\.txt$' "$file.manifest" || return 2
    for role in "${ROLES[@]}" journal; do grep -Eq "(^|/)$role\\.dump$" "$file.manifest" || return 2; done
    verified="$(mktemp -d "$BACKUPS/.verify-logical-XXXXXX")" || return $?
    local -a members; members=(./broker-manifest.txt)
    for role in "${ROLES[@]}" journal; do members+=("./$role.dump"); done
    (set -o pipefail; "$OPENSSL" cms -decrypt -binary -inform DER -in "$file" -inkey "$RECOVERY/key.pem" | tar -xzf - -C "$verified" "${members[@]}") || return $?
    [[ -f "$verified/broker-manifest.txt" && ! -L "$verified/broker-manifest.txt" ]] && cmp -s "$staging/broker-manifest.txt" "$verified/broker-manifest.txt" || return 2
    for role in "${ROLES[@]}" journal; do
        [[ -s "$verified/$role.dump" && ! -L "$verified/$role.dump" ]] && cmp -s "$staging/$role.dump" "$verified/$role.dump" || return 2
        "$PG_BIN/pg_restore" --list "$verified/$role.dump" >/dev/null || return $?
        rm -- "$verified/$role.dump" || return $?
    done
    rm -- "$verified/broker-manifest.txt" || return $?
    rmdir "$verified" || return $?
    sha256sum "$file" > "$file.sha256" || return $?
    for role in "${ROLES[@]}" journal; do rm -- "$staging/$role.dump" "$staging/$role.list" || return $?; done
    rm -- "$staging/broker-manifest.txt" || return $?
    rmdir "$staging" || return $?
    print "已生成并解密验真：$file"
    print "恢复私钥独立保存于：$RECOVERY/key.pem；请另行离线保管。备份不包含这把解密私钥。"
    print '备份包含独立journal导出，但不自动覆盖当前journal。尚未实现安全恢复向导，不以归档验真冒充恢复演练。'
}
# 仅展示无秘密连接身份及CA指纹。
write_connection() {
    local url="$PUBLIC_URL" response deployment fingerprint
    response="$(curl --fail --silent --show-error --cacert "$STATE/TLS/ca.crt" --resolve "$PUBLIC_HOST:$HTTPS_PORT:127.0.0.1" --max-time 5 "https://$PUBLIC_HOST:$HTTPS_PORT/api/bootstrap")" || return $?
    deployment="$(print -r -- "$response" | sed -n 's/.*"deploymentId":"\([^"]*\)".*/\1/p')"
    [[ -n "$deployment" ]] || { print -u2 '公开实例身份缺失。'; return 2; }
    fingerprint="$(set -o pipefail; "$OPENSSL" x509 -in "$STATE/TLS/ca.crt" -noout -fingerprint -sha256 | cut -d= -f2)" || return $?
    write_instance_reference "$deployment" || return $?
    if [[ "$DEPLOY_MODE" == local && "$url" == https://*.trycloudflare.com ]]; then
        printf '{"deploymentId":"%s","name":"JobsIM","url":"%s","protocolVersion":"1","tlsTrust":"system","originCaSha256":"%s"}\n' "$deployment" "$url" "$fingerprint" > "$STATE/connection.json" || return $?
    else
        printf '{"deploymentId":"%s","name":"JobsIM","url":"%s","protocolVersion":"1","caSha256":"%s"}\n' "$deployment" "$url" "$fingerprint" > "$STATE/connection.json" || return $?
    fi
    print "用户端：$url"; print "deploymentId：$deployment"
    print "Origin CA SHA256（后端直连核对，非Quick公网证书根）：$fingerprint"
    print "公开连接描述：$STATE/connection.json；端侧system-trust描述导入需各端另验。"
}
# 本地和服务器各自使用独立实例；公网就绪后才发布菜单。
run_instance() {
    confirm_action || return $?
    capture_resource_origin || return $?
    prepare_state || return $?
    acquire_lock || return $?
    initialize_resource_ledger || return $?
    apply_owner_relocation || return $?
    [[ ! -e "$STATE/.Deployment/undeploy" ]] || { print -u2 '此实例已有反部署检查点；须先核对原备份和受保护journal，禁止重建身份。'; return 2; }
    machine_budget_check || return $?
    prepare_current_tools || return $?
    RUN_ACTIVE=1
    stop_tunnel || return $?
    if [[ -f "$STATE/Application/instance.json" ]]; then backup_instance || return $?; stop_clusters || return $?; fi
    stop_backend || return $?
    stop_nats || return $?
    stop_clusters || return $?
    check_ports || return $?
    prepare_clusters || return $?
    prepare_databases || return $?
    import_server_tls || return $?
    prepare_tls || return $?
    build_source || return $?
    start_nats || return $?
    start_backend_for_url "$PUBLIC_URL" || return $?
    if [[ "$DEPLOY_MODE" == local && "$LOCAL_ACCESS" == quick ]]; then attempt_quick_access || return $?; fi
    if [[ "$DEPLOY_MODE" == server ]]; then curl --fail --silent --max-time 10 "$PUBLIC_URL/api/health" >/dev/null || { print -u2 '服务器正式URL严格HTTPS未通过，不宣称对外可用。'; return 2; }; fi
    printf 'project=%s\nuid=%s\nmode=%s\npg_main=%s\npg_journal=%s\nhttps=%s\nrole_base=%s\n' "$REPO" "$(id -u)" "$DEPLOY_MODE" "$MAIN_PORT" "$JOURNAL_PORT" "$HTTPS_PORT" "$ROLE_PORT" > "$STATE/receipt.txt"
    record_resource receipt.txt
    write_connection
    local deployment="$(sed -n 's/.*"deploymentId":[[:space:]]*"\([^"]*\)".*/\1/p' "$STATE/Application/instance.json")"
    bind_resource_deployment "$deployment"
    [[ ! -f "$STATE/connection.json" ]] || record_resource connection.json
    [[ ! -f "$STATE/connection.png" ]] || record_resource connection.png
    prepare_access_qr
    show_endpoint "$PUBLIC_URL/"
    print '管理端未实现，不伪造管理端URL；Quick地址与二维码随本轮退出失效。'
    release_lock; start_parent_guardian; terminal_notice; monitor_instance
}
# 只读发现工具、归属和状态，不初始化资源。
read_only() {
    print "动作=${ACTION}；不会创建state、启动/停止服务、生成密钥或安装环境。"
    find_tools
    assert_owner
    print "源码=$REPO"
    print "state=$STATE"
    print "HTTPS=${HTTPS_PORT}；独立PG=$MAIN_PORT/${JOURNAL_PORT}；内部端口=${ROLE_PORT}～$ROLE_LAST"
    print '当前是单机原生开发切片：两独立PG集群、按角色数据库与用户、TLS、WebDist；不宣称完整生产／强保护／RPO0。'
    if process_matches; then print '匹配本实例后端仍运行。'; else print '没有已匹配的本实例后端。'; fi
    print '本实例NATS：'; show_endpoint "$JOBS_IM_NATS_URL"
    print "JetStream=$NATS_STORE；token仅私有文件。"
    if nats_process_matches; then print '匹配本实例NATS运行中。'; else print '没有匹配的本实例NATS。'; fi
}
# 按反依赖顺序回收本轮资源并保留数据。
cleanup() {
    local result="${1:-$?}"
    (( ${CLEANING:-0} == 0 )) || return 0
    CLEANING=1
    set +e
    if (( RUN_ACTIVE == 1 )); then
        stop_parent_guardian
        expire_access_qr
        if [[ -n "${BROWSER_MENU_PID:-}" && "$(ps -p "$BROWSER_MENU_PID" -o ppid= 2>/dev/null | tr -d " ")" == "$$" ]]; then kill -TERM "$BROWSER_MENU_PID" 2>/dev/null; fi
        stop_tunnel || print -u2 "本轮隧道未完全退出；保留并记录。"
        if [[ -d "$STATE/Lock" && "$(cat "$STATE/Lock/pid" 2>/dev/null)" != "$$" ]]; then
            print '服务退出；已将PG收尾交给持锁的本实例控制事务。'
        else
            if stop_backend && stop_nats; then stop_clusters; else print -u2 '应用或broker未证实停止，保留下游资源。'; fi
        fi
    fi
    RUN_ACTIVE=0
    release_lock
    return "$result"
}
# 只在参数校验后注册本轮清理，不在加载函数时改变调用者trap。
install_cleanup_hooks() {
    trap 'result=$?; trap - EXIT; cleanup "$result"; exit "$result"' EXIT
    trap 'result=$?; if (( ${CLEANING:-0} == 0 )); then cleanup "$result"; exit "$result"; fi' ERR
    trap 'cleanup 130; exit 130' INT
    trap 'cleanup 143; exit 143' TERM HUP
}
# 展示入口能力，检查模式不写状态或启停资源。
show_script_intro() {
    print "JobsIM $DISTRO 原生成对部署：fzf选择本地／服务器，本地默认仅本机；反部署单独选择保留或备份／清零清理。"
    print '影响：本项目独立实例、机器级工具缓存；不安装系统服务、不改全局PATH、CA或防火墙。关闭运行终端，服务将结束。'
}
# 每次部署保存原点；历史实例只登记复用，不能追认成新建资源。
resource_identity() { stat -c '%d:%i:%u' -- "$1"; }
resource_top_known() {
    case "$1" in
        Processes|Logs|Secrets|TLS|Application|Bin|Sockets|Sessions|MainDatabase|JournalDatabase|owner|receipt.txt|connection.json|connection.png|restore-layout.conf|Lock|.Deployment) return 0 ;;
        WebDist.*|.TLS-pending.*|owner-relocation.*|owner-new.*) [[ "$1" != */* ]] ;;
        *) return 1 ;;
    esac
}
resource_path_safe() {
    local relative="$1" component current="$STATE"
    [[ -n "$relative" && "$relative" != /* && "$relative" != *$'\t'* && "$relative" != *$'\n'* && "$relative" != *$'\r'* ]] || return 2
    local -a pieces; IFS='/' read -r -a pieces <<< "$relative"
    for component in "${pieces[@]}"; do
        [[ -n "$component" && "$component" != . && "$component" != .. ]] || return 2
        current="$current/$component"
        [[ ! -L "$current" ]] || return 2
        if [[ -e "$current" ]]; then
            [[ -O "$current" && ( -f "$current" || -d "$current" ) && "$(stat -c %d -- "$current")" == "$(stat -c %d -- "$STATE")" ]] || return 2
        fi
    done
}
capture_resource_origin() {
    RESOURCE_ORIGIN=new; [[ ! -e "$STATE" ]] || RESOURCE_ORIGIN=reused
    RESOURCE_INITIAL=()
    local relative kind identity status
    for relative in Processes Logs Secrets TLS Application Bin Sockets Sessions MainDatabase JournalDatabase owner receipt.txt connection.json connection.png restore-layout.conf; do
        kind=file; case "$relative" in Processes|Logs|Secrets|TLS|Application|Bin|Sockets|Sessions|MainDatabase|JournalDatabase) kind=directory ;; esac
        status=created; identity=-
        if [[ -e "$STATE/$relative" || -L "$STATE/$relative" ]]; then
            resource_path_safe "$relative" || { print -u2 "原点资源不安全，保留：$relative"; return 2; }
            status=reused; identity="$(resource_identity "$STATE/$relative")"
        fi
        RESOURCE_INITIAL+=("$status"$'\t'"$kind"$'\t'"$identity"$'\t'"$relative")
    done
}
resource_ledger_validate() {
    local metadata="$STATE/.Deployment" expected_root expected_uid format status kind identity relative seen='|'
    [[ -d "$metadata" && ! -L "$metadata" && -O "$metadata" && -f "$metadata/resources.v1" && ! -L "$metadata/resources.v1" && -O "$metadata/resources.v1" && -f "$metadata/resources.tsv" && ! -L "$metadata/resources.tsv" && -O "$metadata/resources.tsv" ]] || { print -u2 '缺可信原点清单；保留旧实例，未完成反部署。'; return 2; }
    format="$(sed -n 's/^format=//p' "$metadata/resources.v1")"
    expected_root="$(sed -n 's/^root=//p' "$metadata/resources.v1")"; expected_uid="$(sed -n 's/^uid=//p' "$metadata/resources.v1")"
    [[ "$(stat -c %a -- "$metadata")" == 700 && "$(stat -c %a -- "$metadata/resources.v1")" == 600 && "$(stat -c %a -- "$metadata/resources.tsv")" == 600 ]] || return 2
    [[ "$format" == JobsIM-resources-v1 && "$expected_uid" == "$(id -u)" && "$expected_root" == "$(resource_identity "$STATE")" ]] || { print -u2 '原点根目录身份变化，保留现场。'; return 2; }
    while IFS=$'\t' read -r status kind identity relative; do
        [[ "$status" == created || "$status" == reused ]] && [[ "$kind" == file || "$kind" == directory ]] && [[ "$identity" == - || "$identity" =~ ^[0-9]+:[0-9]+:[0-9]+$ ]] || return 2
        resource_top_known "$relative" && [[ "$relative" != Lock && "$relative" != .Deployment && "$relative" != */* && "$seen" != *"|$relative|"* ]] || return 2
        seen+="$relative|"; resource_path_safe "$relative" || return 2
        if [[ -e "$STATE/$relative" && "$identity" != - ]]; then
            [[ "$(resource_identity "$STATE/$relative")" == "$identity" ]] || { print -u2 "资源身份偏离清单，保留：$relative"; return 2; }
        fi
    done < "$metadata/resources.tsv"
}
initialize_resource_ledger() {
    (( LOCK_HELD == 1 )) || return 2
    if [[ -e "$STATE/.Deployment" || -L "$STATE/.Deployment" ]]; then resource_ledger_validate; return $?; fi
    [[ ${#RESOURCE_INITIAL[@]} -gt 0 ]] || { print -u2 '没有部署前原点，拒绝生成追认清单。'; return 2; }
    mkdir "$STATE/.Deployment"
    printf 'format=JobsIM-resources-v1\nuid=%s\nroot=%s\norigin=%s\ndeployment=-\n' "$(id -u)" "$(resource_identity "$STATE")" "$RESOURCE_ORIGIN" > "$STATE/.Deployment/resources.v1"
    printf '%s\n' "${RESOURCE_INITIAL[@]}" > "$STATE/.Deployment/resources.tsv"
    local row status kind identity relative
    for row in "${RESOURCE_INITIAL[@]}"; do
        IFS=$'\t' read -r status kind identity relative <<< "$row"
        if [[ "$status" == created && -e "$STATE/$relative" ]]; then record_resource "$relative"; fi
    done
}
# 仅由实际创建点登记；不扫描未知目录并追认归属。
record_resource() {
    local relative="$1" metadata="$STATE/.Deployment" status kind former row output="$STATE/.Deployment/.resources-$$"
    (( LOCK_HELD == 1 )) && resource_top_known "$relative" && resource_path_safe "$relative" || return 2
    [[ -e "$STATE/$relative" && "$relative" != Lock && "$relative" != .Deployment ]] || return 2
    kind=file; [[ ! -d "$STATE/$relative" ]] || kind=directory
    row="$(awk -F '\t' -v p="$relative" '$4==p{print}' "$metadata/resources.tsv")"
    if [[ -n "$row" ]]; then
        IFS=$'\t' read -r status kind former relative <<< "$row"
        if [[ "$status" == reused ]]; then return 0; fi
        [[ "$former" == - || "$former" == "$(resource_identity "$STATE/$relative")" ]] || { print -u2 "登记前资源已替换，保留：$relative"; return 2; }
        awk -F '\t' -v p="$relative" '$4!=p{print}' "$metadata/resources.tsv" > "$output"
    else
        case "$relative" in WebDist.*|.TLS-pending.*|owner-relocation.*|owner-new.*) ;; *) return 2 ;; esac
        status=created; cat "$metadata/resources.tsv" > "$output"
    fi
    printf '%s\t%s\t%s\t%s\n' "$status" "$kind" "$(resource_identity "$STATE/$relative")" "$relative" >> "$output"
    mv -- "$output" "$metadata/resources.tsv"
}
bind_resource_deployment() {
    local deployment="$1" file="$STATE/.Deployment/resources.v1" original output="$STATE/.Deployment/.binding-$$"
    resource_ledger_validate || return $?
    original="$(sed -n 's/^deployment=//p' "$file")"
    [[ "$deployment" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$ && ( "$original" == - || "$original" == "$deployment" ) ]] || return 2
    sed "s/^deployment=.*/deployment=$deployment/" "$file" > "$output"; mv -- "$output" "$file"
}
undeploy_protected() {
    # 独立journal及匹配秘密需另外授权销毁，普通清零不具备此授权。
    case "$1" in JournalDatabase|JournalDatabase/*|Secrets|Secrets/*|TLS|TLS/*|owner|Application|Application/instance.json) return 0 ;; *) return 1 ;; esac
}
undeploy_select() {
    local selected
    selected="$(printf '%s\n' $'stop\t仅停止并保留部署资料' $'cleanup\t进入反部署资源清理' | FZF_DEFAULT_OPTS='' FZF_DEFAULT_OPTS_FILE=/dev/null FZF_DEFAULT_COMMAND='' "$FZF_BIN" --no-sort --no-multi --delimiter=$'\t' --with-nth=2 --height=40% --border --prompt='反部署范围 > ')" || return 130
    case "$selected" in $'stop\t'*) UNDEPLOY_CHOICE=stop; return 0 ;; $'cleanup\t'*) UNDEPLOY_CHOICE=cleanup ;; *) return 130 ;; esac
    selected="$(printf '%s\n' $'backup\t数据库数据备份（默认）：外置验真后清理本项目源数据' $'wipe\t数据库数据清零：不新增备份，永久清理所选源数据' | FZF_DEFAULT_OPTS='' FZF_DEFAULT_OPTS_FILE=/dev/null FZF_DEFAULT_COMMAND='' "$FZF_BIN" --no-sort --no-multi --delimiter=$'\t' --with-nth=2 --height=40% --border --prompt='数据处理 > ' --header='独立journal、匹配密钥及已有备份始终保留；Esc取消。')" || return 130
    case "$selected" in $'backup\t'*) DATA_POLICY=backup ;; $'wipe\t'*) DATA_POLICY=wipe ;; *) return 130 ;; esac
}
undeploy_confirm() {
    print "反部署实例：$STATE"
    print "处理：${DATA_POLICY:-stop}；仅清单created资源；JournalDatabase、Secrets、TLS、owner、Application/instance.json及外置备份和恢复钥保留。"
    print '清理后实例不可直接继续运行；独立journal不可回滚。'
    confirm_enter '确认此准确范围（--yes不跳过）'
}
undeploy_snapshot() {
    local base="$1" destination="$2" absolute relative top kind identity mode size digest links
    local listing="$destination.paths"
    [[ -d "$base" && ! -L "$base" && -O "$base" ]] || return 2
    find -P "$base" -mindepth 1 -print0 | LC_ALL=C sort -z > "$listing" || return 2
    : > "$destination" || return $?
    while IFS= read -r -d '' absolute; do
        relative="${absolute#"$base/"}"; top="${relative%%/*}"
        case "$top" in Lock|.Deployment) continue ;; esac
        resource_top_known "$top" || { print -u2 "未知顶层资源保留，反部署未完成：$relative"; return 2; }
        [[ "$relative" != *$'\t'* && "$relative" != *$'\n'* && "$relative" != *$'\r'* && ! -L "$absolute" && -O "$absolute" ]] || return 2
        kind=file; size=0; digest=-
        if [[ -d "$absolute" ]]; then kind=directory
        elif [[ -f "$absolute" ]]; then
            links="$(stat -c %h -- "$absolute")"; [[ "$links" == 1 ]] || { print -u2 '发现硬链接，保留资源。'; return 2; }
            size="$(stat -c %s -- "$absolute")"; digest="$(sha256sum -- "$absolute" | cut -d' ' -f1)"
        else print -u2 '存在特殊资源，保留不清理。'; return 2; fi
        [[ "$(stat -c %d -- "$absolute")" == "$(stat -c %d -- "$base")" ]] || { print -u2 '存在挂载边界，保留不清理。'; return 2; }
        identity="$(resource_identity "$absolute")" || return $?
        mode="$(stat -c %a -- "$absolute")" || return $?
        printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$kind" "$identity" "$mode" "$size" "$digest" "$relative" >> "$destination" || return $?
    done < "$listing"
    rm -- "$listing" || return $?
}
undeploy_content_manifest() { awk -F '\t' 'BEGIN{OFS="\t"}{print $1,$3,$4,$5,$6}' "$1"; }
undeploy_verify_backup() {
    local original="$1" checkpoint="$STATE/.Deployment/undeploy" file="$BACKUPS/JobsIM-undeploy-$(date +%Y%m%d-%H%M%S)-$$.cms" verify staging
    [[ ! -L "$BACKUPS" && ! -L "$RECOVERY" ]] || return 2
    assert_no_symlink_components "$BACKUPS" || return $?
    assert_no_symlink_components "$RECOVERY" || return $?
    mkdir -p -- "$BACKUPS" "$RECOVERY" || return $?
    chmod 700 "$BACKUPS" "$RECOVERY" || return $?
    local needed="$(awk -F '\t' '{sum+=$4}END{printf "%.0f",(sum+1023)/1024}' "$original")" available="$(/bin/df -Pk -- "$BACKUPS" | awk 'NR==2{print $4}')"
    [[ "$needed" =~ ^[0-9]+$ && "$available" =~ ^[0-9]+$ ]] && (( available > needed*2+4194304 )) || { print -u2 '备份及验真空间不足，保留源数据。'; return 2; }
    if [[ ! -f "$RECOVERY/key.pem" ]]; then
        [[ ! -e "$RECOVERY/cert.pem" ]] || return 2
        "$OPENSSL" req -x509 -newkey rsa:3072 -nodes -days 3650 -keyout "$RECOVERY/key.pem" -out "$RECOVERY/cert.pem" -subj '/CN=JobsIM Backup Recovery' > "$checkpoint/backup-key.log" 2>&1 || return $?
    fi
    [[ -f "$RECOVERY/key.pem" && ! -L "$RECOVERY/key.pem" && -O "$RECOVERY/key.pem" && -f "$RECOVERY/cert.pem" && ! -L "$RECOVERY/cert.pem" && -O "$RECOVERY/cert.pem" ]] || return 2
    "$OPENSSL" pkey -in "$RECOVERY/key.pem" -pubout 2>/dev/null > "$checkpoint/key-public.pem" || return $?
    "$OPENSSL" x509 -in "$RECOVERY/cert.pem" -pubkey -noout > "$checkpoint/cert-public.pem" || return $?
    cmp -s "$checkpoint/key-public.pem" "$checkpoint/cert-public.pem" || { print -u2 '恢复证书与私钥不匹配，保留源数据。'; return 2; }
    (set -o pipefail; tar --exclude='./Lock' --exclude='./.Deployment' -czf - -C "$STATE" . -C "$checkpoint" broker-manifest.txt | "$OPENSSL" cms -encrypt -aes-256-gcm -binary -outform DER -out "$file" "$RECOVERY/cert.pem") || return $?
    verify="$(mktemp -d "$BACKUPS/.verify-XXXXXX")" || return $?
    (set -o pipefail; "$OPENSSL" cms -decrypt -binary -inform DER -in "$file" -inkey "$RECOVERY/key.pem" | tar -xzf - -C "$verify") || { print -u2 '备份解密失败，源及现场保留。'; return 2; }
    [[ -f "$verify/broker-manifest.txt" && ! -L "$verify/broker-manifest.txt" ]] && cmp -s "$checkpoint/broker-manifest.txt" "$verify/broker-manifest.txt" || { print -u2 '冷备份broker清单未验真，保留源。'; return 2; }
    rm -- "$verify/broker-manifest.txt" || return $?
    undeploy_snapshot "$verify" "$checkpoint/backup-content.tsv" || return $?
    undeploy_content_manifest "$original" > "$checkpoint/source-content.tsv" || return $?
    undeploy_content_manifest "$checkpoint/backup-content.tsv" > "$checkpoint/verified-content.tsv" || return $?
    cmp -s "$checkpoint/source-content.tsv" "$checkpoint/verified-content.tsv" || { print -u2 '备份内容与所选源快照不同，禁止清理。'; return 2; }
    sha256sum -- "$file" > "$file.sha256" || return $?
    printf '%s\n' "$file" > "$checkpoint/backup.path" || return $?
    # 验真副本逐文件清除；原加密包、恢复钥及失败现场保留。
    find -P "$verify" -depth -type f -exec rm -- {} + || return $?
    find -P "$verify" -depth -type d -exec rmdir -- {} + || return $?
    print "已外置解密并逐文件验真：$file"
}
undeploy_delete_plan() {
    local checkpoint="$STATE/.Deployment/undeploy" plan="$STATE/.Deployment/undeploy/plan.tsv" remaining="$STATE/.Deployment/undeploy/current.tsv" status kind identity mode size digest relative top current row
    undeploy_snapshot "$STATE" "$remaining" || return $?
    while IFS=$'\t' read -r kind identity mode size digest relative; do
        row="$kind"$'\t'"$identity"$'\t'"$mode"$'\t'"$size"$'\t'"$digest"$'\t'"$relative"
        grep -Fx -- "$row" "$plan" >/dev/null || { print -u2 "清理范围出现新对象或内容已变，保留：$relative"; return 2; }
    done < "$remaining"
    while IFS=$'\t' read -r kind identity mode size digest relative; do
        top="${relative%%/*}"; undeploy_protected "$relative" && continue
        status="$(awk -F '\t' -v p="$top" '$4==p{print $1}' "$STATE/.Deployment/resources.tsv")"
        [[ "$status" == created ]] || continue
        resource_path_safe "$relative" || return 2
        if [[ ! -e "$STATE/$relative" ]]; then
            grep -Fx -- "$relative" "$checkpoint/attempted.txt" >/dev/null || { print -u2 "对象未由本轮登记移除，保留现场：$relative"; return 2; }
            continue
        fi
        current="$(resource_identity "$STATE/$relative")"; [[ "$current" == "$identity" ]] || return 2
        if [[ "$kind" == file ]]; then
            [[ "$(stat -c %s -- "$STATE/$relative")" == "$size" && "$(sha256sum -- "$STATE/$relative" | cut -d' ' -f1)" == "$digest" ]] || return 2
        fi
        printf '%s\n' "$relative" >> "$checkpoint/attempted.txt"
        if [[ "$kind" == directory ]]; then rmdir -- "$STATE/$relative" || return $?
        else rm -- "$STATE/$relative" || return $?; fi
    done < <(LC_ALL=C sort -t $'\t' -k6,6r "$plan")
}
remove_matching_reference() {
    [[ -n "${REF_ID:-}" && -f "$REF_FILE" && ! -L "$REF_FILE" ]] || return 0
    local deployment="$(sed -n 's/^deployment=//p' "$STATE/.Deployment/resources.v1")"
    [[ "$(sed -n '2p' "$REF_FILE")" == "$REF_ID" && "$(sed -n '3p' "$REF_FILE")" == "$deployment" && "$(sed -n '4p' "$REF_FILE")" == "$(id -u)" ]] || return 2
    rm -- "$REF_FILE" || return $?
    rmdir -- "$REF_DIR" 2>/dev/null || print '本机引用目录仍有其他内容，已保留。'
}
undeploy_instance() {
    [[ -d "$STATE" && -f "$STATE/owner" ]] || { print '实例尚未部署，没有移除资源。'; return 0; }
    resource_ledger_validate || return $?
    undeploy_select || return $?
    undeploy_confirm || return $?
    acquire_lock || return $?
    RUN_ACTIVE=0
    stop_tunnel || return $?
    stop_backend || return $?
    stop_nats || return $?
    stop_clusters || return $?
    if [[ "$UNDEPLOY_CHOICE" == stop ]]; then release_lock; print '仅停止；部署资料和数据保留，未执行反部署清理。'; return 0; fi
    local checkpoint="$STATE/.Deployment/undeploy" phase='' count=0
    if [[ -d "$checkpoint" ]]; then
        [[ ! -L "$checkpoint" && -O "$checkpoint" ]] || return 2
        local control
        for control in phase policy plan.tsv attempted.txt; do [[ -f "$checkpoint/$control" && ! -L "$checkpoint/$control" && -O "$checkpoint/$control" && "$(stat -c %a "$checkpoint/$control")" == 600 ]] || { print -u2 '反部署检查点不是可信普通文件，保留现场。'; return 2; }; done
        phase="$(cat "$checkpoint/phase" 2>/dev/null || true)"
        [[ "$phase" == prepared || "$phase" == deleting || "$phase" == partial ]] || { print -u2 '旧反部署检查点尚未完成准备，保留源与备份供核对。'; return 2; }
        [[ "$(cat "$checkpoint/policy")" == "$DATA_POLICY" ]] || { print -u2 '中断重试不得改变原数据策略。'; return 2; }
    else
        mkdir "$checkpoint" || return $?
        printf '%s\n' "$DATA_POLICY" > "$checkpoint/policy" || return $?
        write_nats_backup_manifest "$checkpoint/broker-manifest.txt" || return $?
        undeploy_snapshot "$STATE" "$checkpoint/plan.tsv" || return $?
        if [[ "$DATA_POLICY" == backup ]]; then undeploy_verify_backup "$checkpoint/plan.tsv" || return $?; fi
        undeploy_snapshot "$STATE" "$checkpoint/check.tsv" || return $?
        cmp -s "$checkpoint/plan.tsv" "$checkpoint/check.tsv" || { print -u2 '确认后的源内容变化，源与原备份保留，不重新备份后续删。'; return 2; }
        : > "$checkpoint/attempted.txt" || return $?
        print prepared > "$checkpoint/phase" || return $?
    fi
    print deleting > "$checkpoint/phase" || return $?
    undeploy_delete_plan || return $?
    remove_matching_reference || return $?
    local child
    while IFS= read -r -d '' child; do
        case "${child##*/}" in Lock|.Deployment) continue ;; esac
        print "保留资源／原点尚未完全还原：$child"; (( count+=1 ))
    done < <(find -P "$STATE" -mindepth 1 -maxdepth 1 -print0)
    if (( count > 0 )); then print partial > "$checkpoint/phase"; release_lock; print -u2 '反部署部分完成；受保护journal／密钥、复用或未知资源保留。禁止将此实例当新部署重建。'; return 2; fi
    print complete > "$checkpoint/phase"; release_lock
    print '已清理清单中本次新建资源；原点审计、外置备份及恢复钥保留。'
}

# 只对本入口持锁且已有清单的实际创建点登记。
record_resource_if_managed() {
    [[ -f "$STATE/.Deployment/resources.v1" && "${LOCK_HELD:-0}" == 1 ]] || return 0
    record_resource "$1"
}
record_replacement() {
    [[ -f "$STATE/.Deployment/resources.v1" && "${LOCK_HELD:-0}" == 1 ]] || return 0
    local relative="$1" old="$2" row status kind identity output="$STATE/.Deployment/.replace-$$"
    row="$(awk -F '\t' -v p="$relative" '$4==p{print}' "$STATE/.Deployment/resources.tsv")"
    IFS=$'\t' read -r status kind identity relative <<< "$row"
    [[ "$identity" == "$old" ]] && resource_path_safe "$relative" || return 2
    printf '%s\t%s\t%s\n' "$relative" "$old" "$(resource_identity "$STATE/$relative")" >> "$STATE/.Deployment/replacements.tsv"
    awk -F '\t' -v p="$relative" '$4!=p{print}' "$STATE/.Deployment/resources.tsv" > "$output"
    printf '%s\t%s\t%s\t%s\n' "$status" "$kind" "$(resource_identity "$STATE/$relative")" "$relative" >> "$output"
    mv -- "$output" "$STATE/.Deployment/resources.tsv"
}

confirm_enter() {
    local answer
    if [[ -t 1 && -z "${NO_COLOR+x}" && "${TERM:-dumb}" != dumb ]]; then
        printf '\033[1;36m%s\033[0m [\033[1;32m回车继续\033[0m；\033[1;31m其它输入或EOF取消\033[0m]：' "$1"
    else printf '%s [回车继续；其它输入或EOF取消]：' "$1"; fi
    IFS= read -r answer || return 130
    [[ -z "$answer" ]] || return 130
}
linux_platform_check() {
    [[ "$(uname -s)" == Linux && "${BASH_VERSINFO[0]}" -ge 4 ]] || { print -u2 '须在对应Linux发行版使用Bash4+；当前宿主未执行Linux部署。'; return 2; }
    local actual="$(sed -n 's/^ID=//p' /etc/os-release | tr -d '"')"
    case "$DISTRO:$actual" in Ubuntu:ubuntu|Debian:debian|RockyLinux:rocky|AlibabaCloudLinux:alinux|AlibabaCloudLinux:alibaba) ;; *) print -u2 "发行版不匹配：本入口=$DISTRO，当前=$actual。"; return 2 ;; esac
}
assert_no_symlink_components() {
    local path="$1" component current=''; local -a pieces
    [[ "$path" == /* ]] || return 2
    IFS='/' read -r -a pieces <<< "$path"
    for component in "${pieces[@]}"; do [[ -n "$component" ]] || continue; current="$current/$component"; [[ ! -L "$current" ]] || { print -u2 '路径含符号链接祖先，保留并停止。'; return 2; }; done
}
configure_mode_state() {
    REPO_ID="$(printf '%s' "$REPO" | sha256sum | cut -c1-16)"
    REFERENCE_NAME="linux-$DISTRO-$DEPLOY_MODE.ref"; read_instance_reference
    if (( STATE_EXPLICIT == 0 )); then
        STATE="${XDG_STATE_HOME:-$HOME/.local/state}/JobsIM/Instances/$REPO_ID-$DISTRO-$DEPLOY_MODE"; DEFAULT_INSTANCE_STATE="$STATE"
    else DEFAULT_INSTANCE_STATE=''; fi
}
bootstrap_tool() {
    local tool="$1" row version filename digest url kind archive stage candidate
    row="$(awk -F '\t' -v t="$tool" -v a="$MACHINE_ARCH" '$1==t&&$2=="linux"&&$3==a{print}' "$REPO/部署/deploy-tools.lock.tsv")"
    [[ "$(printf '%s\n' "$row" | wc -l | tr -d ' ')" == 1 && -n "$row" ]] || return 2
    install_tool_row "$row"
}
tool_version() {
    case "$1" in
        go) GOROOT= GOTOOLCHAIN=local "$2" version | sed -nE 's/^go version go([0-9]+\.[0-9]+\.[0-9]+).*/\1/p' ;;
        node) "$2" --version | sed -nE 's/^v([0-9]+\.[0-9]+\.[0-9]+)$/\1/p' ;;
        fzf) "$2" --version | sed -nE 's/^([0-9]+\.[0-9]+\.[0-9]+).*/\1/p' ;;
        nats-server) "$2" --version | sed -nE 's/.*v([0-9]+\.[0-9]+\.[0-9]+).*/\1/p' ;;
        cloudflared) "$2" --version | sed -nE 's/^cloudflared version ([0-9]+\.[0-9]+\.[0-9]+).*/\1/p' ;;
    esac
}
tool_healthy() {
    [[ -n "$2" && -x "$2" && -n "$(tool_version "$1" "$2")" ]] || return 1
    case "$1" in
        go) GOROOT= GOTOOLCHAIN=local "$2" env GOOS GOARCH >/dev/null ;;
        node) "$2" -e 'if(!globalThis.fetch||!AbortSignal.any||!globalThis.crypto)process.exit(1)' && [[ -x "$(dirname -- "$2")/npm" ]] && PATH="$(dirname -- "$2"):$PATH" "$(dirname -- "$2")/npm" --version >/dev/null ;;
        fzf) FZF_DEFAULT_OPTS='' FZF_DEFAULT_OPTS_FILE=/dev/null "$2" --help | grep -F -- --expect >/dev/null ;;
        cloudflared) "$2" tunnel --help 2>&1 | grep -F -- --origin-ca-pool >/dev/null ;;
        nats-server) "$2" --help 2>&1 | grep -F -- --config >/dev/null ;;
    esac
}
install_tool_row() {
    local row="$1" tool os arch version filename digest url kind target archive stage candidate count status bytes headers
    IFS=$'\t' read -r tool os arch version filename digest url kind <<< "$row"
    [[ "$os/$arch" == "linux/$MACHINE_ARCH" && "$version" =~ ^[0-9]+\.[0-9]+(\.[0-9]+)?$ && "$digest" =~ ^[0-9a-f]{64}$ && "$filename" != */* && "$url" == https://* && "$kind" != unsupported ]] || return 2
    case "$tool" in go|fzf|cloudflared|node|nats-server) ;; *) return 2 ;; esac
    target="$TOOLS_ROOT/$tool/$version-$MACHINE_ARCH"
    [[ ! -L "$target" && ! -L "$TOOLS_ROOT/$tool" ]] || return 2
    if [[ -f "$target/receipt.tsv" ]] && cmp -s <(printf '%s\n' "$row") "$target/receipt.tsv"; then
        TOOL_RESULT="$(find -P "$target" -type f -name "$tool" ! -path '*/node_modules/*' -perm /111 | head -1)"
        [[ "$tool" != node ]] || TOOL_RESULT="$target/Content/bin/node"
        [[ "$tool" != go ]] || TOOL_RESULT="$target/Content/go/bin/go"
        tool_healthy "$tool" "$TOOL_RESULT" && return 0
        print -u2 '原机器工具缓存损坏；保留原件，不覆盖。'; return 2
    fi
    [[ ! -e "$target" ]] || { print -u2 '机器工具目标已有未知对象，保留。'; return 2; }
    mkdir -p "$TOOLS_ROOT/Downloads" "$TOOLS_ROOT/$tool" || return $?
    archive="$TOOLS_ROOT/Downloads/$digest-$filename"
    if [[ ! -f "$archive" || "$(sha256sum "$archive" | cut -d' ' -f1)" != "$digest" ]]; then
        headers="$(curl --fail --head --location --proto '=https' --tlsv1.2 --connect-timeout 10 --max-time 30 "$url")" || return $?
        bytes="$(printf '%s\n' "$headers" | tr -d '\r' | awk 'tolower($1)~/^http\//{n=""}tolower($1)=="content-length:"{n=$2}END{print n}')"
        [[ "$bytes" =~ ^[1-9][0-9]{0,14}$ ]] || { print -u2 '官方资产未提供可核对字节数，无法预留下载空间；保留现有工具。'; return 2; }
        disk_budget_bytes "$TOOLS_ROOT/Downloads" "$bytes" || return $?
        print "来源｜$tool $version｜$url → $archive"
        local delay=1
        while :; do
            status=0; curl --fail --location --proto '=https' --tlsv1.2 --connect-timeout 10 --max-time 60 --max-filesize "$bytes" --output "$archive.part" "$url" || status=$?
            if (( status == 0 )); then break; fi
            case "$status" in 6|7|18|28|35|52|55|56) print "网络暂不可用，${delay}秒后重试；Ctrl+C取消。"; sleep "$delay"; (( delay < 15 )) && delay=$((delay*2)); (( delay <= 15 )) || delay=15 ;; *) return "$status" ;; esac
        done
        [[ "$(sha256sum "$archive.part" | cut -d' ' -f1)" == "$digest" ]] || { print -u2 '官方下载摘要不一致，保留部分文件并停止。'; return 2; }
        mv -- "$archive.part" "$archive" || return $?
    fi
    if [[ "$kind" == binary ]]; then bytes="$(stat -c %s -- "$archive")" || return $?
    else bytes="$(set -o pipefail; gzip -dc -- "$archive" | wc -c | tr -d ' ')" || return $?; fi
    disk_budget_bytes "$TOOLS_ROOT/$tool" "$bytes" || return $?
    stage="$(mktemp -d "$TOOLS_ROOT/$tool/.prepare-XXXXXX")" || return $?
    mkdir "$stage/Content" || return $?
    if [[ "$kind" == binary ]]; then cp -- "$archive" "$stage/Content/$tool" || return $?; chmod 700 "$stage/Content/$tool" || return $?
    else
        [[ "$kind" == tar.gz || "$kind" == tgz ]] || return 2
        tar -tzf "$archive" > "$stage/archive.list" || return $?
        if grep -Eq '(^/|(^|/)\.\.(/|$))' "$stage/archive.list"; then print -u2 '归档路径异常，保留现场。'; return 2; fi
        if [[ "$tool" == node ]]; then tar -xzf "$archive" --strip-components=1 -C "$stage/Content" || return $?
        else tar -xzf "$archive" -C "$stage/Content" || return $?; fi
    fi
    candidate="$(find -P "$stage/Content" -type f -name "$tool" ! -path '*/node_modules/*' -perm /111 | head -1)"
    tool_healthy "$tool" "$candidate" || { print -u2 '机器工具候选实际健康检查失败，不采用。'; return 2; }
    printf '%s\n' "$row" > "$stage/receipt.tsv" || return $?
    mv -T -- "$stage" "$target" || return $?
    TOOL_RESULT="${candidate/"$stage"/"$target"}"
}
prepare_menu_tools() {
    linux_platform_check || return $?
    case "$(uname -m)" in x86_64) MACHINE_ARCH=amd64 ;; aarch64|arm64) MACHINE_ARCH=arm64 ;; *) print -u2 '不支持的CPU架构。'; return 2 ;; esac
    TOOLS_ROOT="${JOBS_IM_TOOLS_ROOT:-${XDG_DATA_HOME:-$HOME/.local/share}/JobsTools/JobsIM}"
    assert_no_symlink_components "$TOOLS_ROOT"
    [[ "$TOOLS_ROOT" == /* && "$TOOLS_ROOT" != "$REPO" && "$TOOLS_ROOT" != "$REPO"/* && "$TOOLS_ROOT" != "$STATE" && "$TOOLS_ROOT" != "$STATE"/* && "$STATE" != "$TOOLS_ROOT"/* && ! -L "$TOOLS_ROOT" ]] || return 2
    FZF_BIN="${JOBS_IM_FZF_BIN:-$(command -v fzf || true)}"
    if ! tool_healthy fzf "$FZF_BIN"; then bootstrap_tool fzf || return $?; FZF_BIN="$TOOL_RESULT"; fi
}
select_deploy_mode() {
    local selected
    selected="$(printf '%s\n' $'local\t本地部署' $'server\t服务器部署（已有域名／可信HTTPS）' | FZF_DEFAULT_OPTS='' FZF_DEFAULT_OPTS_FILE=/dev/null FZF_DEFAULT_COMMAND='' "$FZF_BIN" --no-sort --no-multi --delimiter=$'\t' --with-nth=2 --height=40% --border --prompt='部署模式 > ')" || return 130
    case "$selected" in $'local\t'*) DEPLOY_MODE=local ;; $'server\t'*) DEPLOY_MODE=server ;; *) return 130 ;; esac
    configure_mode_state
    if [[ "$DEPLOY_MODE" == server && -z "${JOBS_IM_PUBLIC_URL:-}" ]]; then
        printf '%s' '服务器正式HTTPS地址（仅origin；空行取消）：'; IFS= read -r JOBS_IM_PUBLIC_URL || return 130
    fi
    configure_public_mode
}
select_local_access() {
    [[ "$DEPLOY_MODE" == local && "$ACCESS_EXPLICIT" == 0 ]] || return 0
    local selected
    selected="$(printf '%s\n' $'local\t仅本机访问（默认）' $'quick\t公开临时穿透（另行确认）' | FZF_DEFAULT_OPTS='' FZF_DEFAULT_OPTS_FILE=/dev/null FZF_DEFAULT_COMMAND='' "$FZF_BIN" --no-sort --no-multi --delimiter=$'\t' --with-nth=2 --height=40% --border --prompt='本地访问范围 > ')" || return 130
    case "$selected" in local$'\t'*) LOCAL_ACCESS=local ;; quick$'\t'*) LOCAL_ACCESS=quick ;; *) return 130 ;; esac
}
select_instance_mode() {
    local selected
    selected="$(printf '%s\n' $'local\t本地部署实例（本机／临时穿透）' $'server\t服务器部署实例' | FZF_DEFAULT_OPTS='' FZF_DEFAULT_OPTS_FILE=/dev/null "$FZF_BIN" --no-sort --no-multi --delimiter=$'\t' --with-nth=2 --height=40% --border --prompt='选择原实例模式 > ')" || return 130
    case "$selected" in local$'\t'*) DEPLOY_MODE=local ;; server$'\t'*) DEPLOY_MODE=server ;; *) return 130 ;; esac
    configure_mode_state
}
configure_public_mode() {
    case "$DEPLOY_MODE" in local) PUBLIC_HOST=127.0.0.1; PUBLIC_URL="https://127.0.0.1:$HTTPS_PORT" ;; server)
        PUBLIC_URL="${JOBS_IM_PUBLIC_URL:-}"
        [[ "$PUBLIC_URL" =~ ^https://([a-zA-Z0-9][a-zA-Z0-9.-]*)(:[1-9][0-9]{0,4})?/?$ ]] || { print -u2 '服务器模式须显式提供真实HTTPS origin，不接受凭据、路径、查询或片段。'; return 2; }
        PUBLIC_HOST="${BASH_REMATCH[1]}"; PUBLIC_URL="${PUBLIC_URL%/}"
        case "$PUBLIC_HOST" in localhost|127.*|0.0.0.0) print -u2 '服务器模式拒绝回环或监听地址，不能把本地模式贴成服务器标签。'; return 2 ;; esac
        ;;
        *) return 2 ;;
    esac
}
machine_budget_check() {
    local cores="$(getconf _NPROCESSORS_ONLN)" total available disk
    total="$(awk '/^MemTotal:/{print $2}' /proc/meminfo)"; available="$(awk '/^MemAvailable:/{print $2}' /proc/meminfo)"
    disk="$(/bin/df -Pk "$STATE" | awk 'NR==2{print $4}')"
    print "机器体检｜CPU=$cores｜总内存KiB=$total｜可用内存KiB=$available｜可用磁盘KiB=$disk；未测定最低硬件档。"
    [[ "$disk" =~ ^[0-9]+$ ]] && (( disk>=4194304 )) || { print -u2 '可用磁盘低于4GiB，暂停新的重型下载／构建；保留数据与缓存。'; return 2; }
}
disk_budget_bytes() {
    local directory="$1" bytes="$2" available
    [[ "$bytes" =~ ^[0-9]{1,15}$ ]] || return 2
    available="$(/bin/df -Pk -- "$directory" | awk 'NR==2{print $4}')" || return $?
    [[ "$available" =~ ^[0-9]+$ ]] && (( available*1024 >= bytes+4294967296 )) || { print -u2 '按当前资产实际字节预留后不足4GiB，暂停新下载／解包；不清缓存。'; return 2; }
}
files_actual_bytes() {
    local list file size total=0
    list="$(mktemp "$BACKUPS/.byte-budget-XXXXXX")" || return $?
    find -P "$@" -type f -print0 > "$list" || return $?
    while IFS= read -r -d '' file; do
        [[ ! -L "$file" ]] || return 2
        size="$(stat -c %s -- "$file")" || return $?
        [[ "$size" =~ ^[0-9]{1,15}$ ]] || return 2
        total=$((total+size))
    done < "$list"
    rm -- "$list" || return $?
    printf '%s\n' "$total"
}
prepare_current_tools() {
    local go="${JOBS_IM_GO:-$(command -v go || true)}" tool row status delay version explicit candidate target best best_version args
    if ! tool_healthy go "$go"; then bootstrap_tool go || return $?; go="$TOOL_RESULT"; fi
    GO_TOOL="$go"
    SESSIONS="$STATE/Sessions"; mkdir -p "$SESSIONS"; record_resource Sessions
    RUN_DIR="$(mktemp -d "$SESSIONS/run-XXXXXX")"; LOG_FILE="$RUN_DIR/operation.log"; : > "$LOG_FILE"
    (cd "$REPO/部署/公共组件/依赖版本" && GOROOT= GOWORK=off GOTOOLCHAIN=local "$go" build -mod=readonly -o "$RUN_DIR/versions" .) || return $?
    VERSION_TOOL="$RUN_DIR/versions"
    for tool in go fzf node nats-server postgres; do
        delay=1
        while :; do
            status=0
            row="$("$VERSION_TOOL" --tool "$tool" --os linux --arch "$MACHINE_ARCH" --locked-manifest "$REPO/部署/deploy-tools.lock.tsv")" || status=$?
            (( status == 0 )) && break
            (( status == 75 )) || { print -u2 "官方$tool稳定版本未能确认，停止；不冒充最新。"; return "$status"; }
            print "官方$tool查询暂不可用，${delay}秒后重试；Ctrl+C取消。"; sleep "$delay"; (( delay < 15 )) && delay=$((delay*2)); (( delay <= 15 )) || delay=15
        done
        printf '%s\n' "$row" >> "$RUN_DIR/official-targets.tsv"
        IFS=$'\t' read -r _ _ _ target _ _ _ kind <<< "$row"
        if [[ "$tool" == postgres ]]; then print "PostgreSQL官方当前稳定目标=$target；当前业务固定PG18，采用发行版既有PG18并核验能力，不升级用户数据库主版本。"; continue; fi
        explicit=''; case "$tool" in go) explicit="${JOBS_IM_GO:-}" ;; node) explicit="${JOBS_IM_NODE:-}" ;; fzf) explicit="${JOBS_IM_FZF_BIN:-}" ;; nats-server) explicit="${JOBS_IM_NATS_BIN:-}" ;; cloudflared) explicit="${JOBS_IM_CLOUDFLARED:-}" ;; esac
        best=''; best_version=''
        for candidate in "$explicit" "$(command -v "$tool" || true)" "$TOOLS_ROOT/$tool/"*/Content/{,bin/,go/bin/}"$tool"; do
            tool_healthy "$tool" "$candidate" || continue
            version="$(tool_version "$tool" "$candidate")"
            "$VERSION_TOOL" --compare "$version" --at-least "$target" >/dev/null || continue
            if [[ -z "$best" ]] || "$VERSION_TOOL" --compare "$version" --at-least "$best_version" >/dev/null; then best="$candidate"; best_version="$version"; fi
        done
        if [[ -n "$explicit" ]]; then
            tool_healthy "$tool" "$explicit" && "$VERSION_TOOL" --compare "$(tool_version "$tool" "$explicit")" --at-least "$target" >/dev/null || { print -u2 "显式$tool未达到官方目标或健康要求，未覆盖原文件。"; return 2; }
            best="$explicit"
        fi
        if [[ -z "$best" ]]; then install_tool_row "$row" || return $?; best="$TOOL_RESULT"; fi
        case "$tool" in go) GO_TOOL="$best" ;; node) export PATH="$(dirname -- "$best"):$PATH" ;; fzf) FZF_BIN="$best" ;; nats-server) NATS_BIN="$best"; export JOBS_IM_NATS_BIN="$best" ;; cloudflared) CLOUDFLARED_BIN="$best" ;; esac
        printf '%s\t%s\t%s\tshared-machine-tool\n' "$tool" "$(tool_version "$tool" "$best")" "$best" >> "$STATE/.Deployment/shared-tools.tsv"
    done
    export JOBS_IM_GO="$GO_TOOL"
    find_tools
    [[ "$("$PG_BIN/pg_ctl" --version)" =~ PostgreSQL\)\ 18\. ]] || { print -u2 '须配置既有PG18，未自动安装其他主版本。'; return 2; }
    "$PG_BIN/pg_dump" --version; "$PG_BIN/pg_restore" --version
    local module name temp
    for module in "$REPO/代码/后端/Go" "$REPO/部署/公共组件/地址二维码"; do
        name="$(basename -- "$module")"; temp="$RUN_DIR/$name.mod"; cp "$module/go.mod" "$temp"; [[ ! -f "$module/go.sum" ]] || cp "$module/go.sum" "$RUN_DIR/$name.sum"
        "$VERSION_TOOL" --module-report --go-bin "$GO_TOOL" --module-dir "$module" --modfile "$temp" --timeout-seconds 30 > "$RUN_DIR/module-$name.tsv" || { status=$?; [[ "$status" == 3 ]] || return "$status"; print "业务库$name更新检查超时；按已有锁构建，不视为已确认最新。"; }
    done
    timeout 30 npm --prefix "$REPO/代码/前端/Web" outdated --json > "$RUN_DIR/web-updates.json" || { status=$?; [[ "$status" == 1 || "$status" == 124 ]] || print 'Web业务库只读更新检查未完成；未改变依赖锁。'; }
}
import_server_tls() {
    [[ "$DEPLOY_MODE" == server ]] || return 0
    local cert="${JOBS_IM_SERVER_CERT:-}" key="${JOBS_IM_SERVER_KEY:-}" ca="${JOBS_IM_SERVER_CA:-}" file
    [[ -n "$cert" && -n "$key" && -n "$ca" ]] || { print -u2 '服务器模式需要真实域名配套证书、私钥、CA；设置JOBS_IM_SERVER_CERT/KEY/CA，不自动生成公网信任。'; return 2; }
    for file in "$cert" "$key" "$ca"; do [[ -f "$file" && ! -L "$file" ]] || return 2; done
    [[ -O "$key" && "$(stat -c %a "$key")" == 600 ]] || return 2
    "$OPENSSL" pkey -in "$key" -pubout > "$RUN_DIR/server-key-public.pem" 2>/dev/null || return $?
    "$OPENSSL" x509 -in "$cert" -pubkey -noout > "$RUN_DIR/server-cert-public.pem" || return $?
    cmp -s "$RUN_DIR/server-key-public.pem" "$RUN_DIR/server-cert-public.pem" || { print -u2 '服务器证书与私钥不匹配，未提交到实例。'; return 2; }
    "$OPENSSL" verify -x509_strict -purpose sslserver -verify_hostname "$PUBLIC_HOST" -CAfile "$ca" "$cert" || return $?
    for file in ca.crt server.crt server.key; do
        local source="$ca"; [[ "$file" != server.crt ]] || source="$cert"; [[ "$file" != server.key ]] || source="$key"
        if [[ -e "$STATE/TLS/$file" ]]; then cmp -s "$source" "$STATE/TLS/$file" || { print -u2 '已部署证书不同，保留并停止，不静默覆盖。'; return 2; }
        else cp -- "$source" "$STATE/TLS/$file"; fi
    done
}
start_backend_for_url() {
    local url="$1" count=0
    JOBS_IM_JOURNAL_DATABASE_URL="$JOBS_IM_JOURNAL_DATABASE_URL" JOBS_IM_PUBLIC_URL="$url" "$STATE/Bin/JobsIMd" --service all --addr "127.0.0.1:$HTTPS_PORT" --internal-base-port "$ROLE_PORT" --state-dir "$STATE/Application" --tls-cert "$STATE/TLS/server.crt" --tls-key "$STATE/TLS/server.key" --web-dir "$WEB_RUNTIME" >> "$STATE/Logs/server.log" 2>&1 &
    SERVER_PID=$!
    print "$SERVER_PID" > "$STATE/Processes/server.pid" || return $?
    LC_ALL=C ps -p "$SERVER_PID" -o lstart= > "$STATE/Processes/server.start" || return $?
    while (( count < 60 )); do
        if curl --fail --silent --cacert "$STATE/TLS/ca.crt" --resolve "$PUBLIC_HOST:$HTTPS_PORT:127.0.0.1" --max-time 1 "https://$PUBLIC_HOST:$HTTPS_PORT/api/health" >/dev/null 2>&1; then return 0; fi
        kill -0 "$SERVER_PID" 2>/dev/null || break
        sleep 0.5; (( count+=1 ))
    done
    print -u2 '本轮Go严格HTTPS健康检查失败，保留私有日志。'; return 2
}
load_tunnel_receipt() {
    [[ -z "${TUNNEL_PID:-}" ]] || return 0
    local receipt="$STATE/Processes/tunnel.receipt" binary_hash
    [[ -e "$receipt" ]] || return 0
    [[ -f "$receipt" && ! -L "$receipt" && -O "$receipt" && "$(stat -c %a "$receipt")" == 600 ]] || return 2
    TUNNEL_PID="$(sed -n '1p' "$receipt")"; TUNNEL_START="$(sed -n '2p' "$receipt")"
    CLOUDFLARED_BIN="$(sed -n '3p' "$receipt")"; TUNNEL_RUN_DIR="$(sed -n '4p' "$receipt")"; binary_hash="$(sed -n '5p' "$receipt")"
    [[ "$TUNNEL_PID" =~ ^[1-9][0-9]*$ && "$TUNNEL_RUN_DIR" == "$STATE/Sessions/"* && "$TUNNEL_RUN_DIR" != *'/../'* && -f "$CLOUDFLARED_BIN" && "$(sha256sum "$CLOUDFLARED_BIN" | cut -d' ' -f1)" == "$binary_hash" ]] || return 2
}
tunnel_matches() {
    [[ -n "${TUNNEL_PID:-}" && -n "${TUNNEL_START:-}" && -n "${CLOUDFLARED_BIN:-}" ]] || return 1
    [[ "$(LC_ALL=C ps -p "$TUNNEL_PID" -o lstart= 2>/dev/null || true)" == "$TUNNEL_START" && "$(ps -ww -p "$TUNNEL_PID" -o command= 2>/dev/null || true)" == *"$CLOUDFLARED_BIN tunnel "*"${TUNNEL_RUN_DIR:-$RUN_DIR}/quick-empty.yml"* ]]
}
stop_tunnel() {
    load_tunnel_receipt || { print -u2 '隧道收据不可信，保留现场。'; return 2; }
    if tunnel_matches; then
        kill -TERM "$TUNNEL_PID" || return $?
        local count=0
        while kill -0 "$TUNNEL_PID" 2>/dev/null && (( count < 50 )); do sleep 0.1; (( count+=1 )); done
        ! kill -0 "$TUNNEL_PID" 2>/dev/null || return 2
    elif [[ -n "${TUNNEL_PID:-}" ]] && kill -0 "$TUNNEL_PID" 2>/dev/null; then print -u2 '隧道PID身份改变，保留陌生进程。'; return 2; fi
    TUNNEL_PID=''; TUNNEL_START=''
}
start_quick_tunnel() {
    [[ "$DEPLOY_MODE" == local ]] || return 0
    confirm_enter '本地穿透将生成公开临时地址，取得地址者可以到达本IM登录入口；只使用可公开的联调数据' || return $?
    prepare_quick_tool || return $?
    printf '{}\n' > "$RUN_DIR/quick-empty.yml" || return $?
    (
        local variable
        while IFS='=' read -r variable _; do case "$variable" in TUNNEL_*) unset "$variable" ;; esac; done < <(env)
        exec "$CLOUDFLARED_BIN" tunnel --config "$RUN_DIR/quick-empty.yml" --no-autoupdate --url "https://127.0.0.1:$HTTPS_PORT" --origin-ca-pool "$STATE/TLS/ca.crt" --origin-server-name localhost
    ) > "$RUN_DIR/tunnel.log" 2>&1 &
    TUNNEL_PID=$!; TUNNEL_START="$(LC_ALL=C ps -p "$TUNNEL_PID" -o lstart=)" || return $?
    TUNNEL_RUN_DIR="$RUN_DIR"
    printf '%s\n%s\n%s\n%s\n%s\n' "$TUNNEL_PID" "$TUNNEL_START" "$CLOUDFLARED_BIN" "$RUN_DIR" "$(sha256sum "$CLOUDFLARED_BIN" | cut -d' ' -f1)" > "$STATE/Processes/tunnel.receipt" || return $?
    local count=0 candidate
    while (( count < 120 )); do
        candidate="$(grep -Eo 'https://[a-z0-9-]+\.trycloudflare\.com' "$RUN_DIR/tunnel.log" | tail -1 || true)"
        if [[ -n "$candidate" ]]; then PUBLIC_URL="$candidate"; break; fi
        tunnel_matches || { print -u2 '本轮隧道未能保持健康，停止公开流程。'; return 2; }
        sleep 0.5; (( count+=1 ))
    done
    [[ "$PUBLIC_URL" == https://*.trycloudflare.com ]] || { print -u2 '没有取得本轮真实Quick地址，不猜测公网入口。'; return 2; }
    # BFF严格同源要求真实PublicURL，复用本轮PG/NATS/身份而只重启自己的Go。
    stop_backend || return $?
    start_backend_for_url "$PUBLIC_URL" || return $?
    curl --fail --silent --max-time 10 "$PUBLIC_URL/api/health" >/dev/null || { print -u2 '公网严格HTTPS未通过，不发布地址。'; return 2; }
    LOCAL_ACCESS=quick
}
prepare_quick_tool() {
    local row target candidate version best='' best_version='' explicit="${JOBS_IM_CLOUDFLARED:-}"
    # 本机模式不查也不下载cloudflared；仅明确公开或重试时准备。
    row="$("$VERSION_TOOL" --tool cloudflared --os linux --arch "$MACHINE_ARCH" --locked-manifest "$REPO/部署/deploy-tools.lock.tsv")" || return $?
    IFS=$'\t' read -r _ _ _ target _ _ _ _ <<< "$row"
    printf '%s\n' "$row" >> "$RUN_DIR/official-targets.tsv" || return $?
    for candidate in "$explicit" "$(command -v cloudflared || true)" "$TOOLS_ROOT/cloudflared/"*/Content/cloudflared; do
        tool_healthy cloudflared "$candidate" || continue
        version="$(tool_version cloudflared "$candidate")" || continue
        "$VERSION_TOOL" --compare "$version" --at-least "$target" >/dev/null || continue
        if [[ -z "$best" ]] || "$VERSION_TOOL" --compare "$version" --at-least "$best_version" >/dev/null; then best="$candidate"; best_version="$version"; fi
    done
    if [[ -n "$explicit" ]]; then
        tool_healthy cloudflared "$explicit" && "$VERSION_TOOL" --compare "$(tool_version cloudflared "$explicit")" --at-least "$target" >/dev/null || { print -u2 '显式cloudflared未通过版本／能力检查，保留原件和本地服务。'; return 2; }
        best="$explicit"
    fi
    if [[ -z "$best" ]]; then install_tool_row "$row" || return $?; best="$TOOL_RESULT"; fi
    CLOUDFLARED_BIN="$best"
    printf '%s\t%s\t%s\tshared-machine-tool\n' cloudflared "$(tool_version cloudflared "$best")" "$best" >> "$STATE/.Deployment/shared-tools.tsv" || return $?
}
withdraw_access() {
    expire_access_qr || return $?
    QR_PAGE=''
    if [[ -e "$STATE/connection.json" ]]; then
        [[ -f "$STATE/connection.json" && ! -L "$STATE/connection.json" && -O "$STATE/connection.json" ]] || return 2
        printf '{"status":"unavailable"}\n' > "$STATE/connection.json" || return $?
    fi
}
recover_local_access() {
    [[ "$DEPLOY_MODE" == local ]] || return 2
    withdraw_access || return $?
    stop_tunnel || return $?
    stop_backend || return $?
    PUBLIC_HOST=127.0.0.1; PUBLIC_URL="https://127.0.0.1:$HTTPS_PORT"; LOCAL_ACCESS=local
    start_backend_for_url "$PUBLIC_URL" || { print -u2 '本地严格健康恢复失败，保留数据与日志，不报告可用。'; return 2; }
    write_connection || return $?
    prepare_access_qr || return $?
    show_endpoint "$PUBLIC_URL/"
    print '公开入口已撤回；本地服务健康，仍可在菜单重试穿透、查看诊断或停止。'
}
attempt_quick_access() {
    local guardian_active=0
    [[ -z "${GUARDIAN_PID:-}" ]] || guardian_active=1
    stop_parent_guardian || return $?
    withdraw_access || return $?
    stop_tunnel || return $?
    if start_quick_tunnel; then
        write_connection || return $?
        prepare_access_qr || return $?
        show_endpoint "$PUBLIC_URL/"
    else
        print -u2 '穿透未就绪，收回公开描述并恢复本机入口。'
        recover_local_access || return $?
    fi
    (( guardian_active == 0 )) || start_parent_guardian
}
handle_tunnel_exit() {
    [[ "$DEPLOY_MODE" == local && "$LOCAL_ACCESS" == quick ]] || return 0
    tunnel_matches && return 0
    acquire_lock || return $?
    stop_parent_guardian || return $?
    recover_local_access || return $?
    release_lock || return $?
    start_parent_guardian || return $?
}
prepare_access_qr() {
    QR_BINARY="$RUN_DIR/access-qr"; QR_DIR="$RUN_DIR/AccessQR"
    if ! (cd "$REPO/部署/公共组件/地址二维码" && GOROOT= GOWORK=off GOTOOLCHAIN=local "$GO_TOOL" build -mod=readonly -o "$QR_BINARY" .); then print '二维码构建失败，仍提供完整URL。'; QR_BINARY=''; return 0; fi
    local width="${COLUMNS:-0}"; [[ "$width" =~ ^[0-9]+$ ]] || width=0
    "$QR_BINARY" --output-dir "$QR_DIR" --title JobsIM --entry "用户端|$PUBLIC_URL/" --terminal-width "$width" || { print '二维码生成失败，保留可复制URL。'; return 0; }
    QR_PAGE="$QR_DIR/index.html"
}
expire_access_qr() {
    [[ -n "${QR_BINARY:-}" && -n "${QR_DIR:-}" && -d "$QR_DIR" ]] || return 0
    "$QR_BINARY" --expire --output-dir "$QR_DIR" >/dev/null || print -u2 '旧二维码页标失效失败，请勿复用旧码。'
}
browser_menu_loop() {
    trap - EXIT ERR INT TERM HUP
    local selected selector_pid=''
    trap 'if [[ -n "$selector_pid" ]] && kill -0 "$selector_pid" 2>/dev/null; then kill -TERM "$selector_pid"; wait "$selector_pid" 2>/dev/null || true; fi; exit 143' TERM HUP INT
    printf '%s\n' $'open\t打开用户端' $'qr\t打开本地扫码页面' > "$RUN_DIR/menu-options"
    [[ "$DEPLOY_MODE" != local ]] || printf '%s\n' $'quick\t开启／重试公开临时穿透（另确认）' >> "$RUN_DIR/menu-options"
    printf '%s\n' $'diagnose\t查看本地与穿透诊断' $'stop\t停止本轮部署' >> "$RUN_DIR/menu-options"
    while :; do
        FZF_DEFAULT_OPTS='' FZF_DEFAULT_OPTS_FILE=/dev/null FZF_DEFAULT_COMMAND='' "$FZF_BIN" --no-sort --no-multi --delimiter=$'\t' --with-nth=2 --height=40% --border --prompt='JobsIM访问 > ' --header='关闭终端或Esc停止本轮服务。' < "$RUN_DIR/menu-options" > "$RUN_DIR/menu-selected" &
        selector_pid=$!; selected='stop'; if wait "$selector_pid"; then selected="$(cat "$RUN_DIR/menu-selected")"; fi; selector_pid=''
        case "$selected" in open$'\t'*) printf 'open\n' > "$RUN_DIR/menu-request.new" ;; qr$'\t'*) printf 'qr\n' > "$RUN_DIR/menu-request.new" ;; quick$'\t'*) printf 'quick\n' > "$RUN_DIR/menu-request.new" ;; diagnose$'\t'*) printf 'diagnose\n' > "$RUN_DIR/menu-request.new" ;; *) printf 'stop\n' > "$RUN_DIR/menu-request.new" ;; esac
        mv -- "$RUN_DIR/menu-request.new" "$RUN_DIR/menu-request"
        [[ "$selected" != stop$'\t'* && "$selected" != stop ]] || return 0
        while [[ -e "$RUN_DIR/menu-request" ]]; do sleep 0.1; done
    done
}
monitor_instance() {
    BROWSER_MENU_PID=''
    if [[ -t 0 && -t 1 ]]; then browser_menu_loop & BROWSER_MENU_PID=$!; fi
    while process_matches; do
        handle_tunnel_exit || return $?
        if [[ -f "$RUN_DIR/menu-request" && ! -L "$RUN_DIR/menu-request" ]]; then
            local action="$(cat "$RUN_DIR/menu-request")"; rm -- "$RUN_DIR/menu-request"
            case "$action" in
                open) command -v xdg-open >/dev/null && xdg-open "$PUBLIC_URL/" >/dev/null 2>&1 || print "浏览器不可用，请复制：$PUBLIC_URL/" ;;
                qr) [[ -n "${QR_PAGE:-}" ]] && command -v xdg-open >/dev/null && xdg-open "$QR_PAGE" >/dev/null 2>&1 || print '无图形桌面或二维码页；请复制完整URL。' ;;
                quick) acquire_lock || return $?; attempt_quick_access || return $?; release_lock || return $? ;;
                diagnose) if curl --fail --silent --cacert "$STATE/TLS/ca.crt" --resolve "$PUBLIC_HOST:$HTTPS_PORT:127.0.0.1" --max-time 5 "https://$PUBLIC_HOST:$HTTPS_PORT/api/health" >/dev/null; then print '本地严格HTTPS健康。'; else print '本地健康未通过。'; fi; print "本轮私有日志：$RUN_DIR；后端：$STATE/Logs/server.log" ;;
                stop) return 0 ;;
            esac
        fi
        sleep 0.2
    done
    print -u2 '本轮后端退出，停止剩余本轮资源。'; return 2
}

dispatch_action() {
    case "$ACTION" in
        check|plan|status) read_only ;;
        menu|run)
            prepare_menu_tools
            if [[ "$ACTION" == menu || "$MODE_EXPLICIT" == 0 ]]; then select_deploy_mode; else configure_public_mode; fi
            select_local_access
            ACTION=run; parse_arguments; assert_owner; run_instance
            ;;
        undeploy) prepare_menu_tools; if [[ "$MODE_EXPLICIT" == 0 && "$STATE_EXPLICIT" == 0 ]]; then select_instance_mode; parse_arguments; fi; find_management_tools; assert_owner; undeploy_instance ;;
        stop|backup)
            find_management_tools; assert_owner; [[ -f "$STATE/owner" ]] || { print '本实例尚未创建；没有停止或清理任何资源。'; return 0; }
            confirm_action || return $?
            prepare_state || return $?
            acquire_lock || return $?
            apply_owner_relocation || return $?
            RUN_ACTIVE=1; stop_tunnel || return $?
            if [[ "$ACTION" == backup || "$NO_BACKUP" == 0 ]]; then backup_instance || return $?; else stop_backend || return $?; fi
            stop_nats || return $?
            stop_clusters || return $?
            RUN_ACTIVE=0; release_lock || return $?
            print '仅运维停止；数据库、对象、独立journal、CA与备份保留，未执行反部署清理。'
            ;;
        *) print -u2 '未知动作，使用--help。'; return 2 ;;
    esac
}
main() {
    show_script_intro
    case "${1:-}" in help|--help) print '用法：本平台部署脚本 [menu|check|plan|status|run|backup|stop|undeploy] [--mode local|server] [--access local|quick] [--state 私有绝对路径] [--yes] [--no-backup]。本地默认仅本机；公开及清理均须独立回车，--yes不跳过。'; return 0 ;; esac
    linux_platform_check || return $?
    initialize
    parse_arguments "$@"
    if [[ "$ACTION" == menu && "$YES" == 0 ]]; then confirm_enter 'JobsIM将准备本项目原生运行环境、双PG与NATS；共享工具缓存保留'; fi
    install_cleanup_hooks
    dispatch_action
}
main "$@"

