#!/bin/zsh
# shell: zsh
# Created by Jobs on 2026年11月14日，星期六.
DEFAULT_ACTION="stop"
# 初始化当前解释器、项目路径和实例参数。
initialize() {
    setopt NO_NOMATCH PIPE_FAIL
    set -eu
    umask 077
    SCRIPT_DIR="${${(%):-%x}:A:h}"
    REPO="${SCRIPT_DIR}/../../.."; REPO="${REPO:A}"
    REPO_ID="$(printf '%s' "$REPO" | shasum -a 256 | cut -c1-16)"
    STATE="${JOBS_IM_DEPLOY_STATE:-$HOME/Library/Application Support/JobsIM/Instances/$REPO_ID}"
    MAIN_PORT="${JOBS_IM_PG_PORT:-16543}"; JOURNAL_PORT="${JOBS_IM_JOURNAL_PORT:-16544}"
    HTTPS_PORT="${JOBS_IM_HTTPS_PORT:-9443}"; ROLE_PORT="${JOBS_IM_INTERNAL_PORT:-18101}"
    NATS_PORT="${JOBS_IM_NATS_PORT:-4222}"
    NATS_MAX_FILE="${JOBS_IM_NATS_MAX_FILE_BYTES:-536870912}"; NATS_MAX_MEMORY="${JOBS_IM_NATS_MAX_MEMORY_BYTES:-33554432}"
    PUBLIC_HOST="${JOBS_IM_PUBLIC_HOST:-127.0.0.1}"
    ACTION="$DEFAULT_ACTION"; YES=0; NO_BACKUP=0; RUN_ACTIVE=0; LOCK_HELD=0; CLEANING=0
    discover_roles
}
# 从当前Go入口读取端口顺序；治理沿用既有identity数据库，journal保持独立集群。
discover_roles() {
    local raw role
    raw="$(sed -n 's/^[[:space:]]*roles := \[\]string{\(.*\)}/\1/p' "$REPO/代码/后端/Go/cmd/jobsimd/main.go" | tr '", ' '   ')"
    SERVICES=(${=raw}); ROLES=()
    (( ${#SERVICES[@]} > 0 )) || { print -u2 '无法从Go入口取得角色顺序，拒绝猜测端口。'; return 2; }
    for role in "${SERVICES[@]}"; do
        [[ "$role" =~ '^[a-z][a-z_]*$' ]] || return 2
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
            main_port|journal_port|https_port|role_port|nats_port) [[ "$value" =~ '^[1-9][0-9]{0,4}$' ]] || return 2
                case "$key" in
                    main_port) [[ -n "${JOBS_IM_PG_PORT:-}" ]] || MAIN_PORT="$value" ;;
                    journal_port) [[ -n "${JOBS_IM_JOURNAL_PORT:-}" ]] || JOURNAL_PORT="$value" ;;
                    https_port) [[ -n "${JOBS_IM_HTTPS_PORT:-}" ]] || HTTPS_PORT="$value" ;;
                    role_port) [[ -n "${JOBS_IM_INTERNAL_PORT:-}" ]] || ROLE_PORT="$value" ;;
                    nats_port) [[ -n "${JOBS_IM_NATS_PORT:-}" ]] || NATS_PORT="$value" ;;
                esac ;;
            public_host) [[ "$value" =~ '^[A-Za-z0-9][A-Za-z0-9.-]*$' ]] || return 2; [[ -n "${JOBS_IM_PUBLIC_HOST:-}" ]] || PUBLIC_HOST="$value" ;;
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
            --state) (( $# >= 2 )) || return 2; STATE="$2"; shift 2 ;;
            --help) ACTION="help"; shift ;;
            *) print -u2 "未知参数：$1"; return 2 ;;
        esac
    done
    [[ "$STATE" == /* && "$STATE" != / && "$STATE" != "$HOME" && ! -L "$STATE" ]] || { print -u2 'state须为独立、非符号链接的绝对目录。'; return 2; }
    load_restore_layout
    if [[ "$ACTION" == run && -f "$STATE/Application/restore.pending" ]]; then print -u2 '隔离恢复尚未完成；按原operation继续，当前实例不启动。'; return 2; fi
    BACKUPS="${JOBS_IM_BACKUP_DIR:-$HOME/Documents/JobsIMBackups/${STATE:t}}"
    RECOVERY="${JOBS_IM_RECOVERY_DIR:-${STATE:h}/RecoveryKeys/${STATE:t}}"
    STATE="${STATE:A}"; BACKUPS="${BACKUPS:A}"; RECOVERY="${RECOVERY:A}"
    local private_path
    for private_path in "$STATE" "$BACKUPS" "$RECOVERY"; do
        [[ "$private_path" == /* && "$private_path" != "$REPO" && "$private_path" != "$REPO"/* && ! -L "$private_path" ]] || { print -u2 '实例、归档和恢复钥须为源码外的独立绝对目录。'; return 2; }
    done
    [[ "$STATE" != *"'"* && "$STATE" != *$'\n'* && "$STATE" != *$'\r'* ]] || { print -u2 'state不接受单引号或换行，避免PG启动参数歧义。'; return 2; }
    [[ "$BACKUPS" != "$STATE" && "$BACKUPS" != "$STATE"/* && "$RECOVERY" != "$STATE" && "$RECOVERY" != "$STATE"/* && "$RECOVERY" != "$BACKUPS" && "$RECOVERY" != "$BACKUPS"/* && "$BACKUPS" != "$RECOVERY"/* ]] || { print -u2 '数据、加密备份与解密私钥必须在彼此独立的目录。'; return 2; }
    for value in "$MAIN_PORT" "$JOURNAL_PORT" "$HTTPS_PORT" "$ROLE_PORT" "$NATS_PORT"; do
        [[ "$value" =~ '^[1-9][0-9]{0,4}$' ]] && (( value > 1024 && value < 65536 )) || { print -u2 '端口须为1025～65535的整数。'; return 2; }
    done
    ROLE_LAST=$((ROLE_PORT+${#SERVICES[@]}-1))
    (( ROLE_LAST < 65536 )) || { print -u2 '动态角色端口越界。'; return 2; }
    [[ "$NATS_MAX_FILE" =~ '^[1-9][0-9]{0,12}$' && "$NATS_MAX_MEMORY" =~ '^[1-9][0-9]{0,12}$' ]] && (( NATS_MAX_FILE >= 536870912 && NATS_MAX_MEMORY >= 33554432 )) || { print -u2 'NATS资源下限为磁盘512MiB／内存32MiB，可显式调大。'; return 2; }
    NATS_CONFIG="$STATE/Secrets/nats.conf"; NATS_STORE="$STATE/Application/JetStream"
    JOBS_IM_NATS_TOKEN_FILE="${JOBS_IM_NATS_TOKEN_FILE:-$STATE/Secrets/nats.token}"
    [[ "$JOBS_IM_NATS_TOKEN_FILE" == "$STATE/Secrets/"* && "$JOBS_IM_NATS_TOKEN_FILE" != *'/../'* && ! -L "$JOBS_IM_NATS_TOKEN_FILE" ]] || { print -u2 'NATS token文件须在本实例Secrets内且不是链接。'; return 2; }
    JOBS_IM_NATS_URL="${JOBS_IM_NATS_URL:-nats://127.0.0.1:$NATS_PORT}"
    [[ "$JOBS_IM_NATS_URL" == "nats://127.0.0.1:$NATS_PORT" ]] || { print -u2 '本入口只管理匹配端口的loopback NATS；不接管外部broker。'; return 2; }
    export JOBS_IM_NATS_URL JOBS_IM_NATS_TOKEN_FILE
    [[ "$PUBLIC_HOST" =~ '^[A-Za-z0-9][A-Za-z0-9.-]*$' ]] || { print -u2 '公开主机须为DNS名或IPv4，不含协议、端口或换行。'; return 2; }
}
# 只读检查本平台所需的既有工具。
find_tools() {
    [[ "$(uname -s)" == Darwin ]] || { print -u2 '此入口仅支持MacOS。'; return 2; }
    GO_TOOL="${JOBS_IM_GO:-}"
    if [[ -z "$GO_TOOL" ]]; then
        for candidate in "$HOME"/Library/Caches/JobsIM/Toolchains/go1.27.*/go/bin/go(N) "$HOME/Library/Application Support/JobsIM"/Toolchains/go1.27.*/go/bin/go(N); do GO_TOOL="$candidate"; done
        [[ -n "$GO_TOOL" ]] || GO_TOOL="$(command -v go || true)"
    fi
    PG_BIN="${JOBS_IM_PG_BIN:-}"
    if [[ -z "$PG_BIN" ]]; then
        for candidate in "$HOME"/Library/Caches/JobsIM/Toolchains/PostgresApp-*/Postgres.app/Contents/Versions/18/bin(N) "$HOME/Library/Application Support/JobsIM"/Toolchains/PostgresApp-*/Postgres.app/Contents/Versions/18/bin(N); do PG_BIN="$candidate"; done
        if [[ -z "$PG_BIN" ]]; then local pg="$(command -v pg_ctl || true)"; [[ -z "$pg" ]] || PG_BIN="${pg:h}"; fi
    fi
    [[ -x "$GO_TOOL" && -x "$PG_BIN/initdb" && -x "$PG_BIN/pg_ctl" && -x "$PG_BIN/psql" && -x "$PG_BIN/pg_dump" ]] || { print -u2 '缺受管Go或PG18工具；通过JOBS_IM_GO/JOBS_IM_PG_BIN指定，当前入口不全局安装。'; return 2; }
    OPENSSL="${JOBS_IM_OPENSSL:-$(command -v openssl || true)}"
    [[ -x "$OPENSSL" ]] && command -v curl >/dev/null && command -v npm >/dev/null || { print -u2 '缺OpenSSL、curl或Node/npm。'; return 2; }
    "$GO_TOOL" version
    "$PG_BIN/pg_ctl" --version
    "$OPENSSL" version
    node --version
    find_nats
}
# 只发现既有工具；缺失时失败，不运行包管理器或网络安装。
find_nats() {
    NATS_BIN="${JOBS_IM_NATS_BIN:-$(command -v nats-server || true)}"
    [[ "$NATS_BIN" != *'://'* && "$NATS_BIN" != *$'\n'* && "$NATS_BIN" != *$'\r'* ]] || { print -u2 'NATS工具须为本地可执行路径；无效输入不回显。'; return 2; }
    if [[ -z "$NATS_BIN" ]]; then
        local -a candidates; candidates=("$HOME/Library/Application Support/JobsIM/Toolchains"/NATS-v*/nats-server-*/nats-server(N))
        (( ${#candidates} != 1 )) || NATS_BIN="${candidates[1]}"
    fi
    [[ -n "$NATS_BIN" && -x "$NATS_BIN" ]] || { print -u2 "缺nats-server：检查位置=${NATS_BIN:-PATH及用户受管NATS目录}；用JOBS_IM_NATS_BIN指定已有可执行文件，不自动brew/远程安装。"; return 2; }
    NATS_BIN="${NATS_BIN:A}"; NC="$(command -v nc || true)"
    [[ -x "$NC" ]] || { print -u2 '缺nc，无法做有界NATS认证就绪检查。'; return 2; }
    "$NATS_BIN" --version
}
# 拒绝其他项目或无收据的非空实例。
assert_owner() {
    if [[ -e "$STATE/owner" ]]; then
        [[ "$(sed -n '1p' "$STATE/owner")" == "$REPO" && "$(sed -n '2p' "$STATE/owner")" == "$(id -u)" ]] || { print -u2 '实例收据属于其他项目或用户，拒绝操作。'; return 2; }
    elif [[ -d "$STATE" && -n "$(ls -A "$STATE")" ]]; then
        print -u2 '既有非空state没有归属收据，拒绝接管。'; return 2
    fi
}
# 展示前台终端生命周期。
terminal_notice() {
    if [[ -t 1 && -z "${NO_COLOR:-}" && "${TERM:-dumb}" != dumb ]]; then printf '\033[1;31m%s\033[0m\n' '关闭此运行终端，服务将结束'; else print '关闭此运行终端，服务将结束'; fi
}
# 展示变更范围并确认，显式自动化授权除外。
confirm_action() {
    print "JobsIM MacOS · $ACTION"
    print "实例：$STATE"
    print "HTTPS=${HTTPS_PORT}；业务PG=${MAIN_PORT}；独立journalPG=${JOURNAL_PORT}；内部角色=${ROLE_PORT}～$ROLE_LAST"
    print '只操作此收据匹配实例；不安装全局工具、不清理数据、不修改系统CA。先应用后NATS停止；备份后保持停止。'
    print "自托管NATS=127.0.0.1:$NATS_PORT；磁盘上限=$NATS_MAX_FILE；内存上限=$NATS_MAX_MEMORY。"
    [[ "$ACTION" != run ]] || terminal_notice
    if (( YES == 0 )); then
        [[ -t 0 ]] || { print -u2 '非交互执行须显式传--yes；check/plan不需确认。'; return 2; }
        local answer
        read -r 'answer?按回车确认，输入任意文字取消：' || return 130
        [[ -z "$answer" ]] || return 130
    fi
}
# 确认后创建仅本实例的私有状态结构。
prepare_state() {
    assert_owner
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
    [[ "$pid" == <-> ]] || return 1
    start="$(LC_ALL=C ps -p "$pid" -o lstart= 2>/dev/null || true)"
    [[ -n "$start" && "$start" == "$(cat "$STATE/Processes/server.start")" ]] || return 1
    command="$(ps -ww -p "$pid" -o command=)"
    [[ "$command" == *"$STATE/Bin/JobsIMd"* && "$command" == *"$STATE/Application"* ]]
}
# 优雅结束本实例后端，身份不明或超时保留。
stop_backend() {
    if process_matches; then
        local pid="$(cat "$STATE/Processes/server.pid")"
        kill -TERM "$pid"
        local count=0
        while kill -0 "$pid" 2>/dev/null && (( count < 70 )); do sleep 0.1; (( count+=1 )); done
        if kill -0 "$pid" 2>/dev/null; then print -u2 '服务未按时退出，保留数据和进程，拒绝强杀。'; return 2; fi
    elif [[ -f "$STATE/Processes/server.pid" ]]; then
        local pid="$(cat "$STATE/Processes/server.pid")"
        if [[ "$pid" == <-> ]] && kill -0 "$pid" 2>/dev/null; then print -u2 'PID存在但启动身份不匹配，不停止陌生进程。'; return 2; fi
    fi
    if lsof -nP -iTCP:"$HTTPS_PORT" -sTCP:LISTEN >/dev/null 2>&1; then print -u2 '应用端口仍有监听，未证明应用退出；保留broker与数据库。'; return 2; fi
}
# 校验本入口NATS的PID、启动时刻、原工具、配置及摘要；不按名称或端口认领。
nats_process_matches() {
    local name
    for name in pid start binary binary.sha256 config.sha256; do [[ -f "$STATE/Processes/nats.$name" && ! -L "$STATE/Processes/nats.$name" ]] || return 1; done
    local broker_pid="$(cat "$STATE/Processes/nats.pid")" binary="$(cat "$STATE/Processes/nats.binary")" started command
    [[ "$broker_pid" =~ '^[1-9][0-9]*$' && -f "$binary" && ! -L "$binary" && -f "$NATS_CONFIG" && ! -L "$NATS_CONFIG" ]] || return 1
    started="$(LC_ALL=C ps -p "$broker_pid" -o lstart= 2>/dev/null || true)"
    [[ -n "$started" && "$started" == "$(cat "$STATE/Processes/nats.start")" ]] || return 1
    [[ "$(shasum -a 256 "$binary" | cut -d' ' -f1)" == "$(cat "$STATE/Processes/nats.binary.sha256")" && "$(shasum -a 256 "$NATS_CONFIG" | cut -d' ' -f1)" == "$(cat "$STATE/Processes/nats.config.sha256")" ]] || return 1
    command="$(ps -ww -p "$broker_pid" -o command=)"
    [[ "$command" == "$binary -c $NATS_CONFIG" ]]
}
# 应用停止后才优雅停止本入口broker；未知身份或超时保留，不强杀。
stop_nats() {
    if nats_process_matches; then
        local broker_pid="$(cat "$STATE/Processes/nats.pid")" count=0
        kill -TERM "$broker_pid"
        while kill -0 "$broker_pid" 2>/dev/null && (( count < 100 )); do sleep 0.1; (( count+=1 )); done
        if kill -0 "$broker_pid" 2>/dev/null; then print -u2 '本实例NATS未退出；保留JetStream及进程，不强杀。'; return 2; fi
    elif [[ -f "$STATE/Processes/nats.pid" ]]; then
        local broker_pid="$(cat "$STATE/Processes/nats.pid")"
        if [[ "$broker_pid" =~ '^[1-9][0-9]*$' ]] && kill -0 "$broker_pid" 2>/dev/null; then print -u2 'NATS PID存在但身份/配置不匹配，不停止。'; return 2; fi
    fi
}
# 将路径编码到NATS字符串；凭据只经私有文件，不进入argv或日志。
nats_quoted() {
    local value="$1"; value="${value//\\/\\\\}"; value="${value//\"/\\\"}"; printf '%s' "$value"
}
# 在确认后的本实例内准备token与配置；既有数据缺钥或配置漂移拒绝覆盖。
prepare_nats() {
    [[ ! -L "$NATS_STORE" && ! -L "$NATS_CONFIG" && "${JOBS_IM_NATS_TOKEN_FILE:h}" == "$STATE/Secrets" ]] || { print -u2 'NATS状态/配置/密钥路径不合格。'; return 2; }
    if [[ ! -f "$JOBS_IM_NATS_TOKEN_FILE" ]]; then
        [[ ! -e "$NATS_CONFIG" && ! -e "$NATS_STORE" ]] || { print -u2 '既有JetStream或配置缺token，不生成替代密钥。'; return 2; }
        "$OPENSSL" rand -hex 32 > "$JOBS_IM_NATS_TOKEN_FILE"
    fi
    [[ ! -L "$JOBS_IM_NATS_TOKEN_FILE" && "$(stat -f '%Lp' "$JOBS_IM_NATS_TOKEN_FILE")" == 600 && "$(stat -f '%u' "$JOBS_IM_NATS_TOKEN_FILE")" == "$(id -u)" ]] || { print -u2 'NATS token须为当前用户0600普通文件。'; return 2; }
    local token="$(cat "$JOBS_IM_NATS_TOKEN_FILE")" quoted_store quoted_pid temporary="$STATE/Secrets/.nats-config-$$"
    [[ "$token" =~ '^[0-9a-f]{64}$' ]] || { print -u2 'NATS token格式无效；不输出或替换原秘密。'; return 2; }
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
    shasum -a 256 "$NATS_BIN" | cut -d' ' -f1 > "$STATE/Processes/nats.binary.sha256"
    shasum -a 256 "$NATS_CONFIG" | cut -d' ' -f1 > "$STATE/Processes/nats.config.sha256"
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
        [[ "$original_pid" =~ '^[1-9][0-9]*$' ]] && ! kill -0 "$original_pid" 2>/dev/null || { print -u2 'NATS原PID未证实退出，拒绝冷态归档。'; return 2; }
        [[ "$(shasum -a 256 "$NATS_CONFIG" | cut -d' ' -f1)" == "$(cat "$STATE/Processes/nats.config.sha256")" ]] || { print -u2 'NATS配置偏离原收据，保留并拒绝标完整。'; return 2; }
        printf 'component=NATS JetStream\nstate=stopped-before-archive\nstore=Application/JetStream\nconfig=Secrets/nats.conf\ntoken=Secrets/%s\nrestore=offline verification required; not tested by archive listing\n' "${JOBS_IM_NATS_TOKEN_FILE:t}" > "$destination"
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
            if "$PG_BIN/pg_ctl" -D "$data" status >/dev/null 2>&1; then "$PG_BIN/pg_ctl" -D "$data" -m fast -w -t 30 stop; fi
        fi
    done
}
# 检查所有动态角色及依赖端口，不接管占用者。
check_ports() {
    local -a ports; ports=("$HTTPS_PORT" "$MAIN_PORT" "$JOURNAL_PORT" "$NATS_PORT")
    local i port
    for (( i=0; i<${#SERVICES[@]}; i++ )); do ports+=("$((ROLE_PORT+i))"); done
    local -a unique_ports; unique_ports=("${(@u)ports}")
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
        "$OPENSSL" rand -hex 32 > "$STATE/Secrets/operator.password"
    fi
    for name in Main Journal; do
        data="$STATE/${name}Database"; port="$MAIN_PORT"; [[ "$name" == Main ]] || port="$JOURNAL_PORT"
        if [[ ! -f "$data/PG_VERSION" ]]; then
            "$PG_BIN/initdb" -D "$data" -U jobsim_operator --pwfile="$STATE/Secrets/operator.password" --auth-local=trust --auth-host=scram-sha-256 --encoding=UTF8 --locale=C > "$STATE/Logs/init-$name.log"
            cluster_identity "$data" > "$STATE/Processes/$name.cluster"
        fi
        check_cluster "$name" "$data"
        if ! "$PG_BIN/pg_ctl" -D "$data" status >/dev/null 2>&1; then
            # 客户端均走回环TCP；禁用Unix socket以避开MacOS的路径字节上限。
            "$PG_BIN/pg_ctl" -D "$data" -l "$STATE/Logs/pg-$name.log" -w -t 30 -o "-p $port -h 127.0.0.1 -k '' -c max_connections=80 -c shared_buffers=128MB -c fsync=on -c full_page_writes=on -c synchronous_commit=on" start
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
        export "JOBS_IM_${role:u}_DATABASE_URL=postgresql://jobsim_$role:$password@127.0.0.1:$port/jobsim_$role?sslmode=disable"
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
    local tls_stage="$(mktemp -d "$STATE/.TLS-pending.XXXXXX")"
    [[ -n "$tls_stage" && -d "$tls_stage" ]] || return 2
    "$OPENSSL" req -x509 -newkey rsa:3072 -nodes -days 365 -keyout "$tls_stage/ca.key" -out "$tls_stage/ca.crt" -subj '/CN=JobsIM Local Instance CA' -addext 'basicConstraints=critical,CA:TRUE' -addext 'keyUsage=critical,keyCertSign,cRLSign' -addext 'subjectKeyIdentifier=hash' > "$STATE/Logs/tls.log" 2>&1 || return $?
    "$OPENSSL" req -new -newkey rsa:2048 -nodes -keyout "$tls_stage/server.key" -out "$tls_stage/server.csr" -subj "/CN=$PUBLIC_HOST" >> "$STATE/Logs/tls.log" 2>&1 || return $?
    printf 'subjectAltName=DNS:localhost,IP:127.0.0.1,IP:10.0.2.2\n' > "$tls_stage/server.ext"
    if [[ "$PUBLIC_HOST" != 127.0.0.1 && "$PUBLIC_HOST" != localhost ]]; then
        if [[ "$PUBLIC_HOST" =~ '^[0-9.]+$' ]]; then print "subjectAltName=DNS:localhost,IP:127.0.0.1,IP:10.0.2.2,IP:$PUBLIC_HOST" > "$tls_stage/server.ext"; else print "subjectAltName=DNS:localhost,IP:127.0.0.1,IP:10.0.2.2,DNS:$PUBLIC_HOST" > "$tls_stage/server.ext"; fi
    fi
    printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth\nsubjectKeyIdentifier=hash\nauthorityKeyIdentifier=keyid,issuer\n' >> "$tls_stage/server.ext"
    "$OPENSSL" x509 -req -in "$tls_stage/server.csr" -CA "$tls_stage/ca.crt" -CAkey "$tls_stage/ca.key" -CAcreateserial -days 90 -out "$tls_stage/server.crt" -extfile "$tls_stage/server.ext" >> "$STATE/Logs/tls.log" 2>&1 || return $?
    "$OPENSSL" verify -x509_strict -purpose sslserver -CAfile "$tls_stage/ca.crt" "$tls_stage/server.crt" >> "$STATE/Logs/tls.log" 2>&1 || return $?
    # 生成或验真失败只留下私有pending；实例锁内提交前再次确认目标为空。
    rmdir -- "$STATE/TLS" || return $?
    mv -n -- "$tls_stage" "$STATE/TLS" || return $?
    [[ -f "$STATE/TLS/server.crt" && ! -e "$tls_stage" ]] || { print -u2 'TLS提交发生并发变化，保留材料并停止。'; return 2; }
}
# 构建当前源码，失败不替换为假成功。
build_source() {
    print '构建Go角色与WebDist；依赖缓存留在用户级，未安装全局工具。'
    (cd "$REPO/代码/后端/Go" && GOTOOLCHAIN=local "$GO_TOOL" build -trimpath -o "$STATE/Bin/JobsIMd.new" ./cmd/jobsimd)
    mv -- "$STATE/Bin/JobsIMd.new" "$STATE/Bin/JobsIMd"
    (cd "$REPO/代码/前端/Web" && npm run build)
    [[ -f "$REPO/代码/前端/Web/Dist/index.html" ]] || { print -u2 'WebDist缺失。'; return 2; }
}
# 应用与broker停止后生成范围明确的加密归档。
backup_instance() {
    stop_backend
    stop_nats
    prepare_clusters
    mkdir -p "$BACKUPS" "$RECOVERY"
    chmod 700 "$BACKUPS" "$RECOVERY"
    if [[ ! -f "$RECOVERY/key.pem" ]]; then
        [[ ! -e "$RECOVERY/cert.pem" ]] || { print -u2 '恢复密钥材料不完整。'; return 2; }
        "$OPENSSL" req -x509 -newkey rsa:3072 -nodes -days 3650 -keyout "$RECOVERY/key.pem" -out "$RECOVERY/cert.pem" -subj '/CN=JobsIM Backup Recovery' > "$STATE/Logs/backup-key.log" 2>&1
    fi
    local staging="$BACKUPS/.staging-$(date +%Y%m%d-%H%M%S)-$$" file="$BACKUPS/JobsIM-$(date +%Y%m%d-%H%M%S)-$$.cms"
    mkdir "$staging"
    write_nats_backup_manifest "$staging/broker-manifest.txt"
    local role port
    for role in "${ROLES[@]}" journal; do
        port="$MAIN_PORT"; [[ "$role" != journal ]] || port="$JOURNAL_PORT"
        PGPASSWORD="$(cat "$STATE/Secrets/operator.password")" "$PG_BIN/pg_dump" -h 127.0.0.1 -p "$port" -U jobsim_operator -d "jobsim_$role" --format=custom --file="$staging/$role.dump"
        "$PG_BIN/pg_restore" --list "$staging/$role.dump" > "$staging/$role.list"
    done
    tar -czf - -C "$STATE" owner Application TLS Secrets -C "$staging" . | "$OPENSSL" cms -encrypt -aes-256-gcm -binary -outform DER -out "$file" "$RECOVERY/cert.pem"
    "$OPENSSL" cms -decrypt -binary -inform DER -in "$file" -inkey "$RECOVERY/key.pem" | tar -tzf - > "$file.manifest"
    shasum -a 256 "$file" > "$file.sha256"
    for role in "${ROLES[@]}" journal; do rm -- "$staging/$role.dump" "$staging/$role.list"; done
    rm -- "$staging/broker-manifest.txt"; rmdir "$staging"
    print "已生成并解密验真：$file"
    print "恢复私钥独立保存于：$RECOVERY/key.pem；请另行离线保管。备份不包含这把解密私钥。"
    print '归档范围明确列出业务PG、独立journal及已停JetStream/配置/token；不自动回滚journal。归档验真不等于broker或数据库恢复演练。'
}
# 仅展示无秘密连接身份及CA指纹。
write_connection() {
    local url="https://$PUBLIC_HOST:$HTTPS_PORT" response deployment fingerprint
    response="$(curl --fail --silent --show-error --cacert "$STATE/TLS/ca.crt" --max-time 5 "$url/api/bootstrap")"
    deployment="$(print -r -- "$response" | sed -n 's/.*"deploymentId":"\([^"]*\)".*/\1/p')"
    [[ -n "$deployment" ]] || { print -u2 '公开实例身份缺失。'; return 2; }
    fingerprint="$("$OPENSSL" x509 -in "$STATE/TLS/ca.crt" -noout -fingerprint -sha256 | cut -d= -f2)"
    printf '{"deploymentId":"%s","name":"JobsIM","url":"%s","protocolVersion":"1","caSha256":"%s"}\n' "$deployment" "$url" "$fingerprint" > "$STATE/connection.json"
    print '用户端：'; show_endpoint "$url"
    print "管理入口：${url}（管理写入口尚未交付，不生成假后台URL）"
    print "deploymentId：$deployment"
    print "CA SHA256：$fingerprint"
    print "公开连接描述：$STATE/connection.json"
    print 'Web须按系统/浏览器受控信任本CA或另用公信HTTPS；未信任时阻断，不能绕过证书警告。脚本不自动安装全局CA。'
    if command -v qrencode >/dev/null; then qrencode -t UTF8 "$url" || print '终端二维码失败，使用上述URL。'; qrencode -o "$STATE/connection.png" "$url" || print 'PNG二维码失败，使用上述URL。'; else print 'qrencode未安装，本轮未生成二维码；保留可复制URL，不记二维码验收通过。'; fi
}
# 先核对旧实例与备份，再启动本轮依赖和应用。
run_instance() {
    confirm_action
    prepare_state; acquire_lock
    if [[ -f "$STATE/Application/instance.json" ]]; then backup_instance; stop_clusters; fi
    stop_backend; stop_nats; stop_clusters; check_ports
    prepare_clusters; prepare_databases; prepare_tls; build_source
    start_nats
    JOBS_IM_JOURNAL_DATABASE_URL="$JOBS_IM_JOURNAL_DATABASE_URL" JOBS_IM_PUBLIC_URL="https://$PUBLIC_HOST:$HTTPS_PORT" "$STATE/Bin/JobsIMd" --service all --addr "127.0.0.1:$HTTPS_PORT" --internal-base-port "$ROLE_PORT" --state-dir "$STATE/Application" --tls-cert "$STATE/TLS/server.crt" --tls-key "$STATE/TLS/server.key" --web-dir "$REPO/代码/前端/Web/Dist" > "$STATE/Logs/server.log" 2>&1 &
    local pid=$!
    print "$pid" > "$STATE/Processes/server.pid"
    LC_ALL=C ps -p "$pid" -o lstart= > "$STATE/Processes/server.start"
    local ready=0 count=0
    while (( count < 60 )); do
        if curl --fail --silent --cacert "$STATE/TLS/ca.crt" --max-time 1 "https://127.0.0.1:$HTTPS_PORT/api/health" >/dev/null 2>&1; then ready=1; break; fi
        kill -0 "$pid" 2>/dev/null || break
        sleep 0.5; (( count+=1 ))
    done
    (( ready == 1 )) || { print -u2 "启动失败；见$STATE/Logs/server.log，未报告成功。"; return 2; }
    printf 'project=%s\nuid=%s\npg_main=%s\npg_journal=%s\nhttps=%s\nrole_base=%s\n' "$REPO" "$(id -u)" "$MAIN_PORT" "$JOURNAL_PORT" "$HTTPS_PORT" "$ROLE_PORT" > "$STATE/receipt.txt"
    write_connection
    release_lock
    terminal_notice
    wait "$pid"
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
    local result=$?
    (( ${CLEANING:-0} == 0 )) || return 0
    CLEANING=1
    set +e
    if (( RUN_ACTIVE == 1 )); then
        if [[ -d "$STATE/Lock" && "$(cat "$STATE/Lock/pid" 2>/dev/null)" != "$$" ]]; then
            print '服务退出；已将PG收尾交给持锁的本实例控制事务。'
        else
            if stop_backend && stop_nats; then stop_clusters; else print -u2 '应用或broker未证实停止，保留其下游资源。'; fi
        fi
    fi
    RUN_ACTIVE=0
    release_lock
    return "$result"
}
# 保留业务退出码并执行精确收尾。
TRAPEXIT() { cleanup; }
# 错误路径复用收尾，避免递归清理。
TRAPZERR() {
    local result=$?
    if (( ${CLEANING:-0} == 1 )); then return 0; fi
    cleanup
    exit "$result"
}
# 中断只处理本轮资源。
TRAPINT() { cleanup; exit 130; }
# 终止信号只处理本轮资源。
TRAPTERM() { cleanup; exit 143; }
# 终端断开只处理本轮资源。
TRAPHUP() { cleanup; exit 143; }
# 展示入口能力；只读动作不需要交互，也不做任何初始化写入。
show_script_intro() {
    print 'JobsIM：check/plan/status只读；run启动本实例双PG、loopback JetStream与Go角色；stop默认备份并保留数据。'
}
# 按已经解析的模式编排业务，未知归属始终保留。
dispatch_action() {
    case "$ACTION" in
        check|plan|status) read_only ;;
        run) find_tools; assert_owner; run_instance ;;
        stop|backup)
            find_tools; assert_owner; [[ -f "$STATE/owner" ]] || { print '本实例尚未创建；没有停止或清理任何资源。'; return 0; }
            confirm_action; prepare_state; acquire_lock
            RUN_ACTIVE=1
            if [[ "$ACTION" == backup || "$NO_BACKUP" == 0 ]]; then backup_instance; else stop_backend; fi
            stop_nats; stop_clusters; RUN_ACTIVE=0; release_lock
            print '本实例已停止；数据库、对象、CA/私钥、独立journal与既有备份均保留。'
            ;;
        help) print '用法：部署运行.command check|plan|status|run|backup|stop [--state 绝对目录] [--yes] [--no-backup]；stop默认加密备份后停止，绝不删数据。' ;;
        *) print -u2 '未知动作；使用--help。'; return 2 ;;
    esac
}
# 唯一入口按展示、配置、参数和业务依次收口。
main() {
    show_script_intro "$@" # 展示本入口能力与只读边界。
    initialize # 解析脚本位置、持久目录和当前源码角色。
    parse_arguments "$@" # 校验显式参数、端口和实例边界。
    dispatch_action # 由选定动作执行只读检查或确认后的生命周期。
}
main "$@"
