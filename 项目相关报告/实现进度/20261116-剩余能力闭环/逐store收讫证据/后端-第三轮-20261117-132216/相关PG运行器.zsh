#!/bin/zsh
set -eu
setopt PIPE_FAIL
umask 077
local_run="$HOME/Library/Application Support/JobsIM/Integration/66bf4668-aa47-4668-b583-925a7ac93cc0/Safety"
pgbin="$HOME/Library/Application Support/JobsIM/Toolchains/PostgresApp-2.9.7/Postgres.app/Contents/Versions/18/bin"
go_binary="$HOME/Library/Application Support/JobsIM/Toolchains/go1.27.4/go/bin/go"
evidence=${0:A:h}
main_started=0
journal_started=0
for cluster in MainDatabase JournalDatabase; do
 if "$pgbin/pg_ctl" -D "$local_run/$cluster" status >/dev/null 2>&1; then print -u2 "owned cluster already active; no lifecycle changed"; exit 70; fi
done
python3 - <<'CHECK'
import os,subprocess
from pathlib import Path
if os.statvfs('.').f_bavail*os.statvfs('.').f_frsize<4*1024**3:raise SystemExit('4GiB gate: PG window deferred')
for port in [62381,62382]:
 if subprocess.run(['/usr/sbin/lsof','-nP','-iTCP:'+str(port),'-sTCP:LISTEN'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode==0:raise SystemExit('owned PG endpoint occupied; no lifecycle changed')
CHECK
finish(){
 local code=$?
 trap - EXIT
 if (( journal_started )); then "$pgbin/pg_ctl" -D "$local_run/JournalDatabase" -m fast -w -t 30 stop >> "$evidence/相关生命周期.log" 2>&1 || true; fi
 if (( main_started )); then "$pgbin/pg_ctl" -D "$local_run/MainDatabase" -m fast -w -t 30 stop >> "$evidence/相关生命周期.log" 2>&1 || true; fi
 printf 'testExit=%s\n' "$code" >> "$evidence/相关生命周期.log"
 exit "$code"
}
trap finish EXIT
export PGPASSFILE="$local_run/Secrets/pgpass" GOMAXPROCS=2 GOPROXY=off GOTOOLCHAIN=local
main_started=1
"$pgbin/pg_ctl" -D "$local_run/MainDatabase" -l "$local_run/Logs/pg-Main.log" -w -t 30 -o "-p 62381 -h 127.0.0.1 -k '' -c max_connections=100 -c shared_buffers=64MB -c fsync=on -c full_page_writes=on -c synchronous_commit=on" start >> "$evidence/相关生命周期.log" 2>&1
journal_started=1
"$pgbin/pg_ctl" -D "$local_run/JournalDatabase" -l "$local_run/Logs/pg-Journal.log" -w -t 30 -o "-p 62382 -h 127.0.0.1 -k '' -c max_connections=100 -c shared_buffers=64MB -c fsync=on -c full_page_writes=on -c synchronous_commit=on" start >> "$evidence/相关生命周期.log" 2>&1
journal_started=1
export JOBS_IM_SYNC_SCHEMAS_TEST=1 JOBS_IM_SYNC_SCHEMAS_REALM_URL=https://127.0.0.1:59443
export JOBS_IM_SYNC_SCHEMAS_ADMIN_URL='postgresql://s2_owner_66bf4668aa47@127.0.0.1:62381/postgres?sslmode=disable'
export JOBS_IM_RECEIPT_RUNTIME_TEST=1
export JOBS_IM_RECEIPT_JOURNAL_ADMIN_URL='postgresql://s2_owner_66bf4668aa47@127.0.0.1:62382/postgres?sslmode=disable'
export JOBS_IM_RECEIPT_NATS_BINARY="$HOME/Library/Application Support/JobsIM/Toolchains/NATS-v2.15.3/nats-server-v2.15.3-darwin-arm64/nats-server"
"$go_binary" tool test2json -p jobsim/backend -t "$evidence/候选.test" -test.v -test.run '^Test(Receipt|ObjectReceipt|Retention|MediaRetention|SyncMigrationPG)' -test.timeout=120s > "$evidence/相关真实PG.jsonl" 2> "$evidence/相关真实PG.stderr"
