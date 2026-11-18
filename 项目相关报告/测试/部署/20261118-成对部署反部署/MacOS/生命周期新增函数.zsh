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
    result="$(printf '%s\n' "$@" | FZF_DEFAULT_OPTS='' FZF_DEFAULT_OPTS_FILE=/dev/null FZF_DEFAULT_COMMAND='' "$FZF_EXECUTABLE" --no-sort --no-multi --height=40% --border --delimiter=$'\t' --with-nth=2 --bind='up:up,down:down,tab:down,btab:up' --prompt="$prompt > " --header="$header")" || code=$?
    (( code == 0 )) || { (( code == 1 || code == 130 )) && return 130; return "$code"; }
    [[ -n "$result" ]] || return 130
    SELECTED_OPERATION="${result%%$'\t'*}"
}
# 最终回车只确认已显示的确切范围；yes、非空输入和EOF均不替代此确认。
confirm_cleanup() {
    print "将清理原点中本实例新建的程序、业务数据库和运行资料：$STATE"
    print '保留：独立JournalDatabase、Secrets、TLS、归属审计、共享工具链、源码、外置备份和恢复钥。'
    print "数据方式：$DATA_CHOICE；未知、复用或并发变化对象均保留并报告未完成。"
    local answer
    printf '按回车执行以上清理；输入任意文字或输入结束取消：'
    IFS= read -r answer || return 130
    [[ -z "$answer" ]] || return 130
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
            hash=-; size=-; kind=d
            [[ ! -L "$entry" && -O "$entry" ]] || return 2
            if [[ -f "$entry" ]]; then kind=f; size="$(stat -f %z "$entry")"; hash="$(shasum -a 256 "$entry" | cut -d' ' -f1)"; fi
            mode="$(stat -f %Lp "$entry")"
            printf '%s\t%s\t%s\t%s\t%s\t%s\n' "${entry#$STATE/}" "$kind" "$(resource_identity "$entry")" "$mode" "$size" "$hash" >> "$work/plan.tsv"
        done < "$path_list"
        rm -- "$path_list"
    done < "$STATE/.Deployment/resources.tsv"
    printf '%s\n' "$DATA_CHOICE" > "$work/data-choice"
    print prepared > "$work/phase"
}
# 重查快照所有对象及目录全集；已删除项只在持久deleting阶段允许缺失。
undeploy_verify_plan() {
    origin_verify_tree || return $?
    local work="$STATE/.Deployment/undeploy" relative kind identity mode size hash entry phase disposition extra list child
    phase="$(cat "$work/phase")"
    [[ "$phase" == prepared || "$phase" == deleting ]] || return 2
    while IFS=$'\t' read -r relative kind identity mode size hash; do
        [[ "$relative" != /* && "$relative" != ../* && "$relative" != *'/../'* && "$relative" != .Deployment* && "$relative" != Lock* ]] || return 2
        entry="$STATE/$relative"
        if [[ ! -e "$entry" && ! -L "$entry" && "$phase" == deleting ]]; then continue; fi
        [[ -e "$entry" && ! -L "$entry" && -O "$entry" && "$(resource_identity "$entry")" == "$identity" && "$(stat -f %Lp "$entry")" == "$mode" ]] || { print -u2 "确认快照已变化，保留：$relative"; return 2; }
        if [[ "$kind" == f ]]; then
            [[ -f "$entry" && "$(stat -f %z "$entry")" == "$size" && "$(shasum -a 256 "$entry" | cut -d' ' -f1)" == "$hash" ]] || { print -u2 "确认后文件内容变化，保留：$relative"; return 2; }
        else [[ "$kind" == d && -d "$entry" ]] || return 2; fi
    done < "$work/plan.tsv"
    cut -f1 "$work/plan.tsv" > "$work/planned-paths"
    while IFS=$'\t' read -r disposition relative kind identity extra; do
        [[ "$disposition" == created && -e "$STATE/$relative" ]] || continue
        resource_protected "$relative" && continue
        list="$(mktemp "$work/.scan.XXXXXX")"
        /usr/bin/find -P "$STATE/$relative" -print0 > "$list" || return $?
        while IFS= read -r -d '' child; do
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
            [[ "$(shasum -a 256 "$entry" | cut -d' ' -f1)" == "$hash" ]] || return 2
            /bin/rm -- "$entry"
        else /bin/rmdir -- "$entry"; fi
    done < "$work/plan.tsv"
    remove_instance_reference
    print complete-protected-retained > "$work/phase"
    print '反部署清理已完成所列可清资源；独立journal、匹配密钥及原点/复用资料仍保留，整体结果为partial。'
}
# 只移除与本次原点部署身份相同的本机引用。
remove_instance_reference() {
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
    pick_operation '反部署' '默认仅停止并保留；Esc取消，不删除数据。' $'retain\t仅停止并保留全部资料' $'cleanup\t进入反部署清理' || return $?
    case "$SELECTED_OPERATION" in
        retain)
            acquire_lock; RUN_ACTIVE=1
            stop_backend; stop_tunnel; stop_nats; stop_clusters
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
        undeploy_verify_plan; confirm_cleanup; undeploy_delete_plan
        release_lock
        return 0
    fi
    [[ ! -e "$work" ]] || { print -u2 '存在未封存清理准备目录，保留并停止；不重新初始化。'; return 2; }
    pick_operation '数据处理' '默认备份后清理；清零不可恢复；Esc取消。' $'backup\t数据库数据备份（默认，验真后清理）' $'clear\t数据库数据清零（保留独立journal与匹配钥）' || return $?
    DATA_CHOICE="$SELECTED_OPERATION"
    [[ "$DATA_CHOICE" == backup || "$DATA_CHOICE" == clear ]] || return 2
    acquire_lock; RUN_ACTIVE=1
    stop_backend; stop_tunnel; stop_nats
    if [[ "$DATA_CHOICE" == backup ]]; then backup_instance; fi
    stop_clusters; RUN_ACTIVE=0
    origin_verify_tree
    undeploy_snapshot
    confirm_cleanup || { release_lock; return 130; }
    undeploy_delete_plan
    release_lock
}
