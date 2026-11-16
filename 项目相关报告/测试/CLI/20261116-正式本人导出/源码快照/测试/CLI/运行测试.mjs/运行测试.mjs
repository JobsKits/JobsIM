#!/usr/bin/env node
// Created by Jobs on 2026年11月14日，星期六.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import crypto from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

const here = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const root = path.resolve(here, '../..');
const temporary = fs.mkdtempSync(path.join(os.tmpdir(), 'jobsim-cli-tests-'));
const overlay = path.join(temporary, 'overlay.json');
const testTimeout = process.env.JOBSIM_CLI_TEST_TIMEOUT ?? '120s';
if (!/^[1-9][0-9]*(ms|s|m)$/.test(testTimeout)) throw new Error('Invalid JOBSIM_CLI_TEST_TIMEOUT');
const tests = ['outbox_test.go', 'governance_test.go', 'collaboration_test.go', 'auth_test.go', 'auth_https_test.go', 'sync_test.go', 'self_state_test.go', 'receipt_test.go', 'receipt_objects_test.go', 'data_export_test.go'];
const replacements = Object.fromEntries(tests.map(name => [
  path.join(root, '代码/前端/CLI/cmd/jobsim', name), path.join(here, name)
]));
fs.writeFileSync(overlay, JSON.stringify({ Replace: replacements }));
const result = spawnSync(process.env.GO_BINARY ?? 'go', [
  '-C', path.join(root, '代码/前端/CLI'), 'test', '-overlay', overlay, '-timeout', testTimeout, '-count=1', '-v', './...'
], { encoding: 'utf8', env: process.env });
const output = (result.stdout ?? '') + (result.stderr ?? '');
process.stdout.write(output);
const sources = [
  ...fs.readdirSync(path.join(root, '代码/前端/CLI/cmd/jobsim')).filter(name => name.endsWith('.go')).map(name => '代码/前端/CLI/cmd/jobsim/' + name),
  '代码/前端/CLI/go.mod', '代码/前端/CLI/go.sum',
  ...tests.map(name => '测试/CLI/' + name)
].sort();
const sourceFiles = sources.map(name => ({
  path: name, sha256: crypto.createHash('sha256').update(fs.readFileSync(path.join(root, name))).digest('hex')
}));
const report = process.env.JOBSIM_CLI_REPORT_DIR ? path.resolve(process.env.JOBSIM_CLI_REPORT_DIR) : path.join(root, '项目相关报告/测试/CLI/执行记录', String(Date.now()) + '-' + process.pid);
fs.mkdirSync(report, { recursive: true });
fs.writeFileSync(path.join(report, 'SQLite测试.log'), output);
fs.writeFileSync(path.join(report, 'SQLite结果.json'), JSON.stringify({
  at: new Date().toISOString(), status: result.status === 0 ? 'passed' : 'failed',
  exitCode: result.status, error: result.error?.code ?? null,
  passedGroups: [...output.matchAll(/^--- PASS: ([^ ]+)/gm)].length,
  failedGroups: [...output.matchAll(/^--- FAIL: ([^ ]+)/gm)].length,
  sourceFiles,
  checks: [
    'durable commit before HTTP; original ID/payload after unknown results',
    'real SQLite reopen, namespace/restore isolation, WAL/FULL',
    'message/cursor atomicity and large string version monotonicity',
    'explicit atomic legacy import; unavailable content never exposes old body',
    'original send admission retained; absent capability receives no POST',
    'request/reopen original operation persisted before HTTP and reused after reopen',
    'completed operations query current inbound policy without new POST',
    'only corresponding direct is frozen before relationship write; raw payload/hash unchanged',
    'leave/member-self removal atomically withdraws history/cursor before HTTP and retains draft',
    'SQL fault rolls leave freeze/history/cursor back together',
    'explicit original member generations; PATCH/DELETE unknown results not replayed',
    'schema1 to schema2 retains original Outbox bytes and independent reason',
    'original frozen reconciliation uses full route/body read-only message-results, never creates an old intent',
    'schema1 authoritative snapshot/change raw envelope checks, durable original registration and SQLite guard CAS',
    'sync client ID is never an ACK; complete original read-only result and payload hash are required',
    'terminal global cleanup and scope-proven cross-device durable cleanup do not resurrect retained bodies'
  ]
}, null, 2));
fs.unlinkSync(overlay);
fs.rmdirSync(temporary);
process.exitCode = result.status ?? 1;
