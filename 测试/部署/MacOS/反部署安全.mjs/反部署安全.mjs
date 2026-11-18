#!/usr/bin/env node
// Created by Jobs on 2026年11月18日，星期三.
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import crypto from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../../..');
const report = path.resolve(process.env.JOBSIM_UNDEPLOY_REPORT ?? path.join(root, '项目相关报告/测试/部署/20261118-成对部署反部署/MacOS/安全夹具-' + Date.now()));
fs.mkdirSync(report, { recursive: true });
const base = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'JobsIM-反部署 空格.')));
const files = ['Deployment/【MacOS】部署JobsIM.command/【MacOS】部署JobsIM.command', 'Undeployment/【MacOS】反部署JobsIM.command/【MacOS】反部署JobsIM.command'].map(p => path.join(root, '部署/MacOS', p));
const results = [];
const digest = p => crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex');
const before = files.map(p => ({ path: path.relative(root, p), sha256: digest(p) }));
const setup = String.raw`
setopt NO_NOMATCH PIPE_FAIL
set -eu
umask 077
source "$1"
REPO="$2/仓库副本"; STATE="$2/实例"; mkdir -p "$REPO"
RUN_ACTIVE=0; LOCK_HELD=0; CLEANING=1; RELOCATE_OWNER=0; REF_ID=''; REF_DEPLOYMENT=''; DEFAULT_INSTANCE_STATE=''; RUN_DIR=''; LOG_FILE=/dev/null; YES=1; ACTION=undeploy; DATA_CHOICE=clear
MAIN_PORT=17543; JOURNAL_PORT=17544; HTTPS_PORT=19443; ROLE_PORT=20101; ROLE_LAST=20109; NATS_PORT=20422; NATS_MAX_FILE=536870912; NATS_MAX_MEMORY=33554432
REF_DIR="$REPO/.jobsim-local"; REF_FILE="$REF_DIR/instance.ref"; REPO_ID=1111111111111111
prepare_state
origin_directory MainDatabase; origin_directory JournalDatabase
print 'synthetic-business' > "$STATE/MainDatabase/record"
print 'synthetic-journal' > "$STATE/JournalDatabase/record"
print 'synthetic-owned' > "$STATE/Application/record"
print '{"deploymentId":"11111111-1111-4111-8111-111111111111","key":"synthetic-matching-key"}' > "$STATE/Application/instance.json"
print 'synthetic-key' > "$STATE/Secrets/record"
print 'synthetic-ca' > "$STATE/TLS/record"
origin_bind_deployment 11111111-1111-4111-8111-111111111111
`;
const cases = [
  ['原点与新建资源', 'origin_verify_tree; [[ -f "$STATE/.Deployment/origin" ]]'],
  ['默认保护独立journal和密钥', 'origin_verify_tree; undeploy_snapshot; undeploy_delete_plan; [[ ! -e "$STATE/MainDatabase" && ! -e "$STATE/Application/record" && -f "$STATE/Application/instance.json" && -f "$STATE/JournalDatabase/record" && -f "$STATE/Secrets/record" && -f "$STATE/TLS/record" ]]'],
  ['未知顶层拒绝', 'print keep > "$STATE/unknown"; if origin_verify_tree; then exit 22; fi; [[ -f "$STATE/unknown" && -f "$STATE/Application/record" ]]'],
  ['嵌套链接拒绝且目标保留', 'print keep > "$2/external"; ln -s "$2/external" "$STATE/Application/link"; if origin_verify_tree; then exit 23; fi; [[ "$(cat "$2/external")" == keep ]]'],
  ['目录对象替换拒绝', 'origin_verify_tree; undeploy_snapshot; mv "$STATE/Bin" "$2/old-bin"; mkdir "$STATE/Bin"; if undeploy_verify_plan; then exit 24; fi; [[ -d "$STATE/Bin" ]]'],
  ['确认后正文变化拒绝', 'origin_verify_tree; undeploy_snapshot; print changed >> "$STATE/Application/record"; if undeploy_verify_plan; then exit 25; fi; [[ -f "$STATE/MainDatabase/record" ]]'],
  ['确认后新增子项拒绝', 'origin_verify_tree; undeploy_snapshot; print keep > "$STATE/Application/new-file"; if undeploy_verify_plan; then exit 26; fi; [[ -f "$STATE/Application/new-file" ]]'],
  ['中断断点续删不重初始化', 'origin_verify_tree; undeploy_snapshot; first="$(head -1 "$STATE/.Deployment/undeploy/plan.tsv" | cut -f1)"; print deleting > "$STATE/.Deployment/undeploy/phase"; if [[ -d "$STATE/$first" ]]; then rmdir "$STATE/$first"; else rm "$STATE/$first"; fi; undeploy_delete_plan; [[ -f "$STATE/JournalDatabase/record" && ! -e "$STATE/MainDatabase" ]]'],
  ['仅移除匹配本机reference', 'DEFAULT_INSTANCE_STATE="$STATE"; mkdir -p "$REF_DIR"; printf "JobsIM-instance-ref-v1\\n%s\\n%s\\n%s\\n" "$REPO_ID" "11111111-1111-4111-8111-111111111111" "$(id -u)" > "$REF_FILE"; origin_verify_tree; undeploy_snapshot; undeploy_delete_plan; [[ ! -e "$REF_FILE" ]]'],
  ['错误reference保留', 'DEFAULT_INSTANCE_STATE="$STATE"; mkdir -p "$REF_DIR"; printf "JobsIM-instance-ref-v1\\n%s\\nwrong\\n%s\\n" "$REPO_ID" "$(id -u)" > "$REF_FILE"; if remove_instance_reference; then exit 27; fi; [[ -f "$REF_FILE" ]]'],
  ['显式隔离state不处理默认reference', 'mkdir -p "$REF_DIR"; print unrelated > "$REF_FILE"; remove_instance_reference; [[ "$(cat "$REF_FILE")" == unrelated ]]'],
  ['yes不能旁路EOF清理确认', 'if confirm_cleanup < /dev/null; then exit 28; fi; [[ -f "$STATE/MainDatabase/record" ]]'],
  ['非空输入取消', 'if confirm_cleanup <<< cancel; then exit 29; fi; [[ -f "$STATE/MainDatabase/record" ]]'],
  ['空行确认保留实际选择', 'confirm_cleanup <<< ""; [[ "$DATA_CHOICE" == clear ]]'],
  ['旧缺原点不得追认清理', 'mv "$STATE/.Deployment" "$2/old-ledger"; if origin_load; then exit 30; fi; [[ -f "$STATE/MainDatabase/record" ]]'],
  ['备份失败不进入删除', String.raw`prepare_selector(){ return 0; }; find_tools(){ return 0; }; confirm_action(){ return 0; }; pick_operation(){ if [[ "$1" == 反部署 ]]; then SELECTED_OPERATION=cleanup; else SELECTED_OPERATION=backup; fi; }; stop_backend(){ return 0; }; stop_tunnel(){ return 0; }; stop_nats(){ return 0; }; backup_instance(){ return 31; }; stop_clusters(){ return 0; }; if undeploy_instance <<< ""; then exit 32; fi; [[ -f "$STATE/MainDatabase/record" && ! -e "$STATE/.Deployment/undeploy" ]]`],
  ['条件调用也必须传播broker清单失败', String.raw`stop_backend(){ return 0; }; stop_tunnel(){ return 0; }; stop_nats(){ return 0; }; prepare_clusters(){ return 0; }; require_host_space(){ return 0; }; BACKUPS="$2/backups"; RECOVERY="$2/recovery"; mkdir "$RECOVERY"; print synthetic > "$RECOVERY/key.pem"; print synthetic > "$RECOVERY/cert.pem"; write_nats_backup_manifest(){ return 42; }; OPENSSL=/usr/bin/false; PG_BIN=/usr/bin; ROLES=(identity); if backup_instance; then exit 33; fi; [[ -f "$STATE/MainDatabase/record" && -z "$(find "$BACKUPS" -name '*.cms' -print)" ]]`],
  ['dump失败不生成完整备份', String.raw`stop_backend(){ return 0; }; stop_tunnel(){ return 0; }; stop_nats(){ return 0; }; prepare_clusters(){ return 0; }; require_host_space(){ return 0; }; BACKUPS="$2/backups"; RECOVERY="$2/recovery"; mkdir "$RECOVERY"; print synthetic > "$RECOVERY/key.pem"; print synthetic > "$RECOVERY/cert.pem"; write_nats_backup_manifest(){ print synthetic > "$1"; }; OPENSSL=/usr/bin/false; PG_BIN=/nonexistent-tools; ROLES=(identity); if backup_instance; then exit 34; fi; [[ -f "$STATE/MainDatabase/record" && -z "$(find "$BACKUPS" -name '*.cms' -print)" ]]`],
  ['条件调用不能吞集群身份失败', String.raw`print synthetic > "$STATE/Secrets/operator.password"; print 18 > "$STATE/MainDatabase/PG_VERSION"; PG_BIN=/nonexistent-tools; check_cluster(){ return 42; }; code=0; prepare_clusters || code=$?; [[ "$code" == 42 && -f "$STATE/MainDatabase/record" ]]`],
  ['实际磁盘4GiB门禁及超大预算拒绝', 'require_host_space 0 "$STATE"; if require_host_space 9223372036854775807 "$STATE"; then exit 35; fi; [[ -f "$STATE/MainDatabase/record" ]]'],
  ['本地默认仅本机不准备穿透', String.raw`CLOUDFLARED_EXECUTABLE=''; prepare_selector(){ return 0; }; pick_operation(){ if [[ "$1" == 部署模式 ]]; then [[ "$2" == *'本地默认仅本机；临时穿透需另选并确认'* && "$3" == $'local\t本地部署' ]] || return 43; SELECTED_OPERATION=local; else SELECTED_OPERATION=private; fi; }; select_im_mode; [[ "$DEPLOY_MODE" == local && "$LOCAL_ACCESS" == private && "$JOBS_IM_PUBLIC_URL" == "https://127.0.0.1:$HTTPS_PORT" && -z "$CLOUDFLARED_EXECUTABLE" ]]`],
  ['公开确认EOF不触发穿透', String.raw`prepare_selector(){ return 0; }; pick_operation(){ if [[ "$1" == 部署模式 ]]; then SELECTED_OPERATION=local; else SELECTED_OPERATION=quick; fi; }; if select_im_mode < /dev/null; then exit 36; fi; [[ -f "$STATE/MainDatabase/record" ]]`],
  ['穿透准备失败保持本地', String.raw`DEPLOY_MODE=local; LOCAL_ACCESS=quick; CLOUDFLARED_EXECUTABLE=''; PUBLIC_URL=''; resolve_latest_tool(){ return 75; }; attempt_public_access initial; [[ "$LOCAL_ACCESS" == private && -z "$PUBLIC_URL" && -f "$STATE/MainDatabase/record" ]]`],
  ['穿透启动失败恢复严格本机Origin', String.raw`DEPLOY_MODE=local; LOCAL_ACCESS=quick; CLOUDFLARED_EXECUTABLE=/usr/bin/false; PUBLIC_URL=https://synthetic.trycloudflare.com; TUNNEL_PID=999999999; stop_browser_menu(){ return 0; }; expire_im_qr(){ return 0; }; stop_tunnel(){ return 0; }; stop_backend(){ return 0; }; start_im_tunnel(){ return 7; }; launch_im_backend(){ ACTUAL_ORIGIN="$1"; return 0; }; write_connection(){ [[ -z "$PUBLIC_URL" ]]; }; prepare_im_qr(){ return 0; }; attempt_public_access initial; [[ -z "$PUBLIC_URL" && -z "$TUNNEL_PID" && "$ACTUAL_ORIGIN" == "https://127.0.0.1:$HTTPS_PORT" && -f "$STATE/MainDatabase/record" && -f "$STATE/Application/instance.json" ]]`],
  ['穿透晚退进入恢复路径', String.raw`SERVER_PID=$$; TUNNEL_PID=999999999; PUBLIC_URL=https://synthetic.trycloudflare.com; BROWSER_MENU_PID=''; CALLED=0; recover_private_after_tunnel(){ CALLED=1; PUBLIC_URL=''; TUNNEL_PID=''; SERVER_PID=999999999; }; code=0; monitor_im_session || code=$?; [[ "$CALLED" == 1 && -z "$PUBLIC_URL" && "$code" == 127 && -f "$STATE/Application/instance.json" ]]`],
];

