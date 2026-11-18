// Created by Jobs on 2026年11月18日，星期三.
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import crypto from 'node:crypto';
import {fileURLToPath} from 'node:url';
import {spawnSync} from 'node:child_process';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../../../..');
const report=process.env.JOBSIM_LINUX_REPORT||path.join(root,'项目相关报告/测试/部署/20261118-成对部署反部署/Linux','Run-'+Date.now());fs.mkdirSync(report,{recursive:true});
const work=fs.mkdtempSync(path.join(fs.realpathSync(os.tmpdir()),'JobsIM-Linux-')),scripts=['Ubuntu','Debian','RockyLinux','AlibabaCloudLinux'].flatMap(p=>['Deployment','Undeployment'].map(b=>{const n=b==='Deployment'?'【Linux】部署JobsIM.sh':'【Linux】反部署JobsIM.sh';return path.join(root,'部署',p,b,n,n)}));
const hashes=()=>Object.fromEntries(scripts.map(f=>[path.relative(root,f),crypto.createHash('sha256').update(fs.readFileSync(f)).digest('hex')])),before=hashes(),results=[];
const runnerHash=crypto.createHash('sha256').update(fs.readFileSync(fileURLToPath(import.meta.url))).digest('hex');
const filter=process.env.JOBSIM_LINUX_CASE_FILTER?new RegExp(process.env.JOBSIM_LINUX_CASE_FILTER):null;
function run(name,args,expected=0,input=''){if(filter&&!filter.test(name))return;const r=spawnSync('/bin/bash',args,{cwd:work,encoding:'utf8',timeout:30000,input});const passed=r.status===expected;results.push({name,passed,expected,exitCode:r.status,signal:r.signal,error:r.error?.message,output:(r.stdout||'')+(r.stderr||'')});console.log((passed?'PASS ':'FAIL ')+name)}
for(const f of scripts){run(path.relative(root,f)+':syntax',['-n',f]);run(path.relative(root,f)+':help',[f,'--help']);if(process.platform!=='linux')run(path.relative(root,f)+':host-refused',[f,'check'],2)}
const source=process.env.JOBSIM_LINUX_LIBRARY||scripts[0],first=fs.readFileSync(source,'utf8'),normalize=s=>s.replace(/^DEFAULT_ACTION=.*$/m,'DEFAULT_ACTION=normalized').replace(/^DISTRO=.*$/m,'DISTRO=normalized');if(!filter)results.push({name:'eight native implementations agree except distro/default action',passed:scripts.every(f=>normalize(fs.readFileSync(f,'utf8'))===normalize(first))});
// Decode the Desktop Entry string and Exec quoting separately, then execute only fake packages.
function desktopArguments(file) {
  const value=fs.readFileSync(file,'utf8').split('\n').find(line=>line.startsWith('Exec=')).slice(5)
    .replace(/\\([sntr\\])/g,(_,character)=>({s:' ',n:'\n',t:'\t',r:'\r','\\':'\\'}[character]));
  const args=[];let quoted=false,word='';
  for(let index=0;index<value.length;index++){
    const character=value[index];
    if(character==='\\' && (!quoted || ['"','`','$','\\'].includes(value[index+1]))){word+=value[++index];continue;}
    if(character==='"'){quoted=!quoted;continue;}
    if(/\s/.test(character)&&!quoted){if(word){args.push(word);word='';}}else word+=character;
  }
  if(quoted)throw Error('Unclosed desktop Exec quote');if(word)args.push(word);return args;
}
for(const platform of ['Ubuntu','Debian','RockyLinux','AlibabaCloudLinux'])for(const type of ['部署','反部署']){
  const original=path.join(root,'部署',platform,`双击${type}（快捷入口）.desktop`),args=desktopArguments(original);
  const fixture=path.join(work,`${platform}-${type} 中文 空格 ' $(literal)`),bucket=type==='部署'?'Deployment':'Undeployment';fs.mkdirSync(path.join(fixture,bucket,'fixture.sh'),{recursive:true});
  fs.writeFileSync(path.join(fixture,bucket,'fixture.sh','fixture.sh'),'#!/bin/bash\nprintf "FAKE_PACKAGE_ONLY\\n"\n',{mode:0o700});
  const local=path.join(fixture,path.basename(original));fs.copyFileSync(original,local);
  const call=(name,value,expected)=>run(platform+'-'+type+'-desktop-'+name,args.slice(1).map(arg=>arg==='%k'?value:arg),expected);
  call('local',local,0);call('relative',path.basename(local),1);call('uri','file://'+local,1);
  fs.mkdirSync(path.join(fixture,bucket,'second.sh'));fs.writeFileSync(path.join(fixture,bucket,'second.sh','second.sh'),'#!/bin/bash\nexit 99\n');call('multiple-candidates',local,1);
}
const library=path.join(work,'library.sh');fs.writeFileSync(library,first.slice(0,first.lastIndexOf('\nmain "$@"'))+'\n');
const quote=s=>"'"+s.replaceAll("'","'\\''")+"'";
const shim=process.platform==='darwin'?String.raw`
stat() {
  [[ "$1" == -c ]] || return 2
  local format="$2"; shift 2; [[ "$1" != -- ]] || shift
  case "$format" in '%d:%i:%u') format='%d:%i:%u';; %d) format='%d';; %a) format='%Lp';; %u) format='%u';; %s) format='%z';; %h) format='%l';; *) return 2;; esac
  /usr/bin/stat -f "$format" "$@"
}
`:'';
const openssl=process.env.JOBSIM_TEST_OPENSSL||'/opt/homebrew/opt/openssl@3/bin/openssl';
const prelude='set -eu\numask 077\nsource '+quote(library)+'\n'+shim+'\nWORK='+quote(work)+'\nOPENSSL='+quote(openssl)+'\n'+String.raw`
REPO="$WORK/repository"; mkdir -p "$REPO"
REF_ID=''; REF_DEPLOYMENT=''; DEFAULT_INSTANCE_STATE=''; LOCK_HELD=0; STATE_EXPLICIT=1; YES=1
MAIN_PORT=16543; JOURNAL_PORT=16544; HTTPS_PORT=9443; ROLE_PORT=18101; ROLE_LAST=18109
STATE="$WORK/case-$CASE/instance"; BACKUPS="$WORK/case-$CASE/Backups"; RECOVERY="$WORK/case-$CASE/Recovery"
NATS_STORE="$STATE/Application/JetStream"; NATS_CONFIG="$STATE/Secrets/nats.conf"; JOBS_IM_NATS_TOKEN_FILE="$STATE/Secrets/nats.token"
ROLES=(gateway identity contact message media control sync)
mkdir -p "$WORK/case-$CASE"
fresh() {
  capture_resource_origin; prepare_state; acquire_lock; initialize_resource_ledger
  mkdir "$STATE/MainDatabase" "$STATE/JournalDatabase" "$STATE/Sessions"
  printf 'business-bytes' > "$STATE/MainDatabase/data"; printf 'journal-protected' > "$STATE/JournalDatabase/data"
  printf 'media-bytes' > "$STATE/Application/media"; printf 'instance-secret' > "$STATE/Application/instance.json"; printf 'journal-secret' > "$STATE/Secrets/journal.password"
  printf 'operator-synthetic' > "$STATE/Secrets/operator.password"
  record_resource MainDatabase; record_resource JournalDatabase; record_resource Sessions
  bind_resource_deployment 00000000-0000-4000-8000-000000000001; release_lock
}
stop_backend() { :; }; stop_nats() { :; }; stop_clusters() { :; }; stop_tunnel() { :; }
undeploy_select() { UNDEPLOY_CHOICE=cleanup; DATA_POLICY=wipe; }
undeploy_confirm() { :; }
`;
const cases=[
['created-origin',String.raw`fresh; resource_ledger_validate; grep -F $'created\tdirectory' "$STATE/.Deployment/resources.tsv" >/dev/null`],
['legacy-preserved',String.raw`mkdir -p "$STATE/Application"; printf old > "$STATE/Application/data"; printf '%s\n%s\n' "$REPO" "$(id -u)" > "$STATE/owner"; set +e; undeploy_instance; code=$?; set -e; [[ "$code" == 2 && -f "$STATE/Application/data" ]]`],
['wipe-protected-journal-keys-source',String.raw`fresh; set +e; undeploy_instance; code=$?; set -e; [[ "$code" == 2 && ! -e "$STATE/MainDatabase" && ! -e "$STATE/Application/media" && -f "$STATE/JournalDatabase/data" && -f "$STATE/Secrets/journal.password" && -f "$STATE/owner" && -d "$REPO" ]]`],
['repeat-partial',String.raw`fresh; set +e; undeploy_instance; a=$?; undeploy_instance; b=$?; set -e; [[ "$a" == 2 && "$b" == 2 && -f "$STATE/JournalDatabase/data" ]]`],
['reused-data-preserved',String.raw`mkdir -p "$STATE/Application"; printf prior > "$STATE/Application/prior"; printf '%s\n%s\n' "$REPO" "$(id -u)" > "$STATE/owner"; fresh; set +e; undeploy_instance; code=$?; set -e; [[ "$code" == 2 && -f "$STATE/Application/prior" && -f "$STATE/Application/media" ]]`],
['symlink-ancestor',String.raw`mkdir -p "$WORK/link-real"; ln -s "$WORK/link-real" "$WORK/link-parent"; set +e; assert_no_symlink_components "$WORK/link-parent/state"; code=$?; set -e; [[ "$code" == 2 ]]`],
['symlink-tree',String.raw`fresh; ln -s "$REPO" "$STATE/Application/foreign"; set +e; undeploy_snapshot "$STATE" "$WORK/link-plan"; code=$?; set -e; [[ "$code" == 2 && -L "$STATE/Application/foreign" && -f "$STATE/MainDatabase/data" ]]`],
['hardlink-tree',String.raw`fresh; ln "$STATE/Application/media" "$STATE/Application/alias"; set +e; undeploy_snapshot "$STATE" "$WORK/hardlink-plan"; code=$?; set -e; [[ "$code" == 2 && -f "$STATE/Application/media" ]]`],
['unknown-top',String.raw`fresh; mkdir "$STATE/ForeignProject"; set +e; undeploy_snapshot "$STATE" "$WORK/unknown-plan"; code=$?; set -e; [[ "$code" == 2 && -d "$STATE/ForeignProject" && -f "$STATE/MainDatabase/data" ]]`],
['changed-inode',String.raw`fresh; mv "$STATE/Application" "$WORK/old-app-$CASE"; mkdir "$STATE/Application"; set +e; resource_ledger_validate; code=$?; set -e; [[ "$code" == 2 && -f "$STATE/MainDatabase/data" ]]`],
['backup-failure',String.raw`fresh; undeploy_select() { UNDEPLOY_CHOICE=cleanup; DATA_POLICY=backup; }; undeploy_verify_backup() { return 47; }; set +e; undeploy_instance; code=$?; set -e; [[ "$code" == 47 && -f "$STATE/MainDatabase/data" && -f "$STATE/Application/media" ]]`],
['post-confirm-mutation',String.raw`fresh; undeploy_select() { UNDEPLOY_CHOICE=cleanup; DATA_POLICY=backup; }; undeploy_verify_backup() { printf changed > "$STATE/MainDatabase/data"; }; set +e; undeploy_instance; code=$?; set -e; [[ "$code" == 2 && -f "$STATE/MainDatabase/data" && -f "$STATE/Application/media" ]]`],
['interruption-resume',String.raw`fresh; rm() { command rm "$@"; case "$*" in *MainDatabase/data*) return 71;; esac; }; set +e; undeploy_instance; a=$?; unset -f rm; release_lock; undeploy_instance; b=$?; set -e; [[ "$a" == 71 && "$b" == 2 && ! -e "$STATE/MainDatabase" && -f "$STATE/JournalDatabase/data" ]]`],
['retain-no-removal',String.raw`fresh; undeploy_select() { UNDEPLOY_CHOICE=stop; }; undeploy_instance; [[ -f "$STATE/MainDatabase/data" && -f "$STATE/Application/media" && ! -d "$STATE/.Deployment/undeploy" ]]`],
['server-reject-loopback',String.raw`DEPLOY_MODE=server; JOBS_IM_PUBLIC_URL=https://127.0.0.1:9443; set +e; configure_public_mode; code=$?; set -e; [[ "$code" == 2 ]]`],
['server-real-origin',String.raw`DEPLOY_MODE=server; JOBS_IM_PUBLIC_URL=https://chat.example.test; configure_public_mode; [[ "$PUBLIC_HOST" == chat.example.test && "$PUBLIC_URL" == https://chat.example.test ]]`],
['conditional-undeploy-manifest-fails-before-archive-delete',String.raw`fresh; undeploy_select() { UNDEPLOY_CHOICE=cleanup; DATA_POLICY=backup; }; write_nats_backup_manifest() { return 55; }; if undeploy_instance; then code=0; else code=$?; fi; [[ "$code" == 55 && -f "$STATE/MainDatabase/data" && ! -e "$BACKUPS" && ! -e "$STATE/.Deployment/undeploy/phase" ]]`],
['local-default-does-not-require-cloudflare',String.raw`DEPLOY_MODE=local; ACCESS_EXPLICIT=0; LOCAL_ACCESS=quick; FZF_BIN="$WORK/first-choice-$CASE"; printf '#!/bin/bash\nhead -1\n' > "$FZF_BIN"; chmod 700 "$FZF_BIN"; select_local_access; configure_public_mode; [[ "$LOCAL_ACCESS" == local && "$PUBLIC_URL" == https://127.0.0.1:9443 ]]`],
['disk-actual-byte-boundary',String.raw`mkdir "$WORK/budget-$CASE"; available="$(/bin/df -Pk "$WORK/budget-$CASE" | awk 'NR==2{print $4}')"; room=$((available*1024-4294967296)); disk_budget_bytes "$WORK/budget-$CASE" "$((room-1048576))"; if disk_budget_bytes "$WORK/budget-$CASE" "$((room+1048576))"; then exit 91; fi`],
];
const backupFixture=String.raw`
fresh
PG_BIN="$WORK/pg-$CASE"; mkdir "$PG_BIN"
printf '#!/bin/bash\nfor arg in "$@"; do case "$arg" in --file=*) dest="$(printf %%s "$arg" | cut -c8-)"; printf synthetic-dump > "$dest" ;; esac; done\n' > "$PG_BIN/pg_dump"
printf '#!/bin/bash\nprintf synthetic-list\\n\n' > "$PG_BIN/pg_restore"
chmod 700 "$PG_BIN/pg_dump" "$PG_BIN/pg_restore"
mkdir -p "$RECOVERY"; printf synthetic-key > "$RECOVERY/key.pem"; printf synthetic-cert > "$RECOVERY/cert.pem"
prepare_clusters() { :; }; find_management_tools() { :; }; assert_owner() { :; }; confirm_action() { :; }; apply_owner_relocation() { :; }
OPENSSL="$WORK/openssl-marker-$CASE"; printf '#!/bin/bash\nprintf called > "$MARKER"\nexit 90\n' > "$OPENSSL"; chmod 700 "$OPENSSL"; export MARKER="$WORK/cms-called-$CASE"
ACTION=backup; NO_BACKUP=0
`;
for(const[stage,injection,expected]of[
 ['manifest','write_nats_backup_manifest() { return 55; };',55],
 ['dump',`printf '#!/bin/bash\\nexit 61\\n' > "$PG_BIN/pg_dump";`,61],
 ['restore-list',`printf '#!/bin/bash\\nexit 62\\n' > "$PG_BIN/pg_restore";`,62],
 ['stop-go','stop_backend() { return 63; };',63],
 ['stop-broker','stop_nats() { return 64; };',64],
 ['prepare-pg','prepare_clusters() { return 65; };',65],
 ['manifest-empty','write_nats_backup_manifest() { :; };',2],
])cases.push(['conditional-dispatch-backup-'+stage,backupFixture+'\n'+injection+`\nif dispatch_action; then code=0; else code=$?; fi; [[ "$code" == ${expected} && ! -e "$MARKER" && -f "$STATE/MainDatabase/data" ]]`]);
if(fs.existsSync(openssl)){
 const actualBackup=backupFixture+'\nOPENSSL='+quote(openssl)+'\nrm -- "$RECOVERY/key.pem" "$RECOVERY/cert.pem"\n';
 cases.push(['logical-eight-dumps-cms-roundtrip',actualBackup+String.raw`
if dispatch_action; then code=0; else code=$?; fi
[[ "$code" == 0 && -f "$STATE/MainDatabase/data" ]]
file="$(find "$BACKUPS" -type f -name '*.cms' | head -1)"; [[ -n "$file" ]]
[[ "$(grep -Ec '^\./[^/]+\.dump$' "$file.manifest")" == 8 ]]
grep -Fx ./broker-manifest.txt "$file.manifest"
`]);
 for(const stage of ['tar','encrypt','decrypt','decrypted-dump']){
  let injection=stage==='tar'?'tar() { return 66; };':stage==='decrypted-dump'?String.raw`printf '#!/bin/bash\ncase "$*" in *.verify-logical-*) exit 69 ;; esac\nprintf list\n' > "$PG_BIN/pg_restore";`:String.raw`REAL_OPENSSL="$OPENSSL"; OPENSSL=openssl_injected; openssl_injected() { if [[ "$1" == cms && "$2" == -STAGE ]]; then return 67; fi; "$REAL_OPENSSL" "$@"; };`.replace('-STAGE','-'+stage);
  cases.push(['conditional-cms-'+stage,actualBackup+'\n'+injection+String.raw`
if dispatch_action; then code=0; else code=$?; fi
[[ "$code" != 0 && -f "$STATE/MainDatabase/data" && -f "$STATE/Application/media" ]]
[[ "$(find "$BACKUPS" -type f -name '*.sha256' | wc -l | tr -d ' ')" == 0 ]]
`]);
 }
}
const accessFixture=String.raw`
fresh
DEPLOY_MODE=local; LOCAL_ACCESS=quick; PUBLIC_HOST=127.0.0.1; PUBLIC_URL=https://old.trycloudflare.com
RUN_DIR="$STATE/Sessions/current"; mkdir "$RUN_DIR"; TRACE="$WORK/access-$CASE.trace"; : > "$TRACE"
printf '{"url":"https://old.trycloudflare.com"}\n' > "$STATE/connection.json"
expire_access_qr() { printf 'expire\n' >> "$TRACE"; }; stop_tunnel() { printf 'stop-tunnel\n' >> "$TRACE"; }; stop_backend() { printf 'stop-go\n' >> "$TRACE"; }
start_backend_for_url() { [[ "$1" == https://127.0.0.1:9443 ]] || return 88; printf 'start-local\n' >> "$TRACE"; }
write_connection() { printf '{"url":"%s"}\n' "$PUBLIC_URL" > "$STATE/connection.json"; }; prepare_access_qr() { printf 'local-qr\n' >> "$TRACE"; }
stop_parent_guardian() { :; }; start_parent_guardian() { printf 'guardian\n' >> "$TRACE"; }
stop_nats() { return 89; }; stop_clusters() { return 89; }
`;
cases.push(['quick-failure-keeps-local',accessFixture+String.raw`
start_quick_tunnel() { PUBLIC_URL=https://failed.trycloudflare.com; return 71; }
attempt_quick_access
[[ "$LOCAL_ACCESS" == local && "$PUBLIC_URL" == https://127.0.0.1:9443 && -f "$STATE/JournalDatabase/data" ]]
grep -F 'https://127.0.0.1:9443' "$STATE/connection.json"; ! grep -F trycloudflare "$STATE/connection.json"
grep -Fx start-local "$TRACE"; grep -Fx local-qr "$TRACE"
`]);
cases.push(['quick-late-exit-monitor-still-has-stop-menu',accessFixture+String.raw`
tunnel_matches() { return 1; }; process_matches() { return 0; }
printf stop > "$RUN_DIR/menu-request"
monitor_instance
[[ "$LOCAL_ACCESS" == local && "$PUBLIC_URL" == https://127.0.0.1:9443 && ! -e "$RUN_DIR/menu-request" ]]
grep -Fx guardian "$TRACE"
`]);
cases.push(['local-mode-monitor-does-not-demand-tunnel',accessFixture+String.raw`
LOCAL_ACCESS=local; tunnel_matches() { return 79; }; process_matches() { return 0; }
printf stop > "$RUN_DIR/menu-request"; monitor_instance; [[ ! -s "$TRACE" ]]
`]);
for(let i=0;i<cases.length;i++){const[name,body]=cases[i],file=path.join(work,'case-'+i+'.sh');fs.writeFileSync(file,'CASE='+i+'\n'+prelude+'\n'+body+'\n');run(name,[file])}
for(const[name,input,expected]of[['confirmation-eof','',130],['confirmation-nonempty','cancel\n',130],['confirmation-enter','\n',0]]){const file=path.join(work,name+'.sh');fs.writeFileSync(file,'CASE='+name+'\n'+prelude.replace('undeploy_confirm() { :; }','')+'\nDATA_POLICY=wipe; undeploy_confirm\n');run(name,[file],expected,input)}
if(fs.existsSync(openssl)){const file=path.join(work,'cold-backup.sh');fs.writeFileSync(file,'CASE=cold-backup\n'+prelude+String.raw`
fresh
undeploy_select() { UNDEPLOY_CHOICE=cleanup; DATA_POLICY=backup; }
set +e; undeploy_instance; code=$?; set -e
[[ "$code" == 2 && ! -e "$STATE/MainDatabase" && -f "$STATE/JournalDatabase/data" && -f "$RECOVERY/key.pem" ]]
[[ "$(find "$BACKUPS" -type f -name '*.cms' | wc -l | tr -d ' ')" == 1 ]]
`);run('cold-cms-backup-bytes',[file])}else results.push({name:'cold-cms-backup-bytes',skipped:true,reason:'No explicit OpenSSL3; no installation attempted'});
const after=hashes();results.push({name:'source-stable',passed:JSON.stringify(before)===JSON.stringify(after)});
const summary={at:new Date().toISOString(),host:process.platform,work,sourceBefore:before,sourceAfter:after,library:{file:source,sha256:crypto.createHash('sha256').update(first).digest('hex')},runnerSHA256:runnerHash,pass:results.filter(r=>r.passed===true).length,fail:results.filter(r=>r.passed===false).length,skip:results.filter(r=>r.skipped).length,results,scope:'Bash syntax, non-Linux refusal, actual private file operations and real OpenSSL CMS encryption/decryption. Darwin stat flags adapted; lifecycle/network and pg_dump/pg_restore are explicit synthetic fixtures. No target Linux, real PG/NATS, network tunnel, package install or actual instance touched.'};fs.writeFileSync(path.join(report,'结果.json'),JSON.stringify(summary,null,2));console.log(JSON.stringify({report,pass:summary.pass,fail:summary.fail,skip:summary.skip}));process.exitCode=summary.fail?1:0;
