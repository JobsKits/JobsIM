#!/bin/zsh
# shell: zsh
# Created by Jobs on 2026年11月14日，星期六.
DEFAULT_ACTION="run"
# 初始化当前解释器、项目路径和实例参数。
initialize() {
    setopt NO_NOMATCH PIPE_FAIL
    set -eu
    umask 077
    SCRIPT_DIR="${${(%):-%x}:A:h}"
    REPO="${SCRIPT_DIR}/../../../.."; REPO="${REPO:A}"
    REPO_ID="$(printf '%s' "$REPO" | shasum -a 256 | cut -c1-16)"
    read_instance_reference
    STATE="${JOBS_IM_DEPLOY_STATE:-$HOME/Library/Application Support/JobsIM/Instances/$REPO_ID}"
    DEFAULT_INSTANCE_STATE="$STATE"
    if [[ -n "${JOBS_IM_DEPLOY_STATE:-}" ]]; then DEFAULT_INSTANCE_STATE=""; fi
    MAIN_PORT="${JOBS_IM_PG_PORT:-16543}"; JOURNAL_PORT="${JOBS_IM_JOURNAL_PORT:-16544}"
    HTTPS_PORT="${JOBS_IM_HTTPS_PORT:-9443}"; ROLE_PORT="${JOBS_IM_INTERNAL_PORT:-18101}"
    NATS_PORT="${JOBS_IM_NATS_PORT:-4222}"
    NATS_MAX_FILE="${JOBS_IM_NATS_MAX_FILE_BYTES:-536870912}"; NATS_MAX_MEMORY="${JOBS_IM_NATS_MAX_MEMORY_BYTES:-33554432}"
    PUBLIC_HOST="${JOBS_IM_PUBLIC_HOST:-127.0.0.1}"
    ACTION="$DEFAULT_ACTION"; YES=0; NO_BACKUP=0; RUN_ACTIVE=0; LOCK_HELD=0; CLEANING=0
    RUN_DIR=""; LOG_FILE="/dev/null"; FZF_EXECUTABLE=""; TUNNEL_PID=""; BROWSER_MENU_PID=""; QR_PAGE=""; QR_BINARY=""; DATA_CHOICE=""; DEPLOY_MODE=""; PUBLIC_URL=""; LOCAL_ACCESS=private; CLOUDFLARED_EXECUTABLE=""
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
            --check) ACTION=check; shift ;;
            --plan) ACTION=plan; shift ;;
            --mode) (( $# >= 2 )) || return 2; DEPLOY_MODE="$2"; shift 2 ;;
            --help) ACTION="help"; shift ;;
            *) print -u2 "未知参数：$1"; return 2 ;;
        esac
    done
    [[ "$STATE" == /* && "$STATE" != / && "$STATE" != "$HOME" && ! -L "$STATE" ]] || { print -u2 'state须为独立、非符号链接的绝对目录。'; return 2; }
    [[ "$STATE" != *$'\t'* && "$STATE" != *$'\n'* && "$STATE" != *$'\r'* ]] || return 2
    if [[ "$ACTION" == run && -e "$STATE/.Deployment/undeploy" ]]; then print -u2 '此实例已有反部署断点或受保护journal遗留；不得重新初始化业务库，请续原反部署或选择全新state。'; return 2; fi
    [[ -z "$DEPLOY_MODE" || "$ACTION" == check || "$ACTION" == plan || "$ACTION" == status || "$ACTION" == demo-prepare || "$ACTION" == demo-run ]] || { print -u2 '--mode仅供只读计划，正常部署须经fzf选择。'; return 2; }
    load_restore_layout
    if [[ "$ACTION" == run && -f "$STATE/Application/restore.pending" ]]; then print -u2 '隔离恢复尚未完成；按原operation继续，当前实例不启动。'; return 2; fi
    BACKUPS="${JOBS_IM_BACKUP_DIR:-$HOME/Documents/JobsIMBackups/${STATE:t}}"
    RECOVERY="${JOBS_IM_RECOVERY_DIR:-${STATE:h}/RecoveryKeys/${STATE:t}}"
    assert_plain_ancestry "$STATE"; assert_plain_ancestry "$BACKUPS"; assert_plain_ancestry "$RECOVERY"
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
    [[ -x "$OPENSSL" ]] && command -v curl >/dev/null || { print -u2 '缺OpenSSL、curl或Node/npm。'; return 2; }
    "$GO_TOOL" version
    "$PG_BIN/pg_ctl" --version
    "$OPENSSL" version
    if (( ${PREPARING_TOOLS:-0} == 0 )); then node --version; fi
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
    if [[ -z "$NATS_BIN" && "${PREPARING_TOOLS:-0}" == 1 ]]; then NC="$(command -v nc || true)"; return 0; fi
    [[ -n "$NATS_BIN" && -x "$NATS_BIN" ]] || { print -u2 "缺nats-server：检查位置=${NATS_BIN:-PATH及用户受管NATS目录}；用JOBS_IM_NATS_BIN指定已有可执行文件，不自动brew/远程安装。"; return 2; }
    NATS_BIN="${NATS_BIN:A}"; NC="$(command -v nc || true)"
    [[ -x "$NC" ]] || { print -u2 '缺nc，无法做有界NATS认证就绪检查。'; return 2; }
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
    REF_DIR="$REPO/.jobsim-local"; REF_FILE="$REF_DIR/instance.ref"
    [[ -e "$REF_DIR" || -L "$REF_DIR" ]] || return 0
    [[ -d "$REF_DIR" && ! -L "$REF_DIR" && -O "$REF_DIR" ]] || { print -u2 '本机实例引用目录归属不明，拒绝使用。'; return 2; }
    [[ "$(stat -f %Lp "$REF_DIR")" == 700 ]] || { print -u2 '本机实例引用目录须为私有目录。'; return 2; }
    [[ -e "$REF_FILE" || -L "$REF_FILE" ]] || return 0
    [[ -f "$REF_FILE" && ! -L "$REF_FILE" && -O "$REF_FILE" ]] || { print -u2 '本机实例引用必须是本人普通文件。'; return 2; }
    [[ "$(stat -f %Lp "$REF_DIR")" == 700 && "$(stat -f %Lp "$REF_FILE")" == 600 ]] || { print -u2 '本机实例引用须为私有目录及文件。'; return 2; }
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
    previous="$(mktemp "$STATE/owner-relocation.XXXXXX")"
    cp -p -- "$STATE/owner" "$previous"
    next="$(mktemp "$STATE/owner-new.XXXXXX")"
    printf '%s\n%s\n' "$REPO" "$(id -u)" > "$next"
    mv -- "$next" "$STATE/owner"
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
# 路径任一级是链接时保留，不在规范化时悄悄改变清理目标。
assert_plain_ancestry() {
    local candidate="$1"
    while [[ "$candidate" != / && -n "$candidate" ]]; do
        [[ ! -L "$candidate" ]] || { print -u2 '私有路径含软链接，保留并拒绝处理。'; return 2; }
        candidate="${candidate:h}"
    done
}
# 以设备和inode绑定本机对象，不把相同字符串路径当作归属证明。
resource_identity() {
    /usr/bin/stat -f '%d:%i' "$1"
}
# 原点只在首次确认部署时建立，旧内容记录为复用，不能追认为可删除。
origin_begin() {
    [[ ! -L "$STATE" && ( ! -e "$STATE" || -d "$STATE" ) ]] || return 2
    local existed=0 entry origin
    [[ ! -e "$STATE" ]] || existed=1
    if [[ -e "$STATE/.Deployment" ]]; then origin_load; return $?; fi
    mkdir -p -- "$STATE"
    [[ -O "$STATE" ]] || return 2
    mkdir -- "$STATE/.Deployment"
    chmod 700 "$STATE" "$STATE/.Deployment"
    printf 'JobsIM-resources-v1\n%s\n%s\n%s\n%s\n' "$(id -u)" "$STATE" "$(resource_identity "$STATE")" "$existed" > "$STATE/.Deployment/origin"
    : > "$STATE/.Deployment/resources.tsv"
    for entry in "$STATE"/*(DN); do
        [[ "${entry:t}" == .Deployment ]] && continue
        [[ ! -L "$entry" ]] || { print -u2 '原点包含链接；保留现场，不进入部署。'; return 2; }
        origin_record reused "${entry:t}"
    done
}
# 校验原点元数据及实例目录，绝不source状态文件。
origin_load() {
    local directory="$STATE/.Deployment" file
    for file in "$directory" "$directory/origin" "$directory/resources.tsv"; do
        [[ -e "$file" && ! -L "$file" && -O "$file" ]] || { print -u2 '缺可验证原点清单；保留资源，反部署未完成。'; return 2; }
    done
    [[ -d "$directory" && -f "$directory/origin" && -f "$directory/resources.tsv" ]] || return 2
    [[ "$(sed -n '1p' "$directory/origin")" == JobsIM-resources-v1 && "$(sed -n '2p' "$directory/origin")" == "$(id -u)" && "$(sed -n '3p' "$directory/origin")" == "$STATE" && "$(sed -n '4p' "$directory/origin")" == "$(resource_identity "$STATE")" ]] || { print -u2 '原点用户、state路径或inode不同；保留全部资源。'; return 2; }
    local disposition relative kind identity extra
    while IFS=$'\t' read -r disposition relative kind identity extra; do
        [[ "$disposition" == created || "$disposition" == reused ]] || return 2
        [[ -n "$relative" && "$relative" != */* && "$relative" != . && "$relative" != .. && "$relative" != .Deployment && "$relative" != Lock && "$relative" != *$'\r'* && -z "$extra" ]] || return 2
        [[ "$kind" == d || "$kind" == f ]] && [[ "$identity" =~ '^[0-9]+:[0-9]+$' ]] || return 2
    done < "$directory/resources.tsv"
    [[ "$(cut -f2 "$directory/resources.tsv" | LC_ALL=C sort | uniq -d | wc -l | tr -d ' ')" == 0 ]] || return 2
}
# 只登记入口刚创建的顶层命名空间；复用对象保留其原始身份。
origin_record() {
    local disposition="$1" relative="$2" kind=f entry="$STATE/$2" next
    [[ "$relative" != */* && "$relative" != *$'\t'* && "$relative" != *$'\n'* && -e "$entry" && ! -L "$entry" && -O "$entry" ]] || return 2
    [[ ! -d "$entry" ]] || kind=d
    [[ -d "$entry" || -f "$entry" ]] || return 2
    if cut -f2 "$STATE/.Deployment/resources.tsv" | grep -Fqx -- "$relative"; then
        [[ "$(awk -F '\t' -v name="$relative" '$2==name{print $1}' "$STATE/.Deployment/resources.tsv")" == created ]] || return 0
        next="$(mktemp "$STATE/.Deployment/.resources.XXXXXX")"
        awk -F '\t' -v name="$relative" '$2!=name' "$STATE/.Deployment/resources.tsv" > "$next"
        printf 'created\t%s\t%s\t%s\n' "$relative" "$kind" "$(resource_identity "$entry")" >> "$next"
        mv -- "$next" "$STATE/.Deployment/resources.tsv"
    else
        printf '%s\t%s\t%s\t%s\n' "$disposition" "$relative" "$kind" "$(resource_identity "$entry")" >> "$STATE/.Deployment/resources.tsv"
    fi
}
# 既有目录不扩大归属；新目录创建后立即记入本实例原点。
origin_directory() {
    local relative="$1"
    if [[ ! -e "$STATE/$relative" ]]; then
        mkdir -- "$STATE/$relative"
        origin_record created "$relative"
    fi
    [[ -d "$STATE/$relative" && ! -L "$STATE/$relative" && -O "$STATE/$relative" ]] || return 2
}
# 保留不可随业务反部署销毁的独立journal及匹配密钥。
resource_protected() {
    case "$1" in JournalDatabase|Secrets|TLS|owner) return 0;; *) return 1;; esac
}
# 未知顶层、链接、所有者变化或根对象替换均在停机和清理前拒绝。
origin_verify_tree() {
    origin_load || return $?
    local disposition relative kind identity extra entry list
    while IFS=$'\t' read -r disposition relative kind identity extra; do
        entry="$STATE/$relative"
        if [[ ! -e "$entry" && ! -L "$entry" && -f "$STATE/.Deployment/undeploy/phase" ]]; then continue; fi
        [[ -e "$entry" && ! -L "$entry" && -O "$entry" && "$(resource_identity "$entry")" == "$identity" ]] || { print -u2 "资源身份变化，保留：$relative"; return 2; }
    done < "$STATE/.Deployment/resources.tsv"
    for entry in "$STATE"/*(DN); do
        relative="${entry:t}"
        [[ "$relative" == .Deployment || "$relative" == Lock ]] && continue
        cut -f2 "$STATE/.Deployment/resources.tsv" | grep -Fqx -- "$relative" || { print -u2 "未知顶层对象，保留并停止：$relative"; return 2; }
    done
    list="$(mktemp "${TMPDIR:-/tmp/}JobsIM-tree-check.XXXXXX")"
    if ! /usr/bin/find -P "$STATE" -print0 > "$list"; then rm -- "$list"; return 2; fi
    while IFS= read -r -d '' entry; do
        [[ ! -L "$entry" && -O "$entry" && ( -f "$entry" || -d "$entry" ) && "$entry" != *$'\n'* && "$entry" != *$'\t'* && "$entry" != *$'\r'* ]] || { rm -- "$list"; print -u2 '实例树含链接、特殊文件、控制字符或未知所有者，保留并停止。'; return 2; }
    done < "$list"
    rm -- "$list"
}
# 运行成功才绑定公开部署身份，原点不随重启改成另一实例。
origin_bind_deployment() {
    local deployment="$1" previous=""
    [[ ! -f "$STATE/.Deployment/deployment" ]] || previous="$(cat "$STATE/.Deployment/deployment")"
    [[ -z "$previous" || "$previous" == "$deployment" ]] || { print -u2 '原点部署身份改变，拒绝接续。'; return 2; }
    printf '%s\n' "$deployment" > "$STATE/.Deployment/deployment"
}
# 每个交互层显式区分取消、错误及实际选择，忽略调用方fzf默认选项。
pick_operation() {
    local prompt="$1" header="$2" result code=0
    shift 2
    result="$(printf '%s\n' "$@" | FZF_DEFAULT_OPTS='' FZF_DEFAULT_OPTS_FILE=/dev/null FZF_DEFAULT_COMMAND='' "$FZF_EXECUTABLE" --no-sort --no-multi --layout=reverse --no-height --border --delimiter=$'\t' --with-nth=2 --bind='up:up,down:down,tab:down,btab:up' --prompt="$prompt > " --header="$header")" || code=$?
    (( code == 0 )) || { (( code == 1 || code == 130 )) && return 130; return "$code"; }
    [[ -n "$result" ]] || return 130
    SELECTED_OPERATION="${result%%$'\t'*}"
}
# 最终回车只确认已显示的确切范围；yes、非空输入和EOF均不替代此确认。
confirm_cleanup() {
    print "将清理原点中本实例新建的程序、业务数据库和运行资料：$STATE"
    print '保留：独立JournalDatabase、Application/instance.json、Secrets、TLS、归属审计、共享工具链、源码、外置备份和恢复钥。'
    print "数据方式：$DATA_CHOICE；未知、复用或并发变化对象均保留并报告未完成。"
    local answer
    printf '按回车执行以上清理；输入任意文字或输入结束取消：'
    IFS= read -r answer || return 130
    [[ -z "$answer" ]] || return 130
}
# 备份覆盖的应用、媒体和匹配钥以同一冷态摘要绑定，独立于PG物理页的正常启动变化。
backup_source_fingerprint() {
    /usr/bin/tar -cf - -C "$STATE" Application TLS Secrets | shasum -a 256 | cut -d' ' -f1
}
# 逐项记录冷态文件摘要和目录身份；目录采用depth顺序供精确删除。
undeploy_snapshot() {
    local work="$STATE/.Deployment/undeploy" disposition relative kind identity extra entry path_list hash size mode
    mkdir -- "$work"
    : > "$work/plan.tsv"
    while IFS=$'\t' read -r disposition relative kind identity extra; do
        [[ "$disposition" == created ]] || continue
        resource_protected "$relative" && continue
        path_list="$(mktemp "$work/.paths.XXXXXX")"
        /usr/bin/find -P "$STATE/$relative" -depth -print0 > "$path_list" || return $?
        while IFS= read -r -d '' entry; do
            case "${entry#$STATE/}" in Application|Application/instance.json) continue;; esac
            hash=-; size=-; kind=d
            [[ ! -L "$entry" && -O "$entry" ]] || return 2
            if [[ -f "$entry" ]]; then kind=f; size="$(stat -f %z "$entry")"; hash="$(resource_digest "$entry")"; fi
            mode="$(stat -f %Lp "$entry")"
            printf '%s\t%s\t%s\t%s\t%s\t%s\n' "${entry#$STATE/}" "$kind" "$(resource_identity "$entry")" "$mode" "$size" "$hash" >> "$work/plan.tsv"
        done < "$path_list"
        rm -- "$path_list"
    done < "$STATE/.Deployment/resources.tsv"
    printf '%s\n' "$DATA_CHOICE" > "$work/data-choice"
    if [[ "$DATA_CHOICE" == backup ]]; then
        printf '%s\n' "$SOURCE_BACKUP_DIGEST" > "$work/source.sha256"
        printf '%s\n' "$LAST_BACKUP_FILE" > "$work/backup.path"
        shasum -a 256 "$LAST_BACKUP_FILE" | cut -d' ' -f1 > "$work/backup.sha256"
    fi
    print prepared > "$work/phase"
}
# 重查快照所有对象及目录全集；已删除项只在持久deleting阶段允许缺失。
undeploy_verify_plan() {
    origin_verify_tree || return $?
    local work="$STATE/.Deployment/undeploy" relative kind identity mode size hash entry phase disposition extra list child
    phase="$(cat "$work/phase")"
    [[ "$phase" == prepared || "$phase" == deleting ]] || return 2
    if [[ "$(cat "$work/data-choice")" == backup ]]; then
        local archive="$(cat "$work/backup.path")"
        [[ -f "$archive" && ! -L "$archive" && "$(shasum -a 256 "$archive" | cut -d' ' -f1)" == "$(cat "$work/backup.sha256")" ]] || { print -u2 '原备份不再匹配，保留源数据。'; return 2; }
        if [[ "$phase" == prepared ]]; then [[ "$(backup_source_fingerprint)" == "$(cat "$work/source.sha256")" ]] || { print -u2 '已备份来源在确认窗口发生变化；保留源与原归档。'; return 2; }; fi
    fi
    while IFS=$'\t' read -r relative kind identity mode size hash; do
        [[ "$relative" != Application && "$relative" != Application/instance.json ]] || return 2
        [[ "$relative" != /* && "$relative" != ../* && "$relative" != *'/../'* && "$relative" != .Deployment* && "$relative" != Lock* ]] || return 2
        entry="$STATE/$relative"
        if [[ ! -e "$entry" && ! -L "$entry" && "$phase" == deleting ]]; then continue; fi
        [[ -e "$entry" && ! -L "$entry" && -O "$entry" && "$(resource_identity "$entry")" == "$identity" && "$(stat -f %Lp "$entry")" == "$mode" ]] || { print -u2 "确认快照已变化，保留：$relative"; return 2; }
        if [[ "$kind" == f ]]; then
            [[ -f "$entry" && "$(stat -f %z "$entry")" == "$size" && "$(resource_digest "$entry")" == "$hash" ]] || { print -u2 "确认后文件内容变化，保留：$relative"; return 2; }
        else [[ "$kind" == d && -d "$entry" ]] || return 2; fi
    done < "$work/plan.tsv"
    cut -f1 "$work/plan.tsv" > "$work/planned-paths"
    while IFS=$'\t' read -r disposition relative kind identity extra; do
        [[ "$disposition" == created && -e "$STATE/$relative" ]] || continue
        resource_protected "$relative" && continue
        list="$(mktemp "$work/.scan.XXXXXX")"
        /usr/bin/find -P "$STATE/$relative" -print0 > "$list" || return $?
        while IFS= read -r -d '' child; do
            case "${child#$STATE/}" in Application|Application/instance.json) continue;; esac
            grep -Fqx -- "${child#$STATE/}" "$work/planned-paths" || { rm -- "$list"; print -u2 '确认后目录新增对象，保留并停止。'; return 2; }
        done < "$list"
        rm -- "$list"
    done < "$STATE/.Deployment/resources.tsv"
}
# 删除前再次核每个原对象，只按快照unlink/rmdir，不递归删除未知树。
undeploy_delete_plan() {
    undeploy_verify_plan || return $?
    local relative kind identity mode size hash entry work="$STATE/.Deployment/undeploy"
    print deleting > "$work/phase"
    while IFS=$'\t' read -r relative kind identity mode size hash; do
        entry="$STATE/$relative"
        [[ -e "$entry" || -L "$entry" ]] || continue
        [[ ! -L "$entry" && "$(resource_identity "$entry")" == "$identity" ]] || return 2
        if [[ "$kind" == f ]]; then
            [[ "$(resource_digest "$entry")" == "$hash" ]] || return 2
            /bin/rm -- "$entry"
        else /bin/rmdir -- "$entry"; fi
    done < "$work/plan.tsv"
    remove_instance_reference
    print complete-protected-retained > "$work/phase"
    print '反部署清理已完成所列可清资源；独立journal、匹配密钥及原点/复用资料仍保留，整体结果为partial。'
}
# 只移除与本次原点部署身份相同的本机引用。
remove_instance_reference() {
    [[ "$STATE" == "$DEFAULT_INSTANCE_STATE" ]] || return 0
    [[ -e "$REF_FILE" || -L "$REF_FILE" ]] || return 0
    local deployment="$(cat "$STATE/.Deployment/deployment" 2>/dev/null || true)"
    [[ -n "$deployment" && -f "$REF_FILE" && ! -L "$REF_FILE" && -O "$REF_FILE" && "$(sed -n '2p' "$REF_FILE")" == "$REPO_ID" && "$(sed -n '3p' "$REF_FILE")" == "$deployment" && "$(sed -n '4p' "$REF_FILE")" == "$(id -u)" ]] || { print -u2 '本机引用不匹配，保留引用；反部署未完成。'; return 2; }
    /bin/rm -- "$REF_FILE"
    /bin/rmdir -- "$REF_DIR" 2>/dev/null || true
}
# 反部署分开选择仅停保留和清理，断点恢复不重新备份或扩大原清理范围。
undeploy_instance() {
    [[ -d "$STATE" ]] || { print '此state尚未创建，没有反部署对象。'; return 0; }
    assert_owner
    origin_load || return $?
    origin_verify_tree || return $?
    confirm_action
    prepare_selector
    find_tools
    pick_operation '反部署' '默认仅停止并保留；Esc取消，不删除数据。' $'retain\t仅停止并保留全部资料' $'cleanup\t进入反部署清理' || return $?
    case "$SELECTED_OPERATION" in
        retain)
            acquire_lock; RUN_ACTIVE=1
            stop_backend || return $?; stop_tunnel || return $?; stop_nats || return $?; stop_clusters || return $?
            RUN_ACTIVE=0; release_lock
            print '本实例已停止；数据及所有原点资源保留。'
            return 0 ;;
        cleanup) ;;
        *) return 2 ;;
    esac
    local work="$STATE/.Deployment/undeploy" phase=""
    if [[ -f "$work/phase" ]]; then
        phase="$(cat "$work/phase")"
        if [[ "$phase" == complete-protected-retained ]]; then print '此前清理已完成可清范围；受保护资料保留，结果仍为partial。'; return 0; fi
        [[ "$phase" == prepared || "$phase" == deleting ]] || { print -u2 '清理断点不完整，保留现场。'; return 2; }
        DATA_CHOICE="$(cat "$work/data-choice")"
        acquire_lock
        undeploy_verify_plan || return $?
        confirm_cleanup || return $?
        undeploy_delete_plan || return $?
        release_lock
        return 0
    fi
    [[ ! -e "$work" ]] || { print -u2 '存在未封存清理准备目录，保留并停止；不重新初始化。'; return 2; }
    pick_operation '数据处理' '默认备份后清理；清零不可恢复；Esc取消。' $'backup\t数据库数据备份（默认，验真后清理）' $'clear\t数据库数据清零（保留独立journal与匹配钥）' || return $?
    DATA_CHOICE="$SELECTED_OPERATION"
    [[ "$DATA_CHOICE" == backup || "$DATA_CHOICE" == clear ]] || return 2
    acquire_lock; RUN_ACTIVE=1
    stop_backend || return $?; stop_tunnel || return $?; stop_nats || return $?
    if [[ "$DATA_CHOICE" == backup ]]; then
        SOURCE_BACKUP_DIGEST="$(backup_source_fingerprint)" || return $?
        backup_instance || return $?
        [[ "$(backup_source_fingerprint)" == "$SOURCE_BACKUP_DIGEST" ]] || { print -u2 '备份期间应用/附件/匹配密钥变化；保留源与原归档，不重新备份后清理。'; return 2; }
    fi
    stop_clusters || return $?; RUN_ACTIVE=0
    origin_verify_tree || return $?
    undeploy_snapshot || return $?
    confirm_cleanup || { release_lock; return 130; }
    undeploy_delete_plan || return $?
    release_lock
}

# 下载校验、稳定版本、引导工具与有界子进程复用Jobs维修平台已验证结构。
hash_file() {
  if command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | awk '{print $1}';
  elif command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}';
  else fail '缺少系统SHA256工具，不能跳过下载校验。'; fi
}
# 官方归档逐次有界获取；网络故障持续重试，摘要/权限/来源错误直接停止。
download_verified() {
  local source_url="$1" target_file="$2" expected="$3" actual code http_code delay=1 transfer_log="$2.transfer.log"
  case "$source_url" in https://*) ;; *) fail '官方来源必须为HTTPS。';; esac
  require_host_space 0 "${target_file:h}" || return $?
  log_message '0;34' "来源｜$source_url"
  log_message '0;34' "目标｜$target_file"
  while :; do
    if [ -f "$target_file.part" ] && [ "$(hash_file "$target_file.part")" = "$expected" ]; then mv "$target_file.part" "$target_file" || fail '无法保存校验后的归档。'; return 0; fi
    code=0; http_code=''
    if command -v curl >/dev/null 2>&1; then
      run_dependency_command "$target_file.http" curl --fail --location --show-error --connect-timeout 10 --max-time 240 --max-filesize "$HOST_ALLOWANCE" --continue-at - --write-out '%{http_code}' --output "$target_file.part" "$source_url" || code=$?
      http_code="$(cat "$target_file.http" 2>/dev/null)"
      [ "$code" -lt 128 ] || exit "$code"
      if [ "$code" != 0 ]; then
        case "$code:$http_code" in
          33:*|*:416) : > "$target_file.part";;
          6:*|7:*|18:*|28:*|35:*|52:*|55:*|56:*|*:408|*:429|*:5[0-9][0-9]) ;;
          *) fail "下载被拒绝或本机条件不满足（curl=$code，HTTP=$http_code）；不使用部分文件。";;
        esac
      fi
    elif command -v wget >/dev/null 2>&1; then
      run_dependency_command "$transfer_log" wget --server-response --continue --timeout=30 --tries=1 -O "$target_file.part" "$source_url" || code=$?
      [ "$code" -lt 128 ] || exit "$code"
      cat "$transfer_log" >> "$LOG_FILE"
      http_code="$(awk '/HTTP\/[0-9.]+ [0-9][0-9][0-9]/{value=$2} END{print value}' "$transfer_log")"
      if [ "$code" != 0 ]; then
        case "$code:$http_code" in
          4:*|*:408|*:429|*:5[0-9][0-9]) ;;
          *) fail "下载被拒绝或本机条件不满足（wget=$code，HTTP=$http_code）；不使用部分文件。";;
        esac
      fi
    else fail '系统缺少curl/wget；请按本系统文档准备基础下载命令。'; fi
    if [ "$code" = 0 ]; then
      actual="$(hash_file "$target_file.part")"
      [ "$actual" = "$expected" ] || fail "SHA256不匹配，停止使用下载文件：$target_file.part"
      mv "$target_file.part" "$target_file" || fail '无法保存已校验归档。'
      return 0
    fi
    dependency_retry_wait "$delay" "官方归档暂未获取完成"
    [ "$delay" -ge 15 ] || delay=$((delay * 2)); [ "$delay" -le 15 ] || delay=15
  done
}
# 只为缺失工具读取本仓锁定清单并安装到机器环境缓存。
install_cached_tool() {
  local tool="$1" row="${2:-}" version filename expected source_url kind cache archive staging candidate tool_directory version_directory download_directory
  [ -n "$row" ] || row="$(awk -F '\t' -v t="$tool" -v o="$PLATFORM_OS" -v a="$PLATFORM_ARCH" '$1==t&&$2==o&&$3==a{print;exit}' "$PROJECT_DIR/部署/deploy-tools.lock.tsv")"
  [ -n "$row" ] || fail "缺少${tool}的$PLATFORM_OS/${PLATFORM_ARCH}官方锁定包。"
  version="$(printf '%s\n' "$row" | cut -f4)"; filename="$(printf '%s\n' "$row" | cut -f5)"
  expected="$(printf '%s\n' "$row" | cut -f6)"; source_url="$(printf '%s\n' "$row" | cut -f7)"; kind="$(printf '%s\n' "$row" | cut -f8)"
  case "$tool" in go) tool_directory=Go; version_directory="go$version";; fzf) tool_directory=Fzf; version_directory="$version";; cloudflared) tool_directory=Cloudflared; version_directory="$version";; esac
  cache="$PROJECT_ENV_DIR/Tools/$tool_directory/$version_directory/$PLATFORM_LABEL-$PLATFORM_ARCH"
  [ ! -L "$ENV_ROOT" ] && [ ! -L "$PROJECT_ENV_DIR/Tools/$tool_directory" ] && [ ! -L "$PROJECT_ENV_DIR/Tools/$tool_directory/$version_directory" ] && [ ! -L "$cache" ] || fail '工具缓存目录是软链接，拒绝跟随到项目外。'
  [ ! -L "$cache/go" ] && [ ! -L "$cache/go/bin" ] && [ ! -L "$cache/$tool" ] || fail '工具缓存内部存在软链接，需先人工核对。'
  if [ "$tool" = go ]; then candidate="$cache/go/bin/go"; else candidate="$cache/$tool"; fi
  if tool_healthy "$tool" "$candidate"; then
    if [ -z "${DEPENDENCY_RESOLVER:-}" ] || tool_meets_version "$tool" "$candidate" "$version"; then TOOL_RESULT="$candidate"; return 0; fi
  fi
  mkdir -p "$(dirname "$cache")" || fail '工具缓存目录不可写。'
  [ ! -L "$PROJECT_ENV_DIR/Downloads" ] || fail '下载缓存目录是软链接，拒绝跟随。'
  mkdir -p "$PROJECT_ENV_DIR/Downloads" || fail '无法创建机器下载缓存。'
  download_directory="$(mktemp -d "$PROJECT_ENV_DIR/Downloads/$tool-$version-$PLATFORM_LABEL-$PLATFORM_ARCH.XXXXXXXX")" || fail '无法创建独立下载目录。'
  archive="$download_directory/$filename"; staging="$download_directory/Extract"
  download_verified "$source_url" "$archive" "$expected"
  mkdir -p "$staging" || fail '无法创建独立解包目录。'
  case "$kind" in
    tar.gz|tgz) local expanded_bytes="$(/usr/bin/gzip -dc "$archive" | wc -c | tr -d ' ')"; require_host_space "$expanded_bytes" "$staging" || return $?; command -v tar >/dev/null 2>&1 || fail '缺少系统tar。'; tar -xzf "$archive" -C "$staging" >> "$LOG_FILE" 2>&1 || fail "解包${tool}失败。";;
    binary) require_host_space "$(stat -f %z "$archive")" "$staging" || return $?; cp "$archive" "$staging/$tool" || fail '无法保存工具。';;
    *) fail "此Shell入口不处理归档类型：$kind";;
  esac
  if [ "$tool" = go ]; then candidate="$staging/go/bin/go"; else candidate="$staging/$tool"; fi
  [ -f "$candidate" ] && [ ! -L "$candidate" ] || fail '归档缺少预期普通可执行文件。'
  chmod u+x "$candidate" || fail '无法赋予本次工具执行位。'
  tool_healthy "$tool" "$candidate" || fail "安装后${tool}健康复检失败。"
  if [ -n "${DEPENDENCY_RESOLVER:-}" ]; then tool_meets_version "$tool" "$candidate" "$version" || fail "安装后${tool}版本不达标。"; fi
  printf '%s\n' "$expected" > "$staging/artifact.sha256"
  hash_file "$candidate" > "$staging/executable.sha256"
  if [ -e "$cache" ]; then mv "$cache" "$RUN_DIR/tool-before-$tool" || fail '无法保留原损坏工具缓存。'; fi
  mv "$staging" "$cache" || fail '工具缓存安装失败。'
  if [ "$tool" = go ]; then TOOL_RESULT="$cache/go/bin/go"; else TOOL_RESULT="$cache/$tool"; fi
  log_message '0;34' "已准备｜$tool $version｜来源 $source_url"
}
# 仅按本轮PID、启动时间与命令快照回收下载/检查子树。
stop_dependency_tree() {
  local pid="$1" identity="$2" child child_identity
  [ "$pid" != "$$" ] || return 0
  [ -n "$identity" ] && [ "$(LC_ALL=C ps -p "$pid" -o lstart= -o command= 2>/dev/null)" = "$identity" ] || return 0
  for child in $(LC_ALL=C ps -axo pid=,ppid= | awk -v parent="$pid" '$2==parent{print $1}'); do
    [ "$(LC_ALL=C ps -p "$pid" -o lstart= -o command= 2>/dev/null)" = "$identity" ] || return 0
    [ "$(LC_ALL=C ps -p "$child" -o ppid= 2>/dev/null | tr -d ' ')" = "$pid" ] || continue
    child_identity="$(LC_ALL=C ps -p "$child" -o lstart= -o command= 2>/dev/null)"
    stop_dependency_tree "$child" "$child_identity"
  done
  [ "$(LC_ALL=C ps -p "$pid" -o lstart= -o command= 2>/dev/null)" = "$identity" ] || return 0
  kill -TERM "$pid" 2>/dev/null || true
  sleep .1
  if [ "$(LC_ALL=C ps -p "$pid" -o lstart= -o command= 2>/dev/null)" = "$identity" ]; then kill -KILL "$pid" 2>/dev/null || true; fi
}
# 工作进程先登记再启动命令，父入口异常退出时独立收回本轮子树。
dependency_command_worker() {
  local owner="$1" owner_identity="$2" output="$3" worker child='' child_started='' result=0
  shift 3
  if [ -n "${ZSH_VERSION:-}" ]; then zmodload zsh/system || return 1; worker="${sysparams[pid]}";
  elif [ -n "${BASHPID:-}" ]; then worker="$BASHPID";
  else worker="$(exec /bin/sh -c 'printf "%s" "$PPID"')"; fi
  [ -n "$worker" ] && [ "$worker" != "$owner" ] || return 2
  [ ! -L "$RUN_DIR/dependency-workers.tsv" ] || return 1
  printf '%s\t%s\n' "$worker" "$(LC_ALL=C ps -p "$worker" -o lstart= -o command= 2>/dev/null)" >> "$RUN_DIR/dependency-workers.tsv" || return 1
  DEPENDENCY_WORKER_PID="$worker"; DEPENDENCY_WORKER_CHILD=''; DEPENDENCY_WORKER_STARTED=''
  dependency_worker_finish() {
    trap - EXIT
    trap '' HUP INT TERM
    local identity
    local owned="${DEPENDENCY_WORKER_CHILD:-}"
    if [ -n "$owned" ] && [ "$(LC_ALL=C ps -p "$owned" -o ppid= 2>/dev/null | tr -d ' ')" = "$DEPENDENCY_WORKER_PID" ] && [ "$(LC_ALL=C ps -p "$owned" -o lstart= 2>/dev/null)" = "$DEPENDENCY_WORKER_STARTED" ]; then
      identity="$(LC_ALL=C ps -p "$owned" -o lstart= -o command= 2>/dev/null)"
      stop_dependency_tree "$owned" "$identity"
    fi
    [ -z "$owned" ] || wait "$owned" 2>/dev/null || true
  }
  trap 'dependency_worker_finish' EXIT
  trap 'exit 129' HUP
  trap 'exit 130' INT
  trap 'exit 143' TERM
  "$@" > "$output" 2>> "$LOG_FILE" &
  child=$!
  child_started="$(LC_ALL=C ps -p "$child" -o lstart= 2>/dev/null)"
  DEPENDENCY_WORKER_CHILD="$child"; DEPENDENCY_WORKER_STARTED="$child_started"
  while kill -0 "$child" 2>/dev/null; do
    [ "$(LC_ALL=C ps -p "$owner" -o lstart= -o command= 2>/dev/null)" = "$owner_identity" ] || return 129
    sleep .2
  done
  if wait "$child"; then result=0; else result=$?; fi
  child=''; DEPENDENCY_WORKER_CHILD=''
  return "$result"
}
# 前台仅等待本轮监督子进程，退出清理可即时中断其实际命令。
run_dependency_command() {
  local output="$1" owner_identity worker result
  shift
  owner_identity="$(LC_ALL=C ps -p "$$" -o lstart= -o command= 2>/dev/null)"
  dependency_command_worker "$$" "$owner_identity" "$output" "$@" &
  worker=$!
  if wait "$worker"; then return 0; else result=$?; return "$result"; fi
}
# 下载/查询产物封存前，先停止本轮仍存活的短作业。
cleanup_dependency_commands() {
  local pid identity
  [ -n "${RUN_DIR:-}" ] && [ -f "$RUN_DIR/dependency-workers.tsv" ] && [ ! -L "$RUN_DIR/dependency-workers.tsv" ] || return 0
  while IFS=$'\t' read -r pid identity; do
    case "$pid" in ''|*[!0-9]*) continue;; esac
    stop_dependency_tree "$pid" "$identity"
  done < "$RUN_DIR/dependency-workers.tsv"
}
# 只给可重试的网络获取等待退避，校验与参数错误直接停止。
dependency_retry_wait() {
  local seconds="$1" reason="$2"
  log_message '1;33' "$reason；${seconds}秒后继续获取，Ctrl+C可取消。"
  run_dependency_command "$RUN_DIR/dependency-wait.out" sleep "$seconds" || exit "$?"
}
# 用本轮私有副本查询和构建，源工程的模块锁始终不写入。
prepare_module_copy() {
  local source="$1" name="$2" directory="$RUN_DIR/module-manifests/$2"
  [ -f "$source/go.mod" ] || fail "缺少模块清单：$source/go.mod"
  mkdir -p "$directory" || fail '无法创建本轮模块清单目录。'
  cp "$source/go.mod" "$directory/module.mod" || fail '无法复制模块清单。'
  if [ -f "$source/go.sum" ]; then cp "$source/go.sum" "$directory/module.sum" || fail '无法复制模块校验锁。'; fi
  MODULE_MODFILE="$directory/module.mod"
}
# 解析器只依赖标准库，由菜单引导阶段已准备的健康Go构建。
prepare_dependency_resolver() {
  local source="$PROJECT_DIR/部署/公共组件/依赖版本"
  prepare_module_copy "$source" resolver
  DEPENDENCY_RESOLVER="$RUN_DIR/dependency-versions"
  log_message '0;34' '依赖检查｜准备本轮官方稳定版本解析器；不会修改源码或全局工具。'
  (cd "$source" && GOROOT= GOWORK=off GOTOOLCHAIN=local GOTELEMETRY=off run_dependency_command "$RUN_DIR/dependency-build.out" "$GO_EXECUTABLE" build -mod=readonly -modfile="$MODULE_MODFILE" -o "$DEPENDENCY_RESOLVER" .) >> "$LOG_FILE" 2>&1 || fail '版本解析器构建失败，停止部署。'
}
# 读取实际稳定版号，拒绝把开发版或预发布输出当作稳定工具。
tool_version() {
  local tool="$1" candidate="$2" info
  case "$tool" in
    go) info="$(GOROOT= GOTOOLCHAIN=local "$candidate" version 2>/dev/null)" || return 1; printf '%s\n' "$info" | sed -nE 's/^go version go([0-9]+\.[0-9]+(\.[0-9]+)?)([[:space:]].*)?$/\1/p';;
    fzf) info="$(FZF_DEFAULT_OPTS='' FZF_DEFAULT_OPTS_FILE=/dev/null FZF_DEFAULT_COMMAND='' "$candidate" --version 2>/dev/null)" || return 1; printf '%s\n' "$info" | sed -nE 's/^([0-9]+\.[0-9]+(\.[0-9]+)?)([[:space:]].*)?$/\1/p';;
    cloudflared) info="$("$candidate" --version 2>/dev/null)" || return 1; printf '%s\n' "$info" | sed -nE 's/^cloudflared version ([0-9]+\.[0-9]+(\.[0-9]+)?)([[:space:]].*)?$/\1/p';;
    *) return 1;;
  esac
}
# 比较器用退出码区分旧版本与非法输入，不把旧版当解析器故障。
tool_at_least() {
  local installed="$1" target="$2" result code
  if result="$("$DEPENDENCY_RESOLVER" --compare "$installed" --at-least "$target" 2>> "$LOG_FILE")"; then
    [ "$result" = 1 ] || fail '版本比较器返回异常。'
    return 0
  else
    code=$?
    if [ "$code" = 1 ] && [ "$result" = 0 ]; then return 1; fi
    fail '无法可靠比较工具版本，停止部署。'
  fi
}
# 只有当前功能与实际稳定版本同时达标，候选工具才可复用。
tool_meets_version() {
  local tool="$1" candidate="$2" target="$3" version
  tool_healthy "$tool" "$candidate" || return 1
  case "$candidate" in "$PROJECT_ENV_DIR"/Tools/*)
    local cache_root="${candidate%/*}"
    [ "$tool" != go ] || cache_root="${candidate%/go/bin/go}"
    [ -f "$cache_root/executable.sha256" ] && [ "$(cat "$cache_root/executable.sha256")" = "$(hash_file "$candidate")" ] || return 1;;
  esac
  version="$(tool_version "$tool" "$candidate")" || return 1
  [ -n "$version" ] && tool_at_least "$version" "$target"
}
# 每次正常部署重新查官方稳定发行版，只有临时网络失败持续重试。
resolve_latest_asset() {
  local tool="$1" row code delay=1 expected_url
  local -a arguments
  arguments=(--tool "$tool" --os "$PLATFORM_OS" --arch "$PLATFORM_ARCH" --locked-manifest "$PROJECT_DIR/部署/deploy-tools.lock.tsv")
  [ "$tool" != go ] || arguments+=(--minimum-go 1.27.0)
  while :; do
    if run_dependency_command "$RUN_DIR/dependency-latest-$tool.tsv" "$DEPENDENCY_RESOLVER" "${arguments[@]}"; then row="$(cat "$RUN_DIR/dependency-latest-$tool.tsv")"; break; else code=$?; fi
    [ "$code" -lt 128 ] || exit "$code"
    [ "$code" = 75 ] || fail "官方${tool}版本或摘要无法确认，停止部署；详见日志。"
    dependency_retry_wait "$delay" "官方${tool}版本查询暂未完成"
    [ "$delay" -ge 15 ] || delay=$((delay * 2)); [ "$delay" -le 15 ] || delay=15
  done
  [ "$(printf '%s\n' "$row" | awk -F '\t' 'NF==8{n++} END{print n+0}')" = 1 ] && [ "$(printf '%s\n' "$row" | wc -l | tr -d ' ')" = 1 ] || fail '官方版本解析行格式不完整。'
  IFS=$'\t' read -r RESOLVED_TOOL RESOLVED_OS RESOLVED_ARCH RESOLVED_VERSION RESOLVED_FILENAME RESOLVED_SHA RESOLVED_URL RESOLVED_KIND <<RESOLVED_ASSET
$row
RESOLVED_ASSET
  [ "$RESOLVED_TOOL/$RESOLVED_OS/$RESOLVED_ARCH" = "$tool/$PLATFORM_OS/$PLATFORM_ARCH" ] || fail '官方版本平台或架构与本机不符。'
  printf '%s\n' "$RESOLVED_VERSION" | grep -Eq '^[0-9]+\.[0-9]+(\.[0-9]+)?$' || fail '官方版本不是稳定数字版本。'
  if [ "$RESOLVED_KIND" = unsupported ]; then
    [ "$RESOLVED_FILENAME/$RESOLVED_SHA/$RESOLVED_URL" = '-/-/-' ] || fail '不支持的平台资产返回格式异常。'
  else
    case "$RESOLVED_KIND" in tar.gz|tgz|binary) ;; *) fail '官方资产归档类型与本Shell平台不符。';; esac
    printf '%s\n' "$RESOLVED_SHA" | grep -Eq '^[a-fA-F0-9]{64}$' || fail '官方资产缺少可信SHA256。'
    case "$RESOLVED_FILENAME" in ''|*/*|*\\*|*:*|.|..) fail '官方资产文件名无效。';; esac
    case "$tool" in go) expected_url="https://go.dev/dl/$RESOLVED_FILENAME";; fzf) expected_url="https://github.com/junegunn/fzf/releases/download/v$RESOLVED_VERSION/$RESOLVED_FILENAME";; cloudflared) expected_url="https://github.com/cloudflare/cloudflared/releases/download/$RESOLVED_VERSION/$RESOLVED_FILENAME";; node) expected_url="https://nodejs.org/dist/v$RESOLVED_VERSION/$RESOLVED_FILENAME";; nats-server) expected_url="https://github.com/nats-io/nats-server/releases/download/v$RESOLVED_VERSION/$RESOLVED_FILENAME";; *) fail '未知工具资产来源，拒绝采用。';; esac
    [ "$RESOLVED_URL" = "$expected_url" ] || fail '官方资产来源与稳定版本不匹配。'
  fi
  RESOLVED_ROW="$row"
}
# 从显式、PATH与所有版本缓存择新；不覆盖显式文件或系统公共工具。
resolve_latest_tool() {
  local tool="$1" explicit='' initial='' candidate version current_version best='' best_version='' directory relative target row kind
  resolve_latest_asset "$tool"
  target="$RESOLVED_VERSION"; row="$RESOLVED_ROW"; kind="$RESOLVED_KIND"
  case "$tool" in
    go) explicit="${GO_BIN:-}"; initial="${GO_EXECUTABLE:-}"; directory=Go; relative=go/bin/go;;
    fzf) explicit="${WB_FZF_BIN:-}"; initial="${FZF_EXECUTABLE:-}"; directory=Fzf; relative=fzf;;
    cloudflared) explicit="${WB_CLOUDFLARED_BIN:-}"; initial="${CLOUDFLARED_EXECUTABLE:-}"; directory=Cloudflared; relative=cloudflared;;
  esac
  [ -n "$initial" ] || initial="$(command -v "$tool" 2>/dev/null || true)"
  current_version="$(tool_version "$tool" "$initial" 2>/dev/null || true)"
  log_message '0;34' "工具版本｜$tool｜当前 ${current_version:-未安装或不可识别}｜官方稳定目标 $target"
  if [ -n "$explicit" ]; then
    case "$explicit" in /*) ;; *) explicit="$PROJECT_DIR/$explicit";; esac
    tool_meets_version "$tool" "$explicit" "$target" || fail "显式${tool}未达到官方稳定目标${target}或功能不兼容；请更新显式配置，原文件保留。"
    best="$explicit"
  else
    [ "$kind" != unsupported ] || fail "官方${tool} ${target}未提供$PLATFORM_OS/${PLATFORM_ARCH}成品；仅可显式指定匹配的原生新版工具。"
    for candidate in "$initial" "$(command -v "$tool" 2>/dev/null || true)" "$ENV_ROOT/$directory"/*/"$PLATFORM_LABEL-$PLATFORM_ARCH/$relative" "$PROJECT_ENV_DIR/Tools/$directory"/*/"$PLATFORM_LABEL-$PLATFORM_ARCH/$relative"; do
      tool_meets_version "$tool" "$candidate" "$target" || continue
      version="$(tool_version "$tool" "$candidate")"
      if [ -z "$best" ] || tool_at_least "$version" "$best_version"; then best="$candidate"; best_version="$version"; fi
    done
    if [ -z "$best" ]; then install_cached_tool "$tool" "$row"; best="$TOOL_RESULT"; fi
    tool_meets_version "$tool" "$best" "$target" || fail "${tool}采用版本复检未达到官方目标，停止部署。"
  fi
  version="$(tool_version "$tool" "$best")"
  log_message '0;34' "采用工具｜$tool $version｜$best"
  case "$tool" in go) GO_EXECUTABLE="$best";; fzf) FZF_EXECUTABLE="$best";; cloudflared) CLOUDFLARED_EXECUTABLE="$best";; esac
}
# 菜单引导完成后，最终构建与运行统一采用重新核验的稳定版本。
prepare_latest_tools() {
  resolve_latest_tool go
  resolve_latest_tool fzf
  # cloudflared在本机健康后按显式公开请求单独准备，失败不终止本机服务。
}
# 仅报告当前模块路径可更新版本，不升级业务库或发现换路径的新主版本。
check_module_updates() {
  local name="$1" source="$2" code output="$RUN_DIR/module-updates-$1.txt" seconds="${WB_DEPENDENCY_CHECK_TIMEOUT_SECONDS:-30}"
  case "$seconds" in ''|*[!0-9]*) fail 'WB_DEPENDENCY_CHECK_TIMEOUT_SECONDS须为1..300秒。';; esac
  [ "$seconds" -ge 1 ] && [ "$seconds" -le 300 ] || fail '业务依赖检查超时须为1..300秒。'
  prepare_module_copy "$source" "$name-check"
  log_message '0;34' "业务依赖检查｜$name｜最多${seconds}秒，仅检查当前模块路径；源码依赖锁保持不变。"
  if (configure_go_module_source && GOROOT= run_dependency_command "$output" "$DEPENDENCY_RESOLVER" --module-report --go-bin "$GO_EXECUTABLE" --module-dir "$source" --modfile "$MODULE_MODFILE" --timeout-seconds "$seconds"); then
    if [ -s "$output" ]; then
      log_message '1;33' "业务库可更新｜$name｜需单独升级与回归："
      awk -F '\t' 'NF==3{printf "%s %s -> %s\n",$1,$2,$3}' "$output" | tee -a "$LOG_FILE"
    else log_message '0;34' "业务依赖检查完成｜${name}当前模块路径未报告更新；未检查换路径的新主版本。"; fi
  else
    code=$?
    [ "$code" = 3 ] || fail "业务依赖检查的参数、路径或源码锁校验失败（$name，退出码$code）；停止核对。" "$code"
    log_message '1;33' "业务依赖检查未完成或超时｜$name｜按现有锁构建，不视为已确认最新；详见日志。"
  fi
}
# 后端和二维码各自独立检查，客户端工具链不在本部署中暗中升级。
check_business_dependencies() {
  check_module_updates backend "$PROJECT_DIR/代码/后端/Go"
  check_module_updates qr "$PROJECT_DIR/部署/公共组件/地址二维码"
}

# 对实际调用的工具执行最小版本和必要参数健康检查。
tool_healthy() {
  local tool="$1" candidate="$2" info minor
  [ -n "$candidate" ] && [ -x "$candidate" ] || return 1
  case "$tool" in
    go) info="$(GOROOT= GOTOOLCHAIN=local GOTELEMETRY=off "$candidate" version 2>/dev/null)" || return 1; minor="$(printf '%s\n' "$info" | sed -n 's/.*go1\.\([0-9][0-9]*\).*/\1/p')"; [ -n "$minor" ] && [ "$minor" -ge 27 ];;
    fzf) FZF_DEFAULT_OPTS='' FZF_DEFAULT_OPTS_FILE=/dev/null FZF_DEFAULT_COMMAND='' "$candidate" --version >/dev/null 2>&1 && FZF_DEFAULT_OPTS='' FZF_DEFAULT_OPTS_FILE=/dev/null FZF_DEFAULT_COMMAND='' "$candidate" --help 2>/dev/null | grep -- '--expect' >/dev/null &&
      printf 'jobsim-selector-probe\n' | FZF_DEFAULT_OPTS='' FZF_DEFAULT_OPTS_FILE=/dev/null FZF_DEFAULT_COMMAND='' "$candidate" --no-height --bind='ctrl-c:print(__jobsim_interrupt)+accept' --filter=jobsim-selector-probe >/dev/null 2>&1;;
    cloudflared)
      "$candidate" --version >/dev/null 2>&1 || return 1
      info="$("$candidate" tunnel --help 2>/dev/null)" || return 1
      printf '%s\n' "$info" | grep -- '--url' >/dev/null && printf '%s\n' "$info" | grep -- '--config' >/dev/null && printf '%s\n' "$info" | grep -- '--no-autoupdate' >/dev/null;;
    *) return 1;;
  esac
}
# 选择模式前只读复用健康fzf；缺失时放本次会话，不改旧部署清单。
prepare_fzf() {
  local candidate="${WB_FZF_BIN:-$(command -v fzf 2>/dev/null || true)}" previous_env="$PROJECT_ENV_DIR" project_id
  if tool_healthy fzf "$candidate"; then FZF_EXECUTABLE="$candidate"; return 0; fi
  project_id="$(printf '%s' "$PROJECT_DIR" | cksum | awk '{print $1}')"
  for candidate in "$ENV_ROOT/Projects/JobsIM/$project_id"/Tools/Fzf/*/"$PLATFORM_LABEL-$PLATFORM_ARCH"/fzf; do
    if tool_healthy fzf "$candidate"; then FZF_EXECUTABLE="$candidate"; return 0; fi
  done
  PROJECT_ENV_DIR="$RUN_DIR/bootstrap-environment"
  install_cached_tool fzf
  FZF_EXECUTABLE="$TOOL_RESULT"
  PROJECT_ENV_DIR="$previous_env"
}
# 用fzf选择两种部署模式，取消和空选不会停止旧实例或启动业务。
# 本地模式只需回车确认公开范围，取消时保留原运行实例。
# 发现可用Go，显式配置错误时不擅自更换到其它版本。
prepare_go() {
  local candidate=""
  if [ -n "${GO_BIN:-}" ]; then
    candidate="$GO_BIN"; case "$candidate" in /*) ;; *) candidate="$PROJECT_DIR/$candidate";; esac
    tool_healthy go "$candidate" || fail 'GO_BIN指定工具不可用或低于Go1.27。'
    GO_EXECUTABLE="$candidate"; return 0
  fi
  candidate="$(command -v go 2>/dev/null || true)"
  if tool_healthy go "$candidate"; then GO_EXECUTABLE="$candidate"; return 0; fi
  for candidate in "$ENV_ROOT"/Go/go*/"$PLATFORM_LABEL-$PLATFORM_ARCH"/go/bin/go; do
    if tool_healthy go "$candidate"; then GO_EXECUTABLE="$candidate"; return 0; fi
  done
  install_cached_tool go; GO_EXECUTABLE="$TOOL_RESULT"
}
# 为本地模式复用健康cloudflared，缺失时校验安装锁定版本。

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
    pick_operation '部署模式' '↑/↓选择、回车确认；Esc取消。本地默认仅本机；临时穿透需另选并确认。' $'local\t本地部署' $'server\t服务器部署' || return $?
    [[ "$SELECTED_OPERATION" == local || "$SELECTED_OPERATION" == server ]] || return 2
    DEPLOY_MODE="$SELECTED_OPERATION"
    if [[ "$DEPLOY_MODE" == local ]]; then
        PUBLIC_HOST=127.0.0.1; JOBS_IM_PUBLIC_URL="https://127.0.0.1:$HTTPS_PORT"
        pick_operation '本地访问范围' '默认仅本机，不准备穿透工具；Esc取消。' $'private\t仅本机（默认）' $'quick\t临时穿透（另行确认公开）' || return $?
        [[ "$SELECTED_OPERATION" == private || "$SELECTED_OPERATION" == quick ]] || return 2
        LOCAL_ACCESS="$SELECTED_OPERATION"
        [[ "$LOCAL_ACCESS" != private ]] || { print '仅本机模式：不准备cloudflared、不建立公网入口。'; return 0; }
        terminal_notice
        print '本轮仅公开JobsIM用户入口；持有临时网址的人可以访问，业务登录/权限继续生效。仅使用合成资料。'
        local answer
        printf '已了解公开范围，按回车继续；其它输入或输入结束取消：'
        IFS= read -r answer || return 130
        [[ -z "$answer" ]] || return 130
    else
        local server_url="${JOBS_IM_PUBLIC_URL:-}"
        if [[ -z "$server_url" ]]; then printf '服务器HTTPS origin（例如https://chat.example.com，无路径；空输入取消）：'; IFS= read -r server_url || return 130; fi
        [[ "$server_url" =~ '^https://[A-Za-z0-9][A-Za-z0-9.-]*(:[0-9]+)?/?$' ]] || { print -u2 '服务器模式必须提供明确HTTPS origin。'; return 2; }
        JOBS_IM_PUBLIC_URL="${server_url%/}"
        PUBLIC_HOST="${${JOBS_IM_PUBLIC_URL#https://}%%:*}"
        print '服务器模式不建穿透；外部反向代理/防火墙由部署方配置，TLS须显式提供并核验SAN和信任链。'
    fi
}
# 在相同支持主版本内核对数据库目标，绝不在部署中跨major升级既有集群。
verify_native_tool_targets() {
    local tool candidate current target latest row kind cache archive extraction selected name
    for tool in node nats-server postgres; do
        resolve_latest_asset "$tool"
        latest="$RESOLVED_VERSION"; target="$latest"; row="$RESOLVED_ROW"; kind="$RESOLVED_KIND"
        case "$tool" in
            node) candidate="${JOBS_IM_NODE:-$(command -v node || true)}"; current="$("$candidate" --version 2>/dev/null | sed -n 's/^v//p')" || current=0.0.0 ;;
            nats-server) candidate="$NATS_BIN"; current="$("$candidate" --version 2>/dev/null | sed -nE 's/.*v([0-9]+\.[0-9]+\.[0-9]+).*/\1/p')" || current=0.0.0 ;;
            postgres) candidate="$PG_BIN/pg_ctl"; current="$("$candidate" --version 2>/dev/null | sed -nE 's/.* ([0-9]+\.[0-9]+).*/\1/p')"; target="$(print -r -- "$row" | cut -f4)" ;;
        esac
        [[ -n "$current" ]] || fail "无法核验当前${tool}版本。"
        if [[ "$tool" == postgres && "${latest%%.*}" != "${current%%.*}" ]]; then
            print "数据库官方最新=$latest；当前受支持主版本=${current%%.*}，现有集群不跨主版本自动升级，兼容补丁目标须单独核对。"
            [[ "${current%%.*}" == 18 ]] || fail 'JobsIM当前部署仅支持既有PG18配套工具；不会把其他major当已适配。'
            continue
        fi
        print "工具核验：$tool；实际=$current；官方目标=$target；时间=$(date -u +%Y-%m-%dT%H:%M:%SZ)；位置=$candidate"
        if tool_at_least "$current" "$target"; then continue; fi
        [[ "$tool" != postgres && "$kind" != unsupported ]] || fail "当前${tool}未达官方目标；缺可验证本平台原生包时须由操作者提供健康候选，不改既有数据库。"
        case "$tool" in node) [[ -z "${JOBS_IM_NODE:-}" ]] || fail '显式Node过旧，保留原文件。';; nats-server) [[ -z "${JOBS_IM_NATS_BIN:-}" ]] || fail '显式NATS过旧，保留原文件。';; esac
        cache="$PROJECT_ENV_DIR/Tools/$tool/$target/$PLATFORM_OS-$PLATFORM_ARCH"
        [[ ! -e "$cache" && ! -L "$cache" ]] || fail '目标工具缓存已存在但未通过健康验证，保留不覆盖。'
        mkdir -p "${cache:h}"
        extraction="$(mktemp -d "$RUN_DIR/native-tool.XXXXXX")"
        archive="$extraction/archive"
        download_verified "$RESOLVED_URL" "$archive" "$RESOLVED_SHA"
        mkdir "$extraction/unpacked"
        local expanded_bytes="$(/usr/bin/gzip -dc "$archive" | wc -c | tr -d ' ')"
        require_host_space "$expanded_bytes" "$extraction/unpacked" || return $?
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
    PREPARING_TOOLS=1
    JOBS_IM_GO="$GO_TOOL"
    find_tools
    verify_native_tool_targets
    PREPARING_TOOLS=0
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
    [[ "$DEPLOY_MODE" == local && "$LOCAL_ACCESS" == quick ]] || return 0
    print '{}' > "$RUN_DIR/quick-empty.yml"
    run_im_quick_client > "$RUN_DIR/tunnel.log" 2>&1 &
    TUNNEL_PID=$!
    local identity_attempt=0
    while [[ "$(ps -ww -p "$TUNNEL_PID" -o command= 2>/dev/null || true)" != *"$CLOUDFLARED_EXECUTABLE"* && $identity_attempt -lt 20 ]]; do sleep .05; (( identity_attempt+=1 )); done
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
    stop_backend || return $?
    launch_im_backend "$PUBLIC_URL" || return $?
    curl --fail --silent --show-error --max-time 20 "$PUBLIC_URL/api/health" > "$RUN_DIR/public-health.json" || return $?
    print '实际公网入口（仍须独立外网业务验收）：'; show_endpoint "$PUBLIC_URL"
}
# 只停止本轮登记且启动身份和完整命令相符的隧道。
stop_tunnel() {
    [[ -f "$STATE/Processes/tunnel.pid" ]] || return 0
    local pid="$(cat "$STATE/Processes/tunnel.pid")" started command
    [[ "$pid" == <-> ]] || return 2
    process_live "$pid" || return 0
    started="$(LC_ALL=C ps -p "$pid" -o lstart= 2>/dev/null || true)"
    command="$(ps -ww -p "$pid" -o command= 2>/dev/null || true)"
    [[ -n "$started" && "$started" == "$(cat "$STATE/Processes/tunnel.start")" && "$command" == "$(cat "$STATE/Processes/tunnel.command")" && "$command" == *"--origin-ca-pool $STATE/TLS/ca.crt"* ]] || { print -u2 '隧道PID身份不符，保留进程。'; return 2; }
    kill -TERM "$pid"
    local attempts=0
    while process_live "$pid" && (( attempts < 50 )); do sleep .1; (( attempts+=1 )); done
    ! process_live "$pid" || return 2
}
# 通过本机解析覆盖验证证书真实SAN，不要求正式域名证书包含127.0.0.1。
local_api_json() {
    local route="$1" timeout="${2:-5}"
    local origin_ip=127.0.0.1
    [[ "$ACTION" != demo-run ]] || origin_ip="$JOBS_IM_DEMO_BIND_IP"
    curl --fail --silent --show-error --cacert "$STATE/TLS/ca.crt" --resolve "$PUBLIC_HOST:$HTTPS_PORT:$origin_ip" --max-time "$timeout" "https://$PUBLIC_HOST:$HTTPS_PORT$route"
}
# 用真实部署URL启动同一个Go候选，身份、PG与NATS均不重新初始化。
launch_im_backend() {
    local url="$1" ready=0 count=0 pid
    local listen_ip=127.0.0.1
    [[ "$ACTION" != demo-run ]] || listen_ip="$JOBS_IM_DEMO_BIND_IP"
    JOBS_IM_JOURNAL_DATABASE_URL="$JOBS_IM_JOURNAL_DATABASE_URL" JOBS_IM_PUBLIC_URL="$url" "$STATE/Bin/JobsIMd" --service all --addr "$listen_ip:$HTTPS_PORT" --internal-base-port "$ROLE_PORT" --state-dir "$STATE/Application" --tls-cert "$STATE/TLS/server.crt" --tls-key "$STATE/TLS/server.key" --web-dir "$WEB_RUNTIME" > "$STATE/Logs/server.log" 2>&1 &
    pid=$!; SERVER_PID="$pid"
    print "$pid" > "$STATE/Processes/server.pid"
    LC_ALL=C ps -p "$pid" -o lstart= > "$STATE/Processes/server.start"
    while (( count < 60 )); do
        if local_api_json /api/health 1 >/dev/null 2>&1; then ready=1; break; fi
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
    local url="${PUBLIC_URL:-${JOBS_IM_PUBLIC_URL:-https://$PUBLIC_HOST:$HTTPS_PORT}}" width=0 height=0 size
    local -a dimensions display_options; display_options=()
    QR_TERMINAL_SHOWN=0
    if [[ -t 1 ]]; then
        size="$(/bin/stty size < /dev/tty 2>/dev/null || true)"; dimensions=(${=size})
        if (( ${#dimensions} == 2 )) && [[ "${dimensions[1]}" == <-> && "${dimensions[2]}" == <-> ]]; then height="${dimensions[1]}"; width="${dimensions[2]}"; fi
        if (( width == 0 )); then width="${COLUMNS:-0}"; [[ "$width" == <-> ]] || width=0; fi
        if [[ "${TERM:-dumb}" != dumb && -n "${TERM:-}" && -z "${NO_COLOR+x}" ]]; then display_options+=(--ansi); fi
    fi
    local label=用户端
    [[ "$url" != https://127.0.0.1:* ]] || label=仅本机，手机不可用
    local -a entries; entries=(--entry "$label|$url/")
    [[ -z "$PUBLIC_URL" ]] || entries+=(--entry "仅本机，手机不可用|https://127.0.0.1:$HTTPS_PORT/")
    local display="$RUN_DIR/qr-display.tmp"
    "$QR_BINARY" --output-dir "$RUN_DIR/AccessQR" --title 'JobsIM访问入口' "${entries[@]}" --terminal-width "$width" --terminal-height "$height" "${display_options[@]}" > "$display" 2>&1 || { print -u2 '二维码生成失败；保留URL，业务继续。'; return 0; }
    /usr/bin/sed -E $'s/\033\\[[0-9;]*[[:alpha:]]//g' "$display" > "$RUN_DIR/qr-output.txt"
    if [[ -t 1 ]]; then cat "$display"; else cat "$RUN_DIR/qr-output.txt"; fi
    /bin/rm -- "$display"
    [[ ! -f "$RUN_DIR/AccessQR/index.html" ]] || QR_PAGE="$RUN_DIR/AccessQR/index.html"
    if [[ -t 1 ]]; then
        if /usr/bin/grep -Fq '终端二维码（' "$RUN_DIR/qr-output.txt"; then
            QR_TERMINAL_SHOWN=1
            print '二维码保留在终端供查看；按回车打开网页菜单，Esc可收起菜单。'
        fi
        # NO_COLOR不能固定终端黑白极性；尺寸不足时也用本轮PNG确保完整可见。
        if (( ${#display_options} == 0 )) || ! /usr/bin/grep -Fq '终端二维码（' "$RUN_DIR/qr-output.txt"; then
            local -a images; images=("$RUN_DIR/AccessQR"/*.png(N))
            if (( ${#images} > 0 )) && open -a Preview "${images[@]}"; then print '已在本机预览打开本轮黑白二维码图片；回环地址仍仅本机可用。'
            else print -u2 '本机二维码预览未能打开；请使用上方完整URL或本地页面。'; fi
        fi
    fi
}
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

# 已存在的TLS只核对不覆盖；服务器模式要求明确的URL、证书、私钥与信任链。
prepare_server_tls() {
    local cert="${JOBS_IM_TLS_CERT:-}" key="${JOBS_IM_TLS_KEY:-}" ca="${JOBS_IM_TLS_CA:-}" item
    for item in "$cert" "$key" "$ca"; do
        [[ -n "$item" && -f "$item" && ! -L "$item" ]] || { print -u2 '服务器模式需JOBS_IM_TLS_CERT/JOBS_IM_TLS_KEY/JOBS_IM_TLS_CA指定真实证书及匹配私钥/信任链。'; return 2; }
    done
    local -a host_check; host_check=(-verify_hostname "$PUBLIC_HOST")
    [[ ! "$PUBLIC_HOST" =~ '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' ]] || host_check=(-verify_ip "$PUBLIC_HOST")
    "$OPENSSL" verify -x509_strict -purpose sslserver "${host_check[@]}" -CAfile "$ca" -untrusted "$cert" "$cert" || return $?
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
# 菜单独占备用屏，避免窗口缩放将旧边框卷入主缓冲；仍只登记本轮进程身份。
start_im_browser_menu() {
    local menu_file="$RUN_DIR/menu.tsv"
    : > "$menu_file"
    if [[ "$DEPLOY_MODE" == server ]]; then printf 'server\t服务器配置入口\n' >> "$menu_file"; else printf 'local\t本机用户端\n' >> "$menu_file"; fi
    if [[ "$DEPLOY_MODE" == local ]]; then printf 'retry\t重试临时穿透（需重新确认公开）\ndiag\t查看穿透诊断\n' >> "$menu_file"; fi
    printf 'stop\t停止本轮服务并保留资料\n' >> "$menu_file"
    [[ -z "$PUBLIC_URL" ]] || printf 'public\t公网用户端\n' >> "$menu_file"
    [[ -z "$QR_PAGE" ]] || printf 'qr\t打开离线扫码页面\n' >> "$menu_file"
    printf 'pause\t收起菜单（服务继续，回车重开）\n' >> "$menu_file"
    FZF_DEFAULT_OPTS='' FZF_DEFAULT_OPTS_FILE=/dev/null FZF_DEFAULT_COMMAND='' "$FZF_EXECUTABLE" --no-sort --no-multi --delimiter=$'\t' --with-nth=2 --no-height --border --bind='up:up,down:down,ctrl-c:print(__jobsim_interrupt)+accept' --prompt='打开网页 > ' --header='回车交给浏览器打开；Esc收起菜单；Ctrl+C停止本轮服务。' < "$menu_file" > "$RUN_DIR/menu-result" &
    BROWSER_MENU_PID=$!
    BROWSER_MENU_IDENTITY="$(LC_ALL=C ps -p "$BROWSER_MENU_PID" -o lstart= -o command= 2>/dev/null || true)"
}
# 菜单与实际服务一起监控，浏览器失败或收起不丢失服务生命周期。
monitor_im_session() {
    local paused="${QR_TERMINAL_SHOWN:-0}" input code result
    while kill -0 "$SERVER_PID" 2>/dev/null; do
        if [[ -n "$TUNNEL_PID" ]] && ! process_live "$TUNNEL_PID"; then
            print '穿透进程已退出，撤销旧公网地址并恢复本机。'
            recover_private_after_tunnel || return $?
            paused="${QR_TERMINAL_SHOWN:-0}"
        fi
        if [[ -n "$BROWSER_MENU_PID" ]]; then
            if ! kill -0 "$BROWSER_MENU_PID" 2>/dev/null; then
                code=0; wait "$BROWSER_MENU_PID" || code=$?
                BROWSER_MENU_PID=""
                result="$(cat "$RUN_DIR/menu-result")"
                # fzf 原始输入模式会消费 Ctrl+C；显式标记才退出会话，Esc 仍仅收起。
                [[ "${result%%$'\n'*}" != __jobsim_interrupt ]] || return 130
                if (( code != 0 )) || [[ -z "$result" ]]; then paused=1
                else
                    case "${result%%$'\t'*}" in
                        server) open "$JOBS_IM_PUBLIC_URL/" || print '服务器入口打开失败，可重试。' ;;
                        local) open "https://127.0.0.1:$HTTPS_PORT/" || print '浏览器打开失败，可再次选择。' ;;
                        public) open "$PUBLIC_URL/" || print '浏览器打开失败，可再次选择。' ;;
                        qr) open "$QR_PAGE" || print '扫码页打开失败，可再次选择。' ;;
                        retry) attempt_public_access retry || return $? ;;
                        diag) print "穿透诊断：$RUN_DIR/tunnel.log"; [[ ! -f "$RUN_DIR/tunnel.log" ]] || open "$RUN_DIR/tunnel.log" || print '诊断文件未能打开，日志仍保留。' ;;
                        stop) stop_backend || return $?; return 0 ;;
                        pause) paused=1 ;;
                        *) paused=1 ;;
                    esac
                fi
            fi
        elif [[ -t 0 && $paused == 0 ]]; then start_im_browser_menu
        elif [[ -t 0 ]] && IFS= read -r -t 0.1 input; then paused=0
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

# 展示前台终端生命周期。
terminal_notice() {
    if [[ -t 1 && -z "${NO_COLOR:-}" && "${TERM:-dumb}" != dumb ]]; then printf '\033[1;31m%s\033[0m\n' '关闭此运行终端，服务将结束'; else print '关闭此运行终端，服务将结束'; fi
}
# 展示变更范围并确认，显式自动化授权除外。
confirm_action() {
    print "JobsIM MacOS · $ACTION"
    print "实例：$STATE"
    print "HTTPS=${HTTPS_PORT}；业务PG=${MAIN_PORT}；独立journalPG=${JOURNAL_PORT}；内部角色=${ROLE_PORT}～$ROLE_LAST"
    print '只操作此收据匹配实例；不改系统CA或全局工具。反部署清理还需独立选择和最终回车确认；stop/backup保留数据。'
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
    origin_begin
    local relative
    for relative in Processes Logs Secrets TLS Application Bin Sockets ToolEnvironment; do origin_directory "$relative"; done
    mkdir -p "$STATE/Sockets/Main" "$STATE/Sockets/Journal"
    if [[ ! -f "$STATE/owner" ]]; then printf '%s\n%s\n' "$REPO" "$(id -u)" > "$STATE/owner"; origin_record created owner; fi
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
# 按所在卷真实可用字节保留4GiB，不照抄未经压测的机器预算。
require_host_space() {
    local additional="${1:-0}" probe="${2:-$STATE}" available
    [[ "$additional" == <-> ]] || return 2
    while [[ ! -d "$probe" && "$probe" != / ]]; do probe="${probe:h}"; done
    available="$(/bin/df -Pk "$probe" | awk 'END{print $4}')"
    [[ "$available" == <-> ]] || { print -u2 '无法读取实际磁盘余量，停止重型步骤。'; return 2; }
    HOST_ALLOWANCE=$((available*1024-(4*1024*1024*1024)))
    (( HOST_ALLOWANCE >= additional )) || { print -u2 "可用字节不足以保留4GiB及本步预算$additional，保留数据且不清缓存。"; return 2; }
    print "磁盘门禁：实际可用=$((available*1024))；保留=4294967296；本步字节预算=$additional"
}
# 僵尸态已释放句柄和监听端口；不能仅用kill -0误判为仍运行。
process_live() {
    local pid="$1" state
    [[ "$pid" == <-> ]] && kill -0 "$pid" 2>/dev/null || return 1
    state="$(LC_ALL=C ps -p "$pid" -o stat= 2>/dev/null || true)"
    [[ -n "$state" && "${state#${state%%[![:space:]]*}}" != Z* ]]
}
# 原生摘要工具避免对每个数据库文件启动解释器，快照仍逐字节核SHA256。
resource_digest() {
    "${OPENSSL:-/usr/bin/openssl}" dgst -sha256 -r "$1" | cut -d' ' -f1
}
# 核验后端PID、启动时刻与原可执行路径。
process_matches() {
    [[ -f "$STATE/Processes/server.pid" && -f "$STATE/Processes/server.start" ]] || return 1
    local pid="$(cat "$STATE/Processes/server.pid")" start command
    [[ "$pid" == <-> ]] || return 1
    [[ "$(ps -p "$pid" -o uid= 2>/dev/null | tr -d ' ')" == "$(id -u)" ]] || return 1
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
        while process_live "$pid" && (( count < 70 )); do sleep 0.1; (( count+=1 )); done
        if process_live "$pid"; then print -u2 '服务未按时退出，保留数据和进程，拒绝强杀。'; return 2; fi
    elif [[ -f "$STATE/Processes/server.pid" ]]; then
        local pid="$(cat "$STATE/Processes/server.pid")"
        if [[ "$pid" == <-> ]] && process_live "$pid"; then print -u2 'PID存在但启动身份不匹配，不停止陌生进程。'; return 2; fi
    fi
    if lsof -nP -iTCP:"$HTTPS_PORT" -sTCP:LISTEN >/dev/null 2>&1; then print -u2 '应用端口仍有监听，未证明应用退出；保留broker与数据库。'; return 2; fi
}
# 校验本入口NATS的PID、启动时刻、原工具、配置及摘要；不按名称或端口认领。
nats_process_matches() {
    local name
    for name in pid start binary binary.sha256 config.sha256; do [[ -f "$STATE/Processes/nats.$name" && ! -L "$STATE/Processes/nats.$name" ]] || return 1; done
    local broker_pid="$(cat "$STATE/Processes/nats.pid")" binary="$(cat "$STATE/Processes/nats.binary")" started command
    [[ "$broker_pid" =~ '^[1-9][0-9]*$' && -f "$binary" && ! -L "$binary" && -f "$NATS_CONFIG" && ! -L "$NATS_CONFIG" ]] || return 1
    [[ "$(ps -p "$broker_pid" -o uid= 2>/dev/null | tr -d ' ')" == "$(id -u)" ]] || return 1
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
        while process_live "$broker_pid" && (( count < 100 )); do sleep 0.1; (( count+=1 )); done
        if process_live "$broker_pid"; then print -u2 '本实例NATS未退出；保留JetStream及进程，不强杀。'; return 2; fi
    elif [[ -f "$STATE/Processes/nats.pid" ]]; then
        local broker_pid="$(cat "$STATE/Processes/nats.pid")"
        if [[ "$broker_pid" =~ '^[1-9][0-9]*$' ]] && process_live "$broker_pid"; then print -u2 'NATS PID存在但身份/配置不匹配，不停止。'; return 2; fi
    fi
    if lsof -nP -iTCP:"$NATS_PORT" -sTCP:LISTEN >/dev/null 2>&1; then print -u2 'broker端口仍有监听，保留其下游资源。'; return 2; fi
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
        [[ "$original_pid" =~ '^[1-9][0-9]*$' ]] && ! process_live "$original_pid" || { print -u2 'NATS原PID未证实退出，拒绝冷态归档。'; return 2; }
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
        "$OPENSSL" rand -hex 32 > "$STATE/Secrets/operator.password" || return $?
    fi
    for name in Main Journal; do
        data="$STATE/${name}Database"; port="$MAIN_PORT"; [[ "$name" == Main ]] || port="$JOURNAL_PORT"
        if [[ ! -f "$data/PG_VERSION" ]]; then
            origin_directory "${name}Database" || return $?
            "$PG_BIN/initdb" -D "$data" -U jobsim_operator --pwfile="$STATE/Secrets/operator.password" --auth-local=trust --auth-host=scram-sha-256 --encoding=UTF8 --locale=C > "$STATE/Logs/init-$name.log" || return $?
            cluster_identity "$data" > "$STATE/Processes/$name.cluster" || return $?
        fi
        check_cluster "$name" "$data" || return $?
        if ! "$PG_BIN/pg_ctl" -D "$data" status >/dev/null 2>&1; then
            # 客户端均走回环TCP；禁用Unix socket以避开MacOS的路径字节上限。
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
        export "JOBS_IM_${role:u}_DATABASE_URL=postgresql://jobsim_$role:$password@127.0.0.1:$port/jobsim_$role?sslmode=disable"
    done
    JOBS_IM_DATABASE_URL="$JOBS_IM_IDENTITY_DATABASE_URL"; export JOBS_IM_DATABASE_URL
    # Governance schema stays in the identity database covered by identity.dump.
    JOBS_IM_GOVERNANCE_DATABASE_URL="$JOBS_IM_IDENTITY_DATABASE_URL"; export JOBS_IM_GOVERNANCE_DATABASE_URL
}
# 准备实例专用TLS材料，不覆盖既有信任根。
prepare_tls() {
    if [[ "$DEPLOY_MODE" == server ]]; then prepare_server_tls; return $?; fi
    if [[ -f "$STATE/TLS/server.crt" && -f "$STATE/TLS/server.key" && -f "$STATE/TLS/ca.crt" ]]; then
        "$OPENSSL" verify -x509_strict -purpose sslserver -CAfile "$STATE/TLS/ca.crt" "$STATE/TLS/server.crt" >/dev/null || { print -u2 '现有TLS证书未通过严格验证；保留CA、指纹与原文件，需区分CA与服务端证书：CA有效才可原CA重签，CA缺陷需受控信任迁移。'; return 2; }
        "$OPENSSL" x509 -in "$STATE/TLS/server.crt" -checkend 86400 -noout >/dev/null || { print -u2 '证书将到期；需受控轮换，当前入口不重置信任根。'; return 2; }
        return
    fi
    [[ -z "$(ls -A "$STATE/TLS")" ]] || { print -u2 'TLS材料不完整或存在未知内容，保留全部原件并停止。'; return 2; }
    local tls_stage="$(mktemp -d "$STATE/.TLS-pending.XXXXXX")"
    [[ -n "$tls_stage" && -d "$tls_stage" ]] || return 2
    origin_record created "${tls_stage:t}"
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
    origin_forget_removed "${tls_stage:t}"
    origin_record created TLS
    [[ -f "$STATE/TLS/server.crt" && ! -e "$tls_stage" ]] || { print -u2 'TLS提交发生并发变化，保留材料并停止。'; return 2; }
}
# 构建当前源码，失败不替换为假成功。
build_source() {
    require_host_space 0 "$STATE" || return $?
    print '构建Go角色与WebDist；依赖缓存留在用户级，未安装全局工具。'
    (cd "$REPO/代码/后端/Go" && GOTOOLCHAIN=local "$GO_TOOL" build -trimpath -o "$STATE/Bin/JobsIMd.new" ./cmd/jobsimd)
    mv -- "$STATE/Bin/JobsIMd.new" "$STATE/Bin/JobsIMd"
    (cd "$REPO/代码/前端/Web" && npm run build)
    # 运行资产放实例持久目录，源码再次移动也不影响已经启动的首页。
    WEB_RUNTIME="$(mktemp -d "$STATE/WebDist.XXXXXX")"
    origin_record created "${WEB_RUNTIME:t}"
    cp -R -- "$REPO/代码/前端/Web/Dist/." "$WEB_RUNTIME/"
    [[ -f "$WEB_RUNTIME/index.html" && ! -L "$WEB_RUNTIME/index.html" ]] || { print -u2 'Web运行资产不完整，保留本轮副本并拒绝启动。'; return 2; }
    [[ -f "$REPO/代码/前端/Web/Dist/index.html" ]] || { print -u2 'WebDist缺失。'; return 2; }
}
# 应用与broker停止后生成范围明确的加密归档。
backup_instance() {
    stop_backend || return $?
    stop_tunnel || return $?
    stop_nats || return $?
    prepare_clusters || return $?
    local bytes=0 directory value
    for directory in MainDatabase JournalDatabase Application Secrets TLS; do
        [[ ! -d "$STATE/$directory" ]] || { value="$(/usr/bin/du -sk "$STATE/$directory" | awk '{print $1}')"; [[ "$value" == <-> ]] || return 2; (( bytes+=value*1024 )); }
    done
    require_host_space "$((bytes*2))" "$BACKUPS" || return $?
    mkdir -p "$BACKUPS" "$RECOVERY" || return $?
    chmod 700 "$BACKUPS" "$RECOVERY" || return $?
    if [[ ! -f "$RECOVERY/key.pem" ]]; then
        [[ ! -e "$RECOVERY/cert.pem" ]] || { print -u2 '恢复密钥材料不完整。'; return 2; }
        "$OPENSSL" req -x509 -newkey rsa:3072 -nodes -days 3650 -keyout "$RECOVERY/key.pem" -out "$RECOVERY/cert.pem" -subj '/CN=JobsIM Backup Recovery' > "$STATE/Logs/backup-key.log" 2>&1 || return $?
    fi
    local staging="$BACKUPS/.staging-$(date +%Y%m%d-%H%M%S)-$$" file="$BACKUPS/JobsIM-$(date +%Y%m%d-%H%M%S)-$$.cms"
    mkdir "$staging" || return $?
    write_nats_backup_manifest "$staging/broker-manifest.txt" || return $?
    local role port
    for role in "${ROLES[@]}" journal; do
        port="$MAIN_PORT"; [[ "$role" != journal ]] || port="$JOURNAL_PORT"
        PGPASSWORD="$(cat "$STATE/Secrets/operator.password")" "$PG_BIN/pg_dump" -h 127.0.0.1 -p "$port" -U jobsim_operator -d "jobsim_$role" --format=custom --file="$staging/$role.dump" || return $?
        "$PG_BIN/pg_restore" --list "$staging/$role.dump" > "$staging/$role.list" || return $?
    done
    tar -czf - -C "$STATE" owner Application TLS Secrets -C "$staging" . | "$OPENSSL" cms -encrypt -aes-256-gcm -binary -outform DER -out "$file" "$RECOVERY/cert.pem" || return $?
    "$OPENSSL" cms -decrypt -binary -inform DER -in "$file" -inkey "$RECOVERY/key.pem" | tar -tzf - > "$file.manifest" || return $?
    grep -Eq '(^|/)broker-manifest.txt$' "$file.manifest" || { print -u2 '解密归档缺broker范围清单，不得清理源数据。'; return 2; }
    for role in "${ROLES[@]}" journal; do grep -Eq "(^|/)$role.dump$" "$file.manifest" || return 2; done
    shasum -a 256 "$file" > "$file.sha256" || return $?
    for role in "${ROLES[@]}" journal; do rm -- "$staging/$role.dump" "$staging/$role.list" || return $?; done
    rm -- "$staging/broker-manifest.txt" || return $?
    rmdir "$staging" || return $?
    LAST_BACKUP_FILE="$file"
    print "已生成并解密验真：$file"
    print "恢复私钥独立保存于：$RECOVERY/key.pem；请另行离线保管。备份不包含这把解密私钥。"
    print '归档含全部域dump、已停JetStream及配置/token；不自动回滚独立journal，归档验真不等于数据库恢复演练。'
}
# 仅展示无秘密连接身份及CA指纹。
write_connection() {
    local url="${PUBLIC_URL:-${JOBS_IM_PUBLIC_URL:-https://$PUBLIC_HOST:$HTTPS_PORT}}" response deployment fingerprint
    response="$(local_api_json /api/bootstrap)"
    deployment="$(print -r -- "$response" | sed -n 's/.*"deploymentId":"\([^"]*\)".*/\1/p')"
    [[ -n "$deployment" ]] || { print -u2 '公开实例身份缺失。'; return 2; }
    fingerprint="$("$OPENSSL" x509 -in "$STATE/TLS/ca.crt" -noout -fingerprint -sha256 | cut -d= -f2)"
    origin_bind_deployment "$deployment"
    write_instance_reference "$deployment"
    if [[ -n "$PUBLIC_URL" ]]; then
        printf '{"deploymentId":"%s","name":"JobsIM","url":"%s","protocolVersion":"1","tlsTrust":"system","originCaSha256":"%s"}\n' "$deployment" "$url" "$fingerprint" > "$STATE/connection.json"
    else
        printf '{"deploymentId":"%s","name":"JobsIM","url":"%s","protocolVersion":"1","caSha256":"%s"}\n' "$deployment" "$url" "$fingerprint" > "$STATE/connection.json"
    fi
    origin_record created connection.json
    print '用户端：'; show_endpoint "$url"
    print '管理网页尚未交付，只有上述JobsIM用户入口，不生成后台URL。'
    print "deploymentId：$deployment"
    print "CA SHA256：$fingerprint"
    print "公开连接描述：$STATE/connection.json"
    print 'Web须按系统/浏览器受控信任本CA或另用公信HTTPS；未信任时阻断，不能绕过证书警告。脚本不自动安装全局CA。'
    if false && command -v qrencode >/dev/null; then qrencode -t UTF8 "$url" || print '终端二维码失败，使用上述URL。'; qrencode -o "$STATE/connection.png" "$url" || print 'PNG二维码失败，使用上述URL。'; else print '离线二维码由本项目锁定的本地编码器生成；见本轮扫码页。'; fi
}
# 固定原生demo适配；远端控制器不能传路径、端口或任意命令。
validate_demo_adapter() {
    [[ "${JOBS_IM_DEMO_ADAPTER:-}" == 1 && "$STATE" == "$HOME/Library/Application Support/JobsIM/Demo/NativeDemo-v1/Instance" ]] || { print -u2 'demo仅允许本机固定专属实例。'; return 2; }
    [[ "$HTTPS_PORT/$MAIN_PORT/$JOURNAL_PORT/$NATS_PORT/$ROLE_PORT" == '19543/19545/19546/19547/19560' && "$ROLE_LAST" == 19568 ]] || return 2
    local address="${JOBS_IM_DEMO_BIND_IP:-}" octet
    [[ "$address" =~ '^([0-9]{1,3}\.){3}[0-9]{1,3}$' ]] || return 2
    for octet in ${(s:.:)address}; do (( octet >= 0 && octet <= 255 )) || return 2; done
    [[ "$address" == 10.* || "$address" == 192.168.* || "$address" =~ '^172\.(1[6-9]|2[0-9]|3[01])\.' ]] || { print -u2 'demo只绑定本机私有LAN IPv4。'; return 2; }
    [[ "$PUBLIC_HOST" == "$address" && "$DEPLOY_MODE" == server ]] || return 2
}
# Mac helper先准备当前产品；此动作不启动PG/NATS/业务服务。
prepare_demo_products() {
    validate_demo_adapter || return $?
    confirm_action || return $?
    require_host_space 0 "$STATE" || return $?
    find_tools || return $?
    prepare_state || return $?
    acquire_lock || return $?
    apply_owner_relocation || return $?
    prepare_tls || return $?
    build_source || return $?
    printf '%s\n' "${WEB_RUNTIME:t}" > "$STATE/demo-web-runtime"
    origin_record created demo-web-runtime
    release_lock
    print '固定demo产品已准备；尚未启动业务服务。'
}
# ensure-start只走此固定无菜单动作，退出仍执行原精确保留数据清理链。
run_demo_instance() {
    validate_demo_adapter || return $?
    find_tools || return $?
    assert_owner || return $?
    [[ -f "$STATE/demo-web-runtime" && ! -L "$STATE/demo-web-runtime" && -x "$STATE/Bin/JobsIMd" ]] || { print -u2 '先在Mac准备固定demo产品。'; return 2; }
    local web_leaf="$(cat "$STATE/demo-web-runtime")"
    [[ "$web_leaf" =~ '^WebDist\.[A-Za-z0-9]+$' ]] || return 2
    WEB_RUNTIME="$STATE/$web_leaf"
    [[ -f "$WEB_RUNTIME/index.html" && ! -L "$WEB_RUNTIME" && ! -L "$WEB_RUNTIME/index.html" ]] || return 2
    prepare_state || return $?
    acquire_lock || return $?
    stop_backend || return $?
    stop_nats || return $?
    stop_clusters || return $?
    check_ports || return $?
    prepare_clusters || return $?
    prepare_databases || return $?
    prepare_tls || return $?
    start_nats || return $?
    launch_im_backend "$JOBS_IM_PUBLIC_URL" || return $?
    write_connection || return $?
    release_lock
    print '固定demo实例已通过严格TLS健康；等待本机controller生命周期。'
    while process_matches && nats_process_matches; do sleep 1; done
    print -u2 '固定demo服务已退出；保留资料并按原归属收尾。'
    return 2
}
# 先核对旧实例与备份，再启动本轮依赖和应用。
run_instance() {
    confirm_action
    select_im_mode
    require_host_space 0 "$STATE"
    prepare_im_tools
    prepare_state; acquire_lock; apply_owner_relocation
    if [[ -f "$STATE/Application/instance.json" ]]; then backup_instance; stop_clusters; fi
    stop_backend; stop_nats; stop_clusters; check_ports
    prepare_clusters; prepare_databases; prepare_tls; build_source
    start_nats
    local configured_url="${JOBS_IM_PUBLIC_URL:-https://$PUBLIC_HOST:$HTTPS_PORT}"
    [[ "$configured_url" == https://* && "$configured_url" != *'?'* && "$configured_url" != *'#'* ]] || { print -u2 '服务器公开URL必须为明确HTTPS origin。'; return 2; }
    launch_im_backend "$configured_url"
    if [[ "$DEPLOY_MODE" == local && "$LOCAL_ACCESS" == quick ]]; then attempt_public_access initial || return $?; fi
    printf 'project=%s\nuid=%s\npg_main=%s\npg_journal=%s\nhttps=%s\nrole_base=%s\n' "$REPO" "$(id -u)" "$MAIN_PORT" "$JOURNAL_PORT" "$HTTPS_PORT" "$ROLE_PORT" > "$STATE/receipt.txt"
    origin_record created receipt.txt
    write_connection
    release_lock
    terminal_notice
    prepare_im_qr
    start_session_guardian
    monitor_im_session
}
# 只读发现工具、归属和状态，不初始化资源。
read_only() {
    print "动作=${ACTION}；不会创建state、启动/停止服务、生成密钥或安装环境。"
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
            if stop_backend && stop_tunnel && stop_nats; then stop_clusters; else print -u2 '应用或broker未证实停止，保留其下游资源。'; fi
        fi
    fi
    RUN_ACTIVE=0
    stop_browser_menu
    expire_im_qr
    cleanup_dependency_commands
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
    print 'JobsIM部署/反部署：本地／服务器双模式，本地默认仅本机；undeploy仅按原点清理，独立journal/密钥默认保留；stop/backup为运维子动作。'
}
# 按已经解析的模式编排业务，未知归属始终保留。
dispatch_action() {
    case "$ACTION" in
        check|plan|status) read_only ;;
        run) assert_owner; run_instance ;;
        demo-prepare) prepare_demo_products ;;
        demo-run) run_demo_instance ;;
        undeploy) undeploy_instance ;;
        stop|backup)
            find_tools; assert_owner; [[ -f "$STATE/owner" ]] || { print '本实例尚未创建；没有停止或清理任何资源。'; return 0; }
            confirm_action; prepare_state; acquire_lock; apply_owner_relocation
            RUN_ACTIVE=1
            if [[ "$ACTION" == backup || "$NO_BACKUP" == 0 ]]; then backup_instance; else stop_backend; fi
            stop_nats; stop_clusters; RUN_ACTIVE=0; release_lock
            print '本实例已停止；数据库、对象、CA/私钥、独立journal与既有备份均保留。'
            ;;
        help) print '用法：部署／反部署JobsIM.command check|plan|status|run|undeploy|backup|stop [--state 绝对目录] [--yes] [--no-backup]；undeploy经fzf选择并独立回车确认清理；stop/backup保留数据。' ;;
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