for (const [scriptIndex, script] of files.entries()) {
  const library = path.join(base, 'functions-' + scriptIndex + '.zsh');
  fs.writeFileSync(library, fs.readFileSync(script, 'utf8').replace(/\nmain "\$@"\s*$/, '\n'));
  for (const [index, [name, body]] of cases.entries()) {
    const directory = path.join(base, `${scriptIndex}-${index}`);
    fs.mkdirSync(directory);
    const run = spawnSync('/bin/zsh', ['-c', setup + '\n' + body, 'probe', library, directory], { cwd: '/', encoding: 'utf8', timeout: 15000 });
    const result = { script: path.relative(root, script), name, exitCode: run.status, error: run.error?.code ?? null };
    results.push(result);
    fs.writeFileSync(path.join(report, `${scriptIndex}-${index}.log`), (run.stdout ?? '') + (run.stderr ?? ''));
    process.stdout.write(`${run.status === 0 ? 'PASS' : 'FAIL'} ${scriptIndex} ${name}\n`);
  }
}
const after = files.map(p => ({ path: path.relative(root, p), sha256: digest(p) }));
const summary = { status: results.every(r => r.exitCode === 0) && JSON.stringify(before) === JSON.stringify(after) ? 'passed' : 'failed', passed: results.filter(r => r.exitCode === 0).length, failed: results.filter(r => r.exitCode !== 0).length, inputStable: JSON.stringify(before) === JSON.stringify(after), before, after, results, privateFixture: base, liveInstanceTouched: false };
fs.writeFileSync(path.join(report, 'summary.json'), JSON.stringify(summary, null, 2));
process.exitCode = summary.status === 'passed' ? 0 : 1;
